// Package remote copies a completed backup zip to an off-box target: a local
// path (e.g. a mounted network share) or any remote the external rclone
// binary supports (F-1.4, docs/WORKPLAN.md). It knows nothing about the
// backups table or jobs; internal/backup.Service drives it and records the
// result via SetRemoteStatus.
package remote

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Uploader copies zipPath (a backup already written to disk) to target,
// under instanceID, so different instances never collide inside a shared
// target. Implementations must honour ctx cancellation.
type Uploader interface {
	Upload(ctx context.Context, target domain.BackupTarget, zipPath, instanceID string) error
}

// Dispatcher implements Uploader by picking a concrete implementation from
// target.Type.
type Dispatcher struct {
	rclonePath string
}

// New builds a Dispatcher. rclonePath is the resolved absolute path to the
// rclone binary, or "" when it is not installed on the manager host (an
// rclone-type upload then fails with a clear error instead of exec'ing
// nothing).
func New(rclonePath string) *Dispatcher {
	return &Dispatcher{rclonePath: rclonePath}
}

// Upload dispatches to the implementation for target.Type.
func (d *Dispatcher) Upload(ctx context.Context, target domain.BackupTarget, zipPath, instanceID string) error {
	switch target.Type {
	case domain.BackupTargetLocal:
		return uploadLocal(target, zipPath, instanceID)
	case domain.BackupTargetRclone:
		return uploadRclone(ctx, d.rclonePath, target, zipPath, instanceID)
	default:
		return fmt.Errorf("unknown backup target type %q", target.Type)
	}
}
