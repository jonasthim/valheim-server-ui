package steam

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// InstalledRef is one instance the UpdateChecker should compare against the
// latest public build id.
type InstalledRef struct {
	ID         string
	InstallDir string
}

// StoreFunc persists one instance's check result (the instance service
// implements this against the instances table's latest_buildid/
// buildid_checked_at columns).
type StoreFunc func(ctx context.Context, instanceID, installedBuildID, latestBuildID string, checkedAt time.Time) error

// ListInstalledFunc lists every instance that has a game install to check.
type ListInstalledFunc func(ctx context.Context) ([]InstalledRef, error)

// UpdateCheckerOption configures an UpdateChecker at construction time.
type UpdateCheckerOption func(*UpdateChecker)

// WithClock overrides time.Now (tests only).
func WithClock(now func() time.Time) UpdateCheckerOption {
	return func(u *UpdateChecker) { u.now = now }
}

// WithLogger overrides the default (slog.Default()) logger.
func WithLogger(log *slog.Logger) UpdateCheckerOption {
	return func(u *UpdateChecker) { u.log = log }
}

// UpdateChecker periodically compares the installed build id of every
// instance against the latest public Steam build id, publishing
// domain.EventUpdateAvailable and storing the result for each instance whose
// installed build differs. See ARCHITECTURE.md §11.
type UpdateChecker struct {
	client        *Client
	interval      func() time.Duration
	listInstalled ListInstalledFunc
	store         StoreFunc
	bus           domain.Publisher
	log           *slog.Logger
	now           func() time.Time

	mu   sync.Mutex
	info map[string]*domain.UpdateInfo
}

// NewUpdateChecker builds an UpdateChecker. interval is re-evaluated before
// every wait so a settings change takes effect on the next cycle; a value <=
// 0 disables the periodic loop (CheckNow still works on demand). listInstalled
// and store may be nil (Run then only refreshes the cached latest build id);
// bus may be nil (no update.available events are published).
func NewUpdateChecker(client *Client, interval func() time.Duration, listInstalled ListInstalledFunc, store StoreFunc, bus domain.Publisher, opts ...UpdateCheckerOption) *UpdateChecker {
	u := &UpdateChecker{
		client:        client,
		interval:      interval,
		listInstalled: listInstalled,
		store:         store,
		bus:           bus,
		log:           slog.Default(),
		now:           func() time.Time { return time.Now().UTC() },
		info:          map[string]*domain.UpdateInfo{},
	}
	for _, o := range opts {
		o(u)
	}
	return u
}

// Run blocks, checking every interval() until ctx is done. When interval()
// is <= 0 it re-polls the provider periodically instead of checking, so a
// later settings change re-enabling it takes effect without a restart.
func (u *UpdateChecker) Run(ctx context.Context) {
	const disabledPoll = time.Minute
	for {
		d := u.interval()
		wait := d
		if wait <= 0 {
			wait = disabledPoll
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if d <= 0 {
			continue
		}
		if err := u.CheckNow(ctx); err != nil {
			u.log.Warn("steam: update check failed", "err", err)
		}
	}
}

// CheckNow runs one check immediately, regardless of the configured interval.
func (u *UpdateChecker) CheckNow(ctx context.Context) error {
	latest, err := u.client.LatestBuildID(ctx)
	if err != nil {
		return err
	}
	if u.listInstalled == nil {
		return nil
	}
	refs, err := u.listInstalled(ctx)
	if err != nil {
		u.log.Warn("steam: list installed instances", "err", err)
		return err
	}
	now := u.now()
	for _, ref := range refs {
		installed, err := u.client.InstalledBuildID(ref.InstallDir)
		if err != nil {
			u.log.Warn("steam: installed buildid", "instance", ref.ID, "err", err)
			continue
		}
		info := &domain.UpdateInfo{
			InstanceID:       ref.ID,
			InstalledBuildID: installed,
			LatestBuildID:    latest,
			UpdateAvailable:  installed != "" && latest != "" && installed != latest,
			CheckedAt:        &now,
		}
		u.mu.Lock()
		u.info[ref.ID] = info
		u.mu.Unlock()

		if u.store != nil {
			if err := u.store(ctx, ref.ID, installed, latest, now); err != nil {
				u.log.Warn("steam: store update info", "instance", ref.ID, "err", err)
			}
		}
		if info.UpdateAvailable && u.bus != nil {
			u.bus.Publish(domain.Event{Name: domain.EventUpdateAvailable, InstanceID: ref.ID, Data: *info})
		}
	}
	return nil
}

// Info returns the last computed result for instanceID, or nil if it has
// never been checked.
func (u *UpdateChecker) Info(instanceID string) *domain.UpdateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	info, ok := u.info[instanceID]
	if !ok {
		return nil
	}
	cp := *info
	return &cp
}
