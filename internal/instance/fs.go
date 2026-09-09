package instance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// maxConsoleLogSize is the rotation threshold (ARCHITECTURE.md §8).
const maxConsoleLogSize = 20 * 1024 * 1024

// keepRotatedLogs is how many rotated console-<ts>.log files survive rotation.
const keepRotatedLogs = 5

// createTree makes the on-disk layout for a new instance (ARCHITECTURE.md §3).
func createTree(paths domain.InstancePaths) error {
	dirs := []string{paths.Root, paths.Server, paths.Save, paths.WorldsDir(), paths.Backups, paths.Logs}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return fmt.Errorf("create %s: %w", d, err)
		}
	}
	return nil
}

// renderLaunch writes launch.json atomically (0600), see ARCHITECTURE.md §7.
func renderLaunch(paths domain.InstancePaths, id string, cfg domain.InstanceConfig) error {
	launch := domain.Launch{
		Version:    domain.LaunchVersion,
		InstanceID: id,
		ServerDir:  paths.Server,
		LogDir:     paths.Logs,
		Args:       cfg.BuildArgs(paths.Save),
		BepInEx:    cfg.BepInExEnabled,
	}
	data, err := json.MarshalIndent(launch, "", "  ")
	if err != nil {
		return fmt.Errorf("encode launch.json: %w", err)
	}
	final := paths.LaunchFile()
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write launch.json: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename launch.json: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// rotateConsoleLog renames console.log to console-<UTC ts>.log when it
// exceeds maxConsoleLogSize and prunes old rotated files beyond
// keepRotatedLogs. Must only be called while the instance is not running
// (ARCHITECTURE.md §8).
func rotateConsoleLog(paths domain.InstancePaths) error {
	logPath := paths.ConsoleLog()
	fi, err := os.Stat(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat console.log: %w", err)
	}
	if fi.Size() <= maxConsoleLogSize {
		return nil
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	rotated := filepath.Join(paths.Logs, fmt.Sprintf("console-%s.log", ts))
	if err := os.Rename(logPath, rotated); err != nil {
		return fmt.Errorf("rotate console.log: %w", err)
	}
	return pruneRotatedLogs(paths.Logs)
}

func pruneRotatedLogs(logsDir string) error {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fmt.Errorf("read logs dir: %w", err)
	}
	var rotated []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "console-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		rotated = append(rotated, name)
	}
	sort.Strings(rotated) // the timestamp format sorts lexically == chronologically
	if len(rotated) <= keepRotatedLogs {
		return nil
	}
	for _, name := range rotated[:len(rotated)-keepRotatedLogs] {
		_ = os.Remove(filepath.Join(logsDir, name))
	}
	return nil
}

// tailFile returns the last n lines of path, reading backward from the end in
// 64 KiB chunks so it never loads more of the file than necessary. A missing
// file returns an empty slice, not an error.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	const chunkSize = 64 * 1024
	var buf []byte
	pos := fi.Size()
	for pos > 0 && bytes.Count(buf, []byte("\n")) <= n {
		readSize := int64(chunkSize)
		if readSize > pos {
			readSize = pos
		}
		pos -= readSize
		chunk := make([]byte, readSize)
		if _, err := f.ReadAt(chunk, pos); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		buf = append(chunk, buf...)
	}

	text := strings.TrimRight(string(buf), "\n")
	if text == "" {
		return []string{}, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// removeTree deletes an instance directory, refusing anything outside
// instancesDir (defence in depth against a bad id ever reaching here).
func removeTree(instancesDir, path string) error {
	instancesAbs, err := filepath.Abs(instancesDir)
	if err != nil {
		return fmt.Errorf("resolve instances dir: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve instance dir: %w", err)
	}
	rel, err := filepath.Rel(instancesAbs, pathAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to delete %s: outside %s", pathAbs, instancesAbs)
	}
	if err := os.RemoveAll(pathAbs); err != nil {
		return fmt.Errorf("remove %s: %w", pathAbs, err)
	}
	return nil
}
