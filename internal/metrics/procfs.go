package metrics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procfsReader reads the Linux /proc files. It is plain file parsing, so it
// compiles everywhere and is unit-tested against fixture directories.
type procfsReader struct {
	root     string
	pageSize int64
}

func (r procfsReader) cpu() (busy, total uint64, err error) {
	b, err := os.ReadFile(filepath.Join(r.root, "stat"))
	if err != nil {
		return 0, 0, fmt.Errorf("read /proc/stat: %w", err)
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return parseCPULine(line)
}

// parseCPULine parses the aggregate "cpu ..." line of /proc/stat.
func parseCPULine(line string) (busy, total uint64, err error) {
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, errors.New("metrics: unexpected /proc/stat format")
	}
	vals := make([]uint64, 0, len(f)-1)
	for _, x := range f[1:] {
		n, err := strconv.ParseUint(x, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("metrics: parse /proc/stat: %w", err)
		}
		vals = append(vals, n)
	}
	for _, v := range vals {
		total += v
	}
	idle := vals[3]
	if len(vals) > 4 {
		idle += vals[4] // iowait counts as idle
	}
	return total - idle, total, nil
}

func (r procfsReader) mem() (total, available int64, err error) {
	b, err := os.ReadFile(filepath.Join(r.root, "meminfo"))
	if err != nil {
		return 0, 0, err
	}
	return parseMeminfo(string(b))
}

// parseMeminfo returns MemTotal and MemAvailable in bytes.
func parseMeminfo(text string) (total, available int64, err error) {
	for _, line := range strings.Split(text, "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		kb, perr := strconv.ParseInt(f[0], 10, 64)
		if perr != nil {
			return 0, 0, fmt.Errorf("metrics: parse meminfo %s: %w", key, perr)
		}
		if key == "MemTotal" {
			total = kb * 1024
		} else {
			available = kb * 1024
		}
	}
	if total == 0 {
		return 0, 0, errors.New("metrics: MemTotal missing")
	}
	return total, available, nil
}

func (r procfsReader) load() (float64, error) {
	b, err := os.ReadFile(filepath.Join(r.root, "loadavg"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("metrics: empty loadavg")
	}
	return strconv.ParseFloat(f[0], 64)
}

func (r procfsReader) procTicks(pid int) (uint64, error) {
	b, err := os.ReadFile(filepath.Join(r.root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	return parseProcStat(string(b))
}

// parseProcStat returns utime+stime (clock ticks) from /proc/<pid>/stat. The
// command name is in parentheses and may itself contain spaces or
// parentheses, so fields are counted from the last ')'.
func parseProcStat(text string) (uint64, error) {
	i := strings.LastIndexByte(text, ')')
	if i < 0 {
		return 0, errors.New("metrics: unexpected /proc/pid/stat format")
	}
	f := strings.Fields(text[i+1:])
	// After ')' the fields are: state(3) ppid pgrp session tty tpgid flags
	// minflt cminflt majflt cmajflt utime(14) stime(15) ...
	if len(f) < 13 {
		return 0, errors.New("metrics: short /proc/pid/stat")
	}
	ut, err := strconv.ParseUint(f[11], 10, 64)
	if err != nil {
		return 0, err
	}
	st, err := strconv.ParseUint(f[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return ut + st, nil
}

func (r procfsReader) procRSS(pid int) (int64, error) {
	b, err := os.ReadFile(filepath.Join(r.root, strconv.Itoa(pid), "statm"))
	if err != nil {
		return 0, err
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0, errors.New("metrics: short /proc/pid/statm")
	}
	pages, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * r.pageSize, nil
}
