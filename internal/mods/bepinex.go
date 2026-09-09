package mods

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

// stopWaitTimeout bounds how long a job waits for the instance to actually
// reach "stopped" after asking the supervisor to stop it
// (ARCHITECTURE.md §9 / WORKPLAN.md WP-08).
const stopWaitTimeout = 150 * time.Second

const stopPollInterval = 500 * time.Millisecond

// packInfo is the JSON written to server/BepInEx/valheim-ui-pack.json on
// install (ARCHITECTURE.md §12).
type packInfo struct {
	Owner       string    `json:"owner"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
}

func packInfoPath(paths domain.InstancePaths) string {
	return filepath.Join(paths.BepInExDir(), "valheim-ui-pack.json")
}

// bepinexInstalled reports whether the loader is present.
func bepinexInstalled(paths domain.InstancePaths) bool {
	return fileExists(filepath.Join(paths.BepInExDir(), "core", "BepInEx.Preloader.dll"))
}

func readPackInfo(paths domain.InstancePaths) (*packInfo, error) {
	data, err := os.ReadFile(packInfoPath(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pack info: %w", err)
	}
	var pi packInfo
	if err := json.Unmarshal(data, &pi); err != nil {
		return nil, fmt.Errorf("parse pack info: %w", err)
	}
	return &pi, nil
}

func writePackInfo(paths domain.InstancePaths, pi packInfo) error {
	data, err := json.MarshalIndent(pi, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pack info: %w", err)
	}
	return writeFileAtomic(packInfoPath(paths), data, 0o640)
}

// bepinexStatus composes the ModsOverview.BepInEx block.
func bepinexStatus(paths domain.InstancePaths, cfgEnabled bool, ts *Thunderstore) domain.BepInExStatus {
	st := domain.BepInExStatus{
		Installed: bepinexInstalled(paths),
		Enabled:   cfgEnabled,
	}
	if pi, err := readPackInfo(paths); err == nil && pi != nil {
		st.Version = pi.Version
	}
	if latest, ok := ts.LatestVersion(domain.BepInExOwner, domain.BepInExName); ok {
		st.LatestVersion = latest
	}
	return st
}

// extractBepInExPack copies the payload of the BepInEx pack zip (wrapped in a
// single top-level "BepInExPack_Valheim/" folder) into serverDir's root,
// overwriting core loader files but never an existing BepInEx/config/*.cfg or
// anything already present under BepInEx/plugins/ (ARCHITECTURE.md §12).
func extractBepInExPack(zipPath, serverDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open bepinex pack zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	names := make([]string, 0, len(r.File))
	cleaned := make(map[string]string, len(r.File))
	for _, f := range r.File {
		clean, err := safeZipEntryPath(f.Name)
		if err != nil {
			return err
		}
		cleaned[f.Name] = clean
		if !f.FileInfo().IsDir() {
			names = append(names, clean)
		}
	}
	// The payload is whatever directory contains BepInEx/core/BepInEx.Preloader.dll.
	// Real packs ship it under "BepInExPack_Valheim/" with Thunderstore metadata
	// (manifest.json, README.md, icon.png, CHANGELOG.md) beside it at the archive
	// root; those metadata files are skipped. A pack with the payload at the root
	// (prefix "") is accepted too.
	prefix, ok := packPayloadPrefix(names)
	if !ok {
		return fmt.Errorf("bepinex pack zip does not contain BepInEx/core/BepInEx.Preloader.dll")
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rel := cleaned[f.Name]
		if prefix != "" {
			if !strings.HasPrefix(rel, prefix) {
				continue
			}
			rel = strings.TrimPrefix(rel, prefix)
		} else if !strings.Contains(rel, "/") && isPackMetadata(rel) {
			continue
		}
		if rel == "" {
			continue
		}

		destAbs := filepath.Join(serverDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(filepath.Clean(destAbs), filepath.Clean(serverDir)+string(filepath.Separator)) {
			return fmt.Errorf("zip-slip: bepinex pack entry escapes server dir: %q", rel)
		}

		if strings.HasPrefix(rel, "BepInEx/config/") && fileExists(destAbs) {
			continue
		}
		if strings.HasPrefix(rel, "BepInEx/plugins/") && fileExists(destAbs) {
			continue
		}
		if err := extractOne(f, destAbs); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

// waitStopped polls status until the instance reports stopped/not-installed
// or stopWaitTimeout elapses.
func waitStopped(ctx context.Context, inst InstanceAccessor, instanceID string) error {
	deadline := time.Now().Add(stopWaitTimeout)
	for {
		st, err := inst.Status(ctx, instanceID)
		if err != nil {
			return err
		}
		if st.State == domain.StateStopped || st.State == domain.StateNotInstalled || st.State == domain.StateFailed {
			return nil
		}
		if time.Now().After(deadline) {
			return domain.Ef(domain.CodeInternal, "instance %q did not stop within %s", instanceID, stopWaitTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stopPollInterval):
		}
	}
}

// withStoppedInstance stops instanceID if it is running (or refuses with
// instance_running if stopIfRunning is false), runs work, and restarts it
// afterwards if it was running before. Mirrors ARCHITECTURE.md §9's
// stop_if_running contract shared by update/restore/world_import/bepinex_install.
func withStoppedInstance(ctx context.Context, inst InstanceAccessor, instanceID string, log *jobs.Logger, work func(ctx context.Context) error) error {
	st, err := inst.Status(ctx, instanceID)
	if err != nil {
		return err
	}
	wasRunning := st.State == domain.StateRunning || st.State == domain.StateStarting
	if wasRunning {
		log.Printf("stopping instance before mod operation")
		if _, err := inst.Stop(ctx, instanceID); err != nil {
			return fmt.Errorf("stop instance: %w", err)
		}
		if err := waitStopped(ctx, inst, instanceID); err != nil {
			return err
		}
	}

	workErr := work(ctx)

	if wasRunning {
		log.Printf("restarting instance")
		if _, err := inst.Start(ctx, instanceID); err != nil {
			if workErr == nil {
				return fmt.Errorf("restart instance: %w", err)
			}
			log.Printf("restart instance failed: %v", err)
		}
	}
	return workErr
}

const packPreloader = "BepInEx/core/BepInEx.Preloader.dll"

// packPayloadPrefix returns the directory prefix (with trailing slash, or "")
// under which the BepInEx payload lives inside a pack zip.
func packPayloadPrefix(names []string) (string, bool) {
	for _, n := range names {
		if n == packPreloader {
			return "", true
		}
		if strings.HasSuffix(n, "/"+packPreloader) {
			return strings.TrimSuffix(n, packPreloader), true
		}
	}
	return "", false
}

func isPackMetadata(name string) bool {
	switch name {
	case "manifest.json", "README.md", "CHANGELOG.md", "icon.png":
		return true
	}
	return false
}
