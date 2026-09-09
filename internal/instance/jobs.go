// Steam install/update jobs and on-demand update checks (WP-05). See
// docs/ARCHITECTURE.md §9/§11 and docs/WORKPLAN.md WP-05.
package instance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/steam"
)

// stopPollInterval/stopPollTimeout bound how long an update job waits for an
// instance it stopped itself to actually reach the stopped state before
// giving up (ARCHITECTURE.md §9).
const (
	stopPollInterval = 500 * time.Millisecond
	stopPollTimeout  = 150 * time.Second
)

// PreUpdateBackupFunc runs a "pre_update" backup before an update job
// installs new game files. It is provided by WP-06's backup.Service; a nil
// value skips the backup step entirely regardless of the instance's
// BackupBeforeUpdate setting (used until WP-06 lands, and safe for instances
// that never asked for the backup).
type PreUpdateBackupFunc func(ctx context.Context, instanceID string, log *jobs.Logger) error

// SteamJobs implements api.SteamService: it enqueues install/update jobs
// against the job runner and answers on-demand update checks, using the
// instance Service to stop/start the instance around an update and to
// persist installed/latest build ids.
//
// It deliberately does not import internal/api (unlike steam.Service, which
// can afford to): internal/api's own test files import internal/instance to
// build a real Service for handler tests, and instance importing api back
// would be an import cycle for those internal test files. api.SteamService
// satisfaction is checked structurally where it matters — the assignment in
// cmd/valheim-ui/wire_steamjobs.go, which does import both packages.
type SteamJobs struct {
	svc             *Service
	runner          *jobs.Runner
	client          *steam.Client
	checker         *steam.UpdateChecker // optional; nil falls back to client.LatestBuildID
	preUpdateBackup PreUpdateBackupFunc  // optional; nil skips the pre-update backup step
	log             *slog.Logger
}

// NewSteamJobs builds the api.SteamService adapter for install/update jobs.
// checker and preUpdateBackup may be nil.
func NewSteamJobs(svc *Service, runner *jobs.Runner, client *steam.Client, checker *steam.UpdateChecker, preUpdateBackup PreUpdateBackupFunc, log *slog.Logger) *SteamJobs {
	if log == nil {
		log = slog.Default()
	}
	return &SteamJobs{svc: svc, runner: runner, client: client, checker: checker, preUpdateBackup: preUpdateBackup, log: log}
}

// isBusyState reports whether st is running, starting or stopping (i.e. not a
// safe time to overwrite game files in place without stopping first).
func isBusyState(st domain.InstanceState) bool {
	switch st {
	case domain.StateRunning, domain.StateStarting, domain.StateStopping:
		return true
	default:
		return false
	}
}

// busyErr builds the instance_busy error used when the instance already has
// an active job.
func busyErr(instanceID string, active *domain.Job) error {
	return domain.Ef(domain.CodeInstanceBusy, "instance %q already has an active %s job", instanceID, active.Type)
}

func steamCMDMissingErr() error {
	return domain.E(domain.CodeSteamCMDMissing, "steamcmd is not installed")
}

// ---------------------------------------------------------------- install

// EnqueueInstall enqueues a job that installs (or validates) the game files
// via SteamCMD. It refuses when the instance already has an active job, is
// running, or SteamCMD is not installed.
func (j *SteamJobs) EnqueueInstall(ctx context.Context, instanceID, requestedBy string) (*domain.Job, error) {
	if _, err := j.svc.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if active := j.runner.ActiveFor(instanceID); active != nil {
		return nil, busyErr(instanceID, active)
	}
	st, err := j.svc.Status(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if isBusyState(st.State) {
		return nil, domain.Ef(domain.CodeInstanceRunning, "instance %q must be stopped before installing game files", instanceID)
	}
	if !j.client.Installed() {
		return nil, steamCMDMissingErr()
	}

	paths := j.svc.Paths(instanceID)
	fn := func(ctx context.Context, log *jobs.Logger) error {
		if err := os.MkdirAll(paths.Server, 0o750); err != nil {
			return fmt.Errorf("create server directory: %w", err)
		}
		log.Printf("running steamcmd app_update for %s", instanceID)
		if err := j.client.InstallOrUpdate(ctx, paths.Server, log.Writer()); err != nil {
			return err
		}
		buildID, err := j.client.InstalledBuildID(paths.Server)
		if err != nil {
			return fmt.Errorf("read installed build id: %w", err)
		}
		if err := j.svc.SetInstalledBuildID(ctx, instanceID, buildID); err != nil {
			return err
		}
		if err := j.svc.PublishStatus(ctx, instanceID); err != nil {
			j.log.Warn("steamjobs: publish status after install", "instance", instanceID, "err", err)
		}
		log.SetSummary("buildid", buildID)
		log.Printf("install complete: buildid=%s", buildID)
		return nil
	}

	return j.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobInstall,
		InstanceID:  instanceID,
		Title:       "Install game files",
		RequestedBy: requestedBy,
		Exclusive:   true,
	}, fn)
}

// ---------------------------------------------------------------- update

// EnqueueUpdate enqueues a job that updates the game files via SteamCMD,
// optionally stopping the instance first (and starting it again afterwards)
// when stopIfRunning is set. It refuses when the instance already has an
// active job or SteamCMD is not installed; if the instance is running and
// stopIfRunning is false, the job itself fails fast with instance_running
// once it starts (ARCHITECTURE.md §9).
func (j *SteamJobs) EnqueueUpdate(ctx context.Context, instanceID, requestedBy string, stopIfRunning bool) (*domain.Job, error) {
	if _, err := j.svc.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	if active := j.runner.ActiveFor(instanceID); active != nil {
		return nil, busyErr(instanceID, active)
	}
	if !j.client.Installed() {
		return nil, steamCMDMissingErr()
	}

	paths := j.svc.Paths(instanceID)
	fn := func(ctx context.Context, log *jobs.Logger) error {
		st, err := j.svc.Status(ctx, instanceID)
		if err != nil {
			return err
		}
		wasRunning := isBusyState(st.State)
		oldBuildID := st.InstalledBuildID
		if wasRunning && !stopIfRunning {
			return domain.Ef(domain.CodeInstanceRunning, "instance %q is running; retry with stop_if_running", instanceID)
		}

		if wasRunning {
			log.Printf("stopping instance for update")
			if _, err := j.svc.Stop(ctx, instanceID); err != nil {
				return fmt.Errorf("stop instance: %w", err)
			}
			if err := j.waitStopped(ctx, instanceID); err != nil {
				return err
			}
		}

		inst, err := j.svc.Get(ctx, instanceID)
		if err != nil {
			return err
		}
		if inst.Config.BackupBeforeUpdate && j.preUpdateBackup != nil {
			log.Printf("running pre-update backup")
			if err := j.preUpdateBackup(ctx, instanceID, log); err != nil {
				return fmt.Errorf("pre-update backup failed, update aborted: %w", err)
			}
		}

		log.Printf("running steamcmd app_update for %s", instanceID)
		if err := j.client.InstallOrUpdate(ctx, paths.Server, log.Writer()); err != nil {
			return err
		}
		newBuildID, err := j.client.InstalledBuildID(paths.Server)
		if err != nil {
			return fmt.Errorf("read installed build id: %w", err)
		}
		if err := j.svc.SetInstalledBuildID(ctx, instanceID, newBuildID); err != nil {
			return err
		}

		restarted := false
		if wasRunning {
			log.Printf("starting instance after update")
			if _, err := j.svc.Start(ctx, instanceID); err != nil {
				return fmt.Errorf("start instance: %w", err)
			}
			restarted = true
		}
		if err := j.svc.PublishStatus(ctx, instanceID); err != nil {
			j.log.Warn("steamjobs: publish status after update", "instance", instanceID, "err", err)
		}

		log.SetSummary("old_buildid", oldBuildID)
		log.SetSummary("new_buildid", newBuildID)
		log.SetSummary("restarted", restarted)
		log.Printf("update complete: %s -> %s (restarted=%v)", oldBuildID, newBuildID, restarted)
		return nil
	}

	return j.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobUpdate,
		InstanceID:  instanceID,
		Title:       "Update game files",
		RequestedBy: requestedBy,
		Exclusive:   true,
	}, fn)
}

// waitStopped polls Status until instanceID is no longer running/starting/
// stopping, or stopPollTimeout elapses.
func (j *SteamJobs) waitStopped(ctx context.Context, instanceID string) error {
	deadline := time.Now().Add(stopPollTimeout)
	for {
		st, err := j.svc.Status(ctx, instanceID)
		if err != nil {
			return err
		}
		if !isBusyState(st.State) {
			return nil
		}
		if time.Now().After(deadline) {
			return domain.Ef(domain.CodeInternal, "timed out waiting for instance %q to stop", instanceID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stopPollInterval):
		}
	}
}

// ---------------------------------------------------------------- checks

// CheckUpdate queries Steam for the latest public build id now, compares it
// against the instance's installed build id, persists the result and
// publishes the refreshed status.
func (j *SteamJobs) CheckUpdate(ctx context.Context, instanceID string) (*domain.UpdateInfo, error) {
	inst, err := j.svc.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	latest, err := j.latestBuildID(ctx)
	if err != nil {
		de := domain.AsError(err)
		if de.Code == domain.CodeSteamCMDMissing {
			return nil, de
		}
		return nil, domain.Wrap(domain.CodeUpstreamError, "check latest steam build", err)
	}

	installed, err := j.client.InstalledBuildID(inst.Paths.Server)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "read installed build id", err)
	}

	checkedAt := time.Now().UTC()
	if err := j.svc.SetBuildIDs(ctx, instanceID, installed, latest, checkedAt); err != nil {
		return nil, err
	}
	if err := j.svc.PublishStatus(ctx, instanceID); err != nil {
		j.log.Warn("steamjobs: publish status after update check", "instance", instanceID, "err", err)
	}

	return &domain.UpdateInfo{
		InstanceID:       instanceID,
		InstalledBuildID: installed,
		LatestBuildID:    latest,
		UpdateAvailable:  installed != "" && latest != "" && installed != latest,
		CheckedAt:        &checkedAt,
	}, nil
}

// latestBuildID refreshes and returns the latest public build id, preferring
// the shared UpdateChecker (which also refreshes every other instance's
// cached info) when one is configured.
func (j *SteamJobs) latestBuildID(ctx context.Context) (string, error) {
	if j.checker != nil {
		if err := j.checker.CheckNow(ctx); err != nil {
			return "", err
		}
		build, _ := j.client.Latest()
		return build, nil
	}
	return j.client.LatestBuildID(ctx)
}

// SteamCMDInstalled reports whether steamcmd is present and executable.
func (j *SteamJobs) SteamCMDInstalled() bool {
	return j.client != nil && j.client.Installed()
}

// LatestBuildID returns the last known public build id and, when available,
// an UpdateInfo carrying when it was checked (GET /system).
func (j *SteamJobs) LatestBuildID() (string, *domain.UpdateInfo) {
	if j.client == nil {
		return "", nil
	}
	build, checkedAt := j.client.Latest()
	if build == "" {
		return "", nil
	}
	return build, &domain.UpdateInfo{LatestBuildID: build, CheckedAt: &checkedAt}
}
