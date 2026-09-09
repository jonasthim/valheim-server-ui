package selfupdate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// UpgradeFunc actually performs an upgrade to version (a release tag). The
// checker calls it only when auto-upgrade is enabled, a newer version was
// seen, self-upgrade is possible, and no players are online anywhere.
type UpgradeFunc func(ctx context.Context, version string) error

// Checker periodically compares the running version against the latest
// GitHub release, publishing domain.EventAppUpdateAvailable once per newer
// version seen, and optionally triggers an automatic upgrade. See
// docs/ARCHITECTURE.md §9/§16.
type Checker struct {
	client         *Client
	currentVersion string
	exePath        string
	interval       func() time.Duration
	autoUpgrade    func() bool
	playersOnline  func(ctx context.Context) (int, error)
	bus            domain.Publisher
	log            *slog.Logger

	mu       sync.Mutex
	info     *domain.AppUpdateInfo
	notified map[string]bool
	upgrade  UpgradeFunc
}

// New builds a Checker. interval is re-evaluated on every tick (a settings
// change takes effect on the next cycle, mirroring steam.UpdateChecker); a
// value <= 0 disables the periodic loop but CheckNow still works on demand.
// playersOnline reports how many players are connected across every
// instance; bus may be nil (no events published).
func New(
	client *Client,
	currentVersion string,
	exePath string,
	interval func() time.Duration,
	autoUpgrade func() bool,
	playersOnline func(ctx context.Context) (int, error),
	bus domain.Publisher,
	log *slog.Logger,
) *Checker {
	if log == nil {
		log = slog.Default()
	}
	c := &Checker{
		client:         client,
		currentVersion: currentVersion,
		exePath:        exePath,
		interval:       interval,
		autoUpgrade:    autoUpgrade,
		playersOnline:  playersOnline,
		bus:            bus,
		log:            log,
		notified:       map[string]bool{},
	}
	c.mu.Lock()
	c.info = c.freshInfo(nil)
	c.mu.Unlock()
	return c
}

// SetUpgradeFunc wires the hook used for automatic upgrades. It is a
// separate method (rather than a New parameter) because the hook is usually
// the upgrade Service's EnqueueUpgrade, and that Service is constructed
// after the Checker (it needs the Checker for CanSelfUpgrade); wiring
// connects the two once both exist. Safe to call from any goroutine.
func (c *Checker) SetUpgradeFunc(fn UpgradeFunc) {
	c.mu.Lock()
	c.upgrade = fn
	c.mu.Unlock()
}

// Run blocks, checking every interval() until ctx is done. When interval()
// is <= 0 it still wakes periodically so a later settings change re-enabling
// it takes effect without a restart (mirrors steam.UpdateChecker.Run).
func (c *Checker) Run(ctx context.Context) {
	const disabledPoll = time.Hour
	for {
		d := c.interval()
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
		if _, err := c.CheckNow(ctx); err != nil {
			c.log.Warn("selfupdate: check failed", "err", err)
		}
	}
}

// CheckNow runs one check immediately, regardless of the configured
// interval, updates the cached Info, publishes app.update_available the
// first time a given newer version is seen, and — when auto-upgrade is
// enabled, self-upgrade is possible and nobody is playing — triggers the
// upgrade hook.
func (c *Checker) CheckNow(ctx context.Context) (*domain.AppUpdateInfo, error) {
	rel, err := c.client.Latest(ctx)
	if err != nil {
		return nil, err
	}
	info := c.freshInfo(rel)

	c.mu.Lock()
	c.info = info
	alreadyNotified := info.UpdateAvailable && c.notified[info.LatestVersion]
	if info.UpdateAvailable {
		c.notified[info.LatestVersion] = true
	}
	c.mu.Unlock()

	if info.UpdateAvailable && !alreadyNotified {
		if c.bus != nil {
			c.bus.Publish(domain.Event{Name: domain.EventAppUpdateAvailable, Data: *info})
		}
	}

	if info.UpdateAvailable && info.CanSelfUpgrade && c.autoUpgrade != nil && c.autoUpgrade() {
		c.maybeAutoUpgrade(ctx, info)
	}
	return info, nil
}

// maybeAutoUpgrade calls the upgrade hook when nobody is playing. It never
// panics: a bad playersOnline/upgrade implementation is logged, not fatal.
func (c *Checker) maybeAutoUpgrade(ctx context.Context, info *domain.AppUpdateInfo) {
	defer func() {
		if p := recover(); p != nil {
			c.log.Error("selfupdate: auto-upgrade panicked", "panic", p)
		}
	}()

	if c.playersOnline == nil {
		return
	}
	n, err := c.playersOnline(ctx)
	if err != nil {
		c.log.Warn("selfupdate: players online check failed", "err", err)
		return
	}
	if n != 0 {
		return
	}

	c.mu.Lock()
	fn := c.upgrade
	c.mu.Unlock()
	if fn == nil {
		return
	}
	c.log.Info("selfupdate: auto-upgrading", "from", info.CurrentVersion, "to", info.LatestVersion)
	if err := fn(ctx, info.LatestVersion); err != nil {
		c.log.Warn("selfupdate: auto-upgrade failed", "version", info.LatestVersion, "err", err)
	}
}

// Info returns the last known update info. It is always non-nil, with
// CurrentVersion, CanSelfUpgrade and Reason always filled in (recomputed
// fresh every call, since directory writability can change at runtime),
// even before the first CheckNow.
func (c *Checker) Info() *domain.AppUpdateInfo {
	c.mu.Lock()
	prev := c.info
	c.mu.Unlock()
	if prev == nil {
		return c.freshInfo(nil)
	}
	// Recompute the self-upgrade verdict fresh, keep the rest of the last
	// known release info as-is.
	cp := *prev
	cp.CanSelfUpgrade, cp.Reason = canSelfUpgrade(c.exePath, c.currentVersion)
	cp.PreviousVersion = readPrevVersion(c.exePath)
	return &cp
}

// freshInfo builds an AppUpdateInfo from rel (nil meaning "no release yet"
// or "not checked yet"), always filling CurrentVersion/CanSelfUpgrade/Reason.
func (c *Checker) freshInfo(rel *Release) *domain.AppUpdateInfo {
	canSelf, reason := canSelfUpgrade(c.exePath, c.currentVersion)
	info := &domain.AppUpdateInfo{
		CurrentVersion:  c.currentVersion,
		CanSelfUpgrade:  canSelf,
		Reason:          reason,
		PreviousVersion: readPrevVersion(c.exePath),
	}
	now := time.Now().UTC()
	info.CheckedAt = &now
	tagged := parseVersion(c.currentVersion).ok
	switch {
	case rel == nil:
		info.Message = "no releases have been published yet"
	case !tagged && c.currentVersion != "dev":
		info.Message = "running an untagged build (" + c.currentVersion + "); the published release " + rel.Tag + " can be installed"
	}
	if rel != nil {
		info.LatestVersion = rel.Tag
		info.UpdateAvailable = c.currentVersion != "dev" && Compare(rel.Tag, c.currentVersion) > 0
		info.ReleaseURL = rel.HTMLURL
		info.ReleaseNotes = rel.Body
		if !rel.PublishedAt.IsZero() {
			pub := rel.PublishedAt
			info.PublishedAt = &pub
		}
	}
	return info
}

// canSelfUpgrade reports whether the manager can replace its own binary:
// version must not be "dev"/empty, exePath must resolve (through symlinks)
// to a regular file, and its directory must be writable by this process.
func canSelfUpgrade(exePath, version string) (ok bool, reason string) {
	if version == "" || version == "dev" {
		return false, "development build"
	}
	real, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		return false, "release asset missing"
	}
	fi, err := os.Lstat(real)
	if err != nil || !fi.Mode().IsRegular() {
		return false, "release asset missing"
	}
	dir := filepath.Dir(real)
	if !dirWritable(dir) {
		return false, "binary directory not writable: " + dir
	}
	return true, ""
}

// dirWritable reports whether this process can create files in dir. It
// actually attempts a create-and-remove rather than inspecting permission
// bits, so it naturally agrees with the OS about edge cases such as root
// (which ignores most permission bits).
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".valheim-ui-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// readPrevVersion returns the version recorded by Upgrader.Apply at
// "<exePath>.prev.version", or "" if there is none (never upgraded, or the
// rollback marker was already cleaned up).
func readPrevVersion(exePath string) string {
	real, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		real = exePath
	}
	b, err := os.ReadFile(real + ".prev.version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
