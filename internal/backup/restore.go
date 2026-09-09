package backup

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// EnqueueRestore restores a backup as a job. The job itself (not this
// enqueue call, mirroring internal/instance/jobs.go's update job) stops the
// instance first and waits for it to actually reach stopped when
// stopIfRunning is set -- or fails fast with instance_running if it is
// running and stopIfRunning was not requested -- creates a pre_restore
// backup of whatever is currently in place, extracts the chosen backup over
// the save directory, switches the active world if the backup was for a
// different one, and restarts the instance if it was running
// (ARCHITECTURE.md §10).
func (s *Service) EnqueueRestore(ctx context.Context, instanceID string, backupID int64, stopIfRunning bool, requestedBy string) (*domain.Job, error) {
	if _, err := s.inst.Get(ctx, instanceID); err != nil {
		return nil, err
	}
	backupRow, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		return nil, err
	}

	return s.runner.Enqueue(ctx, jobs.Spec{
		Type:        domain.JobRestore,
		InstanceID:  instanceID,
		Title:       fmt.Sprintf("Restore backup %s", backupRow.Filename),
		RequestedBy: requestedBy,
	}, func(ctx context.Context, log *jobs.Logger) error {
		return s.runRestore(ctx, instanceID, backupID, stopIfRunning, log)
	})
}

func (s *Service) runRestore(ctx context.Context, instanceID string, backupID int64, stopIfRunning bool, log *jobs.Logger) error {
	inst, err := s.inst.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	backupRow, err := s.getBackupRow(ctx, instanceID, backupID)
	if err != nil {
		return err
	}

	wasRunning := isBusyState(inst.Status.State)
	if wasRunning && !stopIfRunning {
		return domain.Ef(domain.CodeInstanceRunning, "instance %q is running; retry with stop_if_running", instanceID)
	}
	if wasRunning {
		log.Printf("stopping instance before restore")
		if _, err := s.inst.Stop(ctx, instanceID); err != nil {
			return fmt.Errorf("stop instance: %w", err)
		}
		if err := s.waitStopped(ctx, instanceID); err != nil {
			return err
		}
	}

	log.Printf("creating pre-restore backup")
	pre, err := s.Create(ctx, instanceID, domain.BackupPreRestore, "before restoring "+backupRow.Filename)
	if err != nil {
		return fmt.Errorf("pre-restore backup: %w", err)
	}
	log.SetSummary("pre_restore_backup", pre.Filename)

	paths := s.inst.Paths(instanceID)
	zipPath := filepath.Join(paths.Backups, backupRow.Filename)
	world, err := extractBackupZip(zipPath, paths.Save)
	if err != nil {
		return fmt.Errorf("extract backup %s: %w", backupRow.Filename, err)
	}
	log.Printf("restored world %q from %s", world, backupRow.Filename)
	log.SetSummary("restored_world", world)

	if world != "" && world != inst.Config.World {
		newCfg := inst.Config
		newCfg.World = world
		if _, err := s.inst.Update(ctx, instanceID, nil, &newCfg, nil); err != nil {
			return fmt.Errorf("switch active world to %q: %w", world, err)
		}
		log.Printf("active world switched to %q", world)
	}

	if wasRunning {
		log.Printf("starting instance after restore")
		if _, err := s.inst.Start(ctx, instanceID); err != nil {
			return fmt.Errorf("start instance: %w", err)
		}
	}
	if err := s.inst.PublishStatus(ctx, instanceID); err != nil {
		s.log.Warn("backup: publish status after restore", "instance", instanceID, "err", err)
	}
	return nil
}

// waitStopped polls the instance's status every stopPollInterval until it is
// no longer running/starting/stopping, or stopPollTimeout elapses.
func (s *Service) waitStopped(ctx context.Context, instanceID string) error {
	deadline := time.Now().Add(stopPollTimeout)
	for {
		st, err := s.inst.Status(ctx, instanceID)
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
