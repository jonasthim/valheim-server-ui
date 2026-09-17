package instance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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

// Kinds for domain.LogFileInfo.Kind (F-2.6).
const (
	logKindConsole = "console"
	logKindRotated = "rotated"
	logKindBepInEx = "bepinex"
)

// bepInExLogName is the log BepInEx itself writes under BepInExDir().
const bepInExLogName = "LogOutput.log"

// scanBufferSize is the max single-line size tailReader/searchOneFile will
// buffer (bufio.Scanner's default 64 KiB token limit is too small for some
// BepInEx log lines).
const scanBufferSize = 1024 * 1024

// logFile pairs a domain.LogFileInfo with its absolute path on disk. Kept
// unexported so a caller can only ever open a file by looking its name up in
// a list this package produced, never by re-joining a name onto a directory
// itself (ARCHITECTURE.md security rule: a user-supplied name is a key into
// what we listed, not a path component).
type logFile struct {
	Info domain.LogFileInfo
	Path string
}

// listLogFiles enumerates every log file available for an instance (F-2.6):
// the live console.log, rotated console-<ts>.log files, and BepInEx's
// LogOutput.log, each included only if present. Ordered console first, then
// rotated newest-modified first, then bepinex, matching the order an
// operator most likely wants (also the order SearchLogs scans in).
func listLogFiles(paths domain.InstancePaths) ([]logFile, error) {
	var out []logFile

	if fi, err := os.Stat(paths.ConsoleLog()); err == nil {
		out = append(out, logFile{
			Info: domain.LogFileInfo{Name: "console.log", SizeBytes: fi.Size(), ModifiedAt: fi.ModTime().UTC(), Kind: logKindConsole},
			Path: paths.ConsoleLog(),
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat console.log: %w", err)
	}

	entries, err := os.ReadDir(paths.Logs)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read logs dir: %w", err)
	}
	var rotated []logFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "console-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rotated = append(rotated, logFile{
			Info: domain.LogFileInfo{Name: name, SizeBytes: info.Size(), ModifiedAt: info.ModTime().UTC(), Kind: logKindRotated},
			Path: filepath.Join(paths.Logs, name),
		})
	}
	sort.SliceStable(rotated, func(i, j int) bool {
		return rotated[i].Info.ModifiedAt.After(rotated[j].Info.ModifiedAt)
	})
	out = append(out, rotated...)

	bepinexLog := filepath.Join(paths.BepInExDir(), bepInExLogName)
	if fi, err := os.Stat(bepinexLog); err == nil {
		out = append(out, logFile{
			Info: domain.LogFileInfo{Name: bepInExLogName, SizeBytes: fi.Size(), ModifiedAt: fi.ModTime().UTC(), Kind: logKindBepInEx},
			Path: bepinexLog,
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", bepInExLogName, err)
	}

	return out, nil
}

// searchLogFiles scans files (in the order given, newest-first per
// listLogFiles) for q, stopping once limit matches have been collected.
// Within each file only the most recent matches are kept, newest line first;
// a file's matches are always returned before the next file's, so the result
// is newest-first both across files and within one. Plain mode is a
// case-insensitive substring match; useRegex compiles q as a regexp. An
// empty q or an invalid regexp is a validation error on field "q".
func searchLogFiles(files []logFile, q string, useRegex bool, limit int) ([]domain.LogMatch, error) {
	if q == "" {
		return nil, domain.Validation([]domain.FieldError{{Field: "q", Message: "must not be empty"}})
	}
	var re *regexp.Regexp
	if useRegex {
		var err error
		re, err = regexp.Compile(q)
		if err != nil {
			return nil, domain.Validation([]domain.FieldError{{Field: "q", Message: "invalid regular expression: " + err.Error()}})
		}
	}
	needle := strings.ToLower(q)
	matchLine := func(line string) bool {
		if re != nil {
			return re.MatchString(line)
		}
		return strings.Contains(strings.ToLower(line), needle)
	}

	var out []domain.LogMatch
	for _, f := range files {
		remaining := limit - len(out)
		if remaining <= 0 {
			break
		}
		fileMatches, err := searchOneFile(f, matchLine, remaining)
		if err != nil {
			return nil, err
		}
		out = append(out, fileMatches...)
	}
	return out, nil
}

// searchOneFile scans f for lines matchLine accepts, keeping only the most
// recent (up to) remaining matches, newest first. A missing file yields no
// matches, not an error (a rotated log could be pruned mid-search).
func searchOneFile(f logFile, matchLine func(string) bool, remaining int) ([]domain.LogMatch, error) {
	file, err := os.Open(f.Path) //nolint:gosec // f.Path came from listLogFiles, not user input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", f.Path, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), scanBufferSize)
	var kept []string
	for scanner.Scan() {
		line := scanner.Text()
		if !matchLine(line) {
			continue
		}
		kept = append(kept, line)
		if len(kept) > remaining {
			kept = kept[1:] // trim the oldest kept match, keep the most recent `remaining`
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", f.Path, err)
	}

	out := make([]domain.LogMatch, len(kept))
	for i, line := range kept {
		out[len(kept)-1-i] = domain.LogMatch{File: f.Info.Name, Line: line} // reverse: newest (last scanned) first
	}
	return out, nil
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
