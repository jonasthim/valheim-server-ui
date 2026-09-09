package backup

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// backupTimeFormat is the UTC timestamp segment of a backup filename
// (ARCHITECTURE.md §3: "<id>-<world>-<UTC timestamp>-<kind>.zip").
const backupTimeFormat = "20060102-150405"

// sanitizeNameComponent maps a value (e.g. a world name, which may contain
// spaces) onto a safe filename component.
func sanitizeNameComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "world"
	}
	return out
}

// backupFilename renders the canonical backup zip name.
func backupFilename(instanceID, world string, ts time.Time, kind domain.BackupKind) string {
	return fmt.Sprintf("%s-%s-%s-%s.zip", instanceID, sanitizeNameComponent(world), ts.UTC().Format(backupTimeFormat), kind)
}

var backupKindNames = []string{
	string(domain.BackupManual),
	string(domain.BackupScheduled),
	string(domain.BackupPreUpdate),
	string(domain.BackupPreRestore),
	string(domain.BackupUploaded),
}

// middlePattern splits "<world>-<timestamp>" back apart; greedy .+ lets
// world contain hyphens as long as the trailing 8-digit-6-digit timestamp
// shape is unambiguous.
var middlePattern = regexp.MustCompile(`^(.+)-(\d{8}-\d{6})$`)

// parseBackupFilename recovers (kind, world) from a filename matching
// backupFilename's shape, for List's reconciliation of files that were not
// created through this service (hand-copied backups, restores of an older
// naming scheme, etc). Anything that does not match is reported as an
// unrecognised manual backup of an unknown world rather than dropped.
func parseBackupFilename(instanceID, filename string) (domain.BackupKind, string) {
	if !strings.HasSuffix(filename, ".zip") {
		return domain.BackupManual, "unknown"
	}
	name := strings.TrimSuffix(filename, ".zip")
	prefix := instanceID + "-"
	if !strings.HasPrefix(name, prefix) {
		return domain.BackupManual, "unknown"
	}
	rest := strings.TrimPrefix(name, prefix)
	for _, k := range backupKindNames {
		suffix := "-" + k
		if !strings.HasSuffix(rest, suffix) {
			continue
		}
		middle := strings.TrimSuffix(rest, suffix)
		if m := middlePattern.FindStringSubmatch(middle); m != nil {
			return domain.BackupKind(k), m[1]
		}
	}
	return domain.BackupManual, "unknown"
}
