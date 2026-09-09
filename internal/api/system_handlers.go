package api

import (
	"fmt"
	"net/http"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// registerSystemRoutes mounts GET /system (docs/openapi.yaml → SystemInfo).
func registerSystemRoutes(r chi.Router, d *Deps) {
	r.With(RequireRole(domain.RoleViewer)).Get("/system", systemHandler(d))
}

// systemInfo mirrors the SystemInfo schema.
type systemInfo struct {
	Version           string     `json:"version"`
	Commit            string     `json:"commit,omitempty"`
	DataDir           string     `json:"data_dir"`
	Supervisor        string     `json:"supervisor"`
	SteamCMDInstalled bool       `json:"steamcmd_installed"`
	DiskFreeBytes     int64      `json:"disk_free_bytes"`
	DiskTotalBytes    int64      `json:"disk_total_bytes"`
	StartedAt         time.Time  `json:"started_at"`
	LatestBuildID     string     `json:"latest_buildid,omitempty"`
	BuildIDCheckedAt  *time.Time `json:"buildid_checked_at,omitempty"`
}

func systemHandler(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info := systemInfo{
			Version:    d.Version,
			Commit:     d.Commit,
			DataDir:    d.Cfg.DataDir,
			Supervisor: d.Cfg.Supervisor,
			StartedAt:  d.StartedAt,
		}
		if d.Steam != nil {
			info.SteamCMDInstalled = d.Steam.SteamCMDInstalled()
			if build, upd := d.Steam.LatestBuildID(); build != "" {
				info.LatestBuildID = build
				if upd != nil {
					info.BuildIDCheckedAt = upd.CheckedAt
				}
			}
		}
		free, total, err := diskUsage(d.Cfg.DataDir)
		if err != nil {
			d.Log.Warn("system: disk usage", "data_dir", d.Cfg.DataDir, "err", err)
		}
		info.DiskFreeBytes = free
		info.DiskTotalBytes = total

		WriteJSON(w, http.StatusOK, info)
	}
}

// diskUsage returns the free/total bytes of the filesystem containing path.
func diskUsage(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	//nolint:gosec // Bsize/Bavail/Blocks are unsigned on some platforms; disk sizes fit comfortably in int64.
	free = int64(stat.Bavail) * stat.Bsize
	//nolint:gosec // see above
	total = int64(stat.Blocks) * stat.Bsize
	return free, total, nil
}
