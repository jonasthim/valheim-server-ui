package main

import (
	"context"
	"os/exec"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/backup"
	"github.com/jonasthim/valheim-server-ui/internal/backup/remote"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// wireBackups constructs the backup service (WP-06) and attaches it to deps.
// It returns the concrete *backup.Service so wire.go can also plug its
// Service.PreUpdateBackup method into WP-05's instance.NewSteamJobs as the
// instance.PreUpdateBackupFunc.
//
// wire.go calls this from wireServices after deps.DB, deps.Bus, deps.Log and
// deps.Version are set and after the instance service (WP-02) and job runner
// (WP-04) exist.
//
// It also wires F-1.4's off-site copy: an Uploader that dispatches to a
// local-path copy or the external rclone binary (never a new Go dependency,
// per CLAUDE.md), and a targets source reading Settings.Backups.Targets. Both
// closures read deps.Settings lazily, so they work regardless of whether
// wireAuth (which sets deps.Settings) has already run when this is called.
func wireBackups(deps *api.Deps, inst *instance.Service, runner *jobs.Runner) *backup.Service {
	svc := backup.New(deps.DB, inst, runner, deps.Bus, deps.Version, deps.Log)
	deps.Backups = svc

	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		rclonePath = "" // not installed; remote.Dispatcher reports this on use
	}
	svc.SetUploader(remote.New(rclonePath))
	svc.SetTargets(func(ctx context.Context) ([]domain.BackupTarget, error) {
		if deps.Settings == nil {
			return nil, nil
		}
		s, err := deps.Settings.Get(ctx)
		if err != nil {
			return nil, err
		}
		return s.Backups.Targets, nil
	})

	return svc
}
