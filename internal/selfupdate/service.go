package selfupdate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// Service implements api.SelfUpdateService (internal/api/services.go),
// structurally rather than by importing internal/api: jobs.Runner and the
// other feature packages follow the same rule to avoid an import cycle (see
// internal/jobs/runner.go's package comment).
type Service struct {
	client   *Client
	checker  *Checker
	upgrader *Upgrader
	runner   *jobs.Runner
	log      *slog.Logger

	// exit ends the process once the new binary is in place; systemd's
	// Restart=always then starts it again running the new binary (see
	// deploy/valheim-ui.service). Exit code 0 is deliberate: it is a clean,
	// intentional restart, not a crash, and keeps this path indistinguishable
	// from an admin running `systemctl restart valheim-ui` for monitoring
	// purposes. Overridable so tests never actually exit the test process.
	exit func(code int)

	// mu guards the exit-pending latch below.
	mu sync.Mutex
	// exitPending is set once an upgrade has applied and the process is about
	// to restart. It is never cleared: the process exits shortly after, and
	// the new process starts with a fresh (unset) latch.
	exitPending bool
	// upgradeTarget is the release the in-flight/just-applied upgrade installs.
	upgradeTarget string
}

// ServiceOption configures a Service at construction time.
type ServiceOption func(*Service)

// WithExit overrides the default os.Exit(0) (tests only).
func WithExit(fn func(code int)) ServiceOption {
	return func(s *Service) {
		if fn != nil {
			s.exit = fn
		}
	}
}

// NewService builds a Service. client, checker and upgrader must share the
// same underlying repository/binary as configured by wire_selfupdate.go.
func NewService(client *Client, checker *Checker, upgrader *Upgrader, runner *jobs.Runner, log *slog.Logger, opts ...ServiceOption) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		client:   client,
		checker:  checker,
		upgrader: upgrader,
		runner:   runner,
		log:      log,
		exit:     func(code int) { os.Exit(code) },
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Info returns the checker's last known update info (never nil), with the
// current self-upgrade phase attached so the UI can gate its Upgrade button.
func (s *Service) Info(_ context.Context) *domain.AppUpdateInfo {
	info := s.checker.Info()
	info.Upgrade = s.upgradeState()
	return info
}

// upgradeState reports the current self-upgrade phase: exit_pending once an
// upgrade has applied, running while a self_upgrade job is queued/running,
// else idle.
func (s *Service) upgradeState() *domain.AppUpgradeState {
	s.mu.Lock()
	exitPending, target := s.exitPending, s.upgradeTarget
	s.mu.Unlock()
	if exitPending {
		return &domain.AppUpgradeState{State: domain.UpgradeExitPending, Target: target}
	}
	if active := s.runner.ActiveFor(""); active != nil && active.Type == domain.JobSelfUpgrade {
		return &domain.AppUpgradeState{State: domain.UpgradeRunning, Target: target}
	}
	return &domain.AppUpgradeState{State: domain.UpgradeIdle}
}

// markExitPending latches the exit-pending state for target. Called once the
// upgrade has applied, just before the process is scheduled to restart.
func (s *Service) markExitPending(target string) {
	s.mu.Lock()
	s.exitPending = true
	s.upgradeTarget = target
	s.mu.Unlock()
}

// CheckNow forces an immediate release check.
func (s *Service) CheckNow(ctx context.Context) (*domain.AppUpdateInfo, error) {
	return s.checker.CheckNow(ctx)
}

// EnqueueUpgrade enqueues a self_upgrade job that downloads and installs
// version (or the latest release when version == "") and restarts the
// manager. All refusals use domain.CodeConflict: docs/openapi.yaml only
// documents 202/409 for POST /system/upgrade.
func (s *Service) EnqueueUpgrade(ctx context.Context, version, requestedBy string) (*domain.Job, error) {
	s.mu.Lock()
	exitPending := s.exitPending
	s.mu.Unlock()
	if exitPending {
		return nil, domain.E(domain.CodeConflict, "a self-upgrade has completed and the manager is restarting")
	}
	if active := s.runner.ActiveFor(""); active != nil && active.Type == domain.JobSelfUpgrade {
		return nil, domain.E(domain.CodeConflict, "a self-upgrade job is already active")
	}

	if version != "" && !releaseTagPattern.MatchString(version) {
		return nil, domain.E(domain.CodeValidationFailed, "version must look like v1.2.3")
	}
	rel, err := s.resolveRelease(ctx, version)
	if err != nil {
		return nil, err
	}

	info := s.checker.Info()
	// Never move backwards through this path: an older release may carry
	// fixed vulnerabilities. `valheim-ui self-upgrade --rollback` on the host
	// is the deliberate way back to the previous binary.
	if parseVersion(info.CurrentVersion).ok && Compare(rel.Tag, info.CurrentVersion) < 0 {
		return nil, domain.Ef(domain.CodeConflict, "refusing to downgrade from %s to %s", info.CurrentVersion, rel.Tag)
	}
	if !info.CanSelfUpgrade {
		reason := info.Reason
		if reason == "" {
			reason = "self-upgrade is unavailable"
		}
		return nil, domain.Ef(domain.CodeConflict, "cannot self-upgrade: %s", reason)
	}
	fromVersion := info.CurrentVersion

	target := *rel
	s.mu.Lock()
	s.upgradeTarget = target.Tag
	s.mu.Unlock()
	spec := jobs.Spec{
		Type:        domain.JobSelfUpgrade,
		InstanceID:  "",
		Title:       fmt.Sprintf("Upgrade to %s", target.Tag),
		RequestedBy: requestedBy,
	}
	fn := func(ctx context.Context, logger *jobs.Logger) error {
		if _, err := s.upgrader.Apply(ctx, &target, logger.Writer()); err != nil {
			return err
		}
		logger.SetSummary("from", fromVersion)
		logger.SetSummary("to", target.Tag)
		logger.Printf("restarting manager in 2s; game servers keep running (they are separate systemd units)")

		// Latch exit-pending so a second upgrade request in the brief window
		// before the process restarts is refused (409) rather than enqueued
		// and then orphaned by the exit.
		s.markExitPending(target.Tag)

		// The runner marks this job succeeded and publishes job.updated
		// right after Func returns below; exiting has to happen strictly
		// after that so the client sees the finished job before the SSE
		// stream drops. A short, detached sleep gives that ample time
		// without blocking (and therefore delaying) the job's own
		// completion.
		go func() {
			time.Sleep(2 * time.Second)
			s.exit(0)
		}()
		return nil
	}

	return s.runner.Enqueue(ctx, spec, fn)
}

// resolveRelease looks up the release to install: the latest one when
// version == "", or the exact tag otherwise.
func (s *Service) resolveRelease(ctx context.Context, version string) (*Release, error) {
	if version == "" {
		rel, err := s.client.Latest(ctx)
		if err != nil {
			return nil, err
		}
		if rel == nil {
			return nil, domain.E(domain.CodeConflict, "no releases are published yet")
		}
		return rel, nil
	}
	rel, err := s.client.ByTag(ctx, version)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, domain.Ef(domain.CodeConflict, "release %s not found", version)
	}
	return rel, nil
}

// releaseTagPattern is the tag shape the release workflow publishes.
var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)
