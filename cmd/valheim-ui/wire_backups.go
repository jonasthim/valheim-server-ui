package main

import (
	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/backup"
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

func wireBackups(deps *api.Deps, inst *instance.Service, runner *jobs.Runner) *backup.Service {
	svc := backup.New(deps.DB, inst, runner, deps.Bus, deps.Version, deps.Log)
	deps.Backups = svc
	return svc
}
