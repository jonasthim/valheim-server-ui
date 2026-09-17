package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// unitctlTimeout bounds every `sudo -n unitctl <action> <id>` invocation.
const unitctlTimeout = 150 * time.Second

// systemTimestampLayout matches systemctl's ExecMainStartTimestamp format,
// e.g. "Tue 2024-01-16 20:31:12 UTC".
const systemTimestampLayout = "Mon 2006-01-02 15:04:05 MST"

type systemd struct {
	o Options
}

// NewSystemd returns the production Supervisor: mutations go through
// `sudo -n <unitctl> <action> <id>`, status is read directly from systemd
// (no sudo needed for read-only D-Bus properties). See ARCHITECTURE.md §6.
func NewSystemd(o Options) Supervisor { return &systemd{o: o} }

func (s *systemd) Kind() string { return "systemd" }

func (s *systemd) Start(ctx context.Context, id string) error   { return s.unitctl(ctx, "start", id) }
func (s *systemd) Stop(ctx context.Context, id string) error    { return s.unitctl(ctx, "stop", id) }
func (s *systemd) Restart(ctx context.Context, id string) error { return s.unitctl(ctx, "restart", id) }

func (s *systemd) SetAutostart(ctx context.Context, id string, on bool) error {
	action := "disable"
	if on {
		action = "enable"
	}
	return s.unitctl(ctx, action, id)
}

func (s *systemd) unitctl(ctx context.Context, action, id string) error {
	ctx, cancel := context.WithTimeout(ctx, unitctlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sudoPath(), "-n", s.o.UnitctlPath, action, id)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return domain.Wrap(domain.CodeInternal, fmt.Sprintf("unitctl %s %s failed", action, id), fmt.Errorf("%s", msg))
	}
	return nil
}

func (s *systemd) Status(ctx context.Context, id string) (Status, error) {
	unit := UnitName(id)
	cmd := exec.CommandContext(ctx, systemctlPath(), "show", unit,
		"--property=ActiveState,SubState,MainPID,ExecMainStartTimestamp,Result,UnitFileState,NRestarts,ExecMainCode,ExecMainStatus")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr == "" {
			stderr = err.Error()
		}
		return Status{}, domain.Wrap(domain.CodeInternal, "systemctl show failed", fmt.Errorf("%s", stderr))
	}
	return parseSystemctlShow(string(out)), nil
}

// parseSystemctlShow parses `systemctl show --property=...` output (one
// "Key=Value" pair per line) into a Status. Unknown/empty ActiveState maps to
// stopped; a timestamp that fails to parse maps to a zero time.Time.
func parseSystemctlShow(out string) Status {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[key] = val
	}

	var state State
	switch props["ActiveState"] {
	case "active":
		state = StateRunning
	case "activating":
		state = StateStarting
	case "deactivating":
		state = StateStopping
	case "failed":
		state = StateFailed
	case "inactive", "":
		state = StateStopped
	default:
		state = StateStopped
	}

	pid, _ := strconv.Atoi(props["MainPID"])

	var since time.Time
	if ts := strings.TrimSpace(props["ExecMainStartTimestamp"]); ts != "" {
		if t, err := time.Parse(systemTimestampLayout, ts); err == nil {
			since = t
		}
	}

	detail := ""
	if state == StateFailed {
		detail = props["Result"]
	}
	// SubState=auto-restart means systemd is between a crash and its next
	// start attempt (ActiveState is "activating" at that point, not
	// "failed"), so it needs its own Detail source alongside Result above.
	if props["SubState"] == "auto-restart" {
		detail = "auto-restart"
	}

	restarts, _ := strconv.Atoi(props["NRestarts"]) // parse error (missing/garbage) -> 0

	exitDetail := buildExitDetail(props["ExecMainCode"], props["ExecMainStatus"])

	switch props["UnitFileState"] {
	case "enabled", "enabled-runtime", "static":
		return Status{State: state, PID: pid, Since: since, Autostart: true, Detail: detail,
			Restarts: restarts, ExitDetail: exitDetail}
	default:
		return Status{State: state, PID: pid, Since: since, Autostart: false, Detail: detail,
			Restarts: restarts, ExitDetail: exitDetail}
	}
}

// buildExitDetail renders systemd's ExecMainCode/ExecMainStatus properties
// (the last time the unit's main process exited, sticky until the next exit)
// as "exit status <n>" for a non-zero plain exit, or "signal <n>" for a
// signal-terminated (killed) or core-dumped exit. Anything else (never
// exited, or a clean "exited" with status 0) reports "".
func buildExitDetail(execMainCode, execMainStatus string) string {
	switch execMainCode {
	case "exited":
		if execMainStatus != "" && execMainStatus != "0" {
			return "exit status " + execMainStatus
		}
	case "killed", "dumped":
		if execMainStatus != "" {
			return "signal " + execMainStatus
		}
	}
	return ""
}

// sudoPath and systemctlPath resolve the two host binaries once, from the
// fixed system directories only, so a PATH entry the service user can write
// to can never substitute them.
var (
	sudoOnce, systemctlOnce sync.Once
	sudoBin, systemctlBin   string
)

func fixedPathLookup(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		p := dir + "/" + name
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return name // falls back to PATH lookup (tests, unusual layouts)
}

func sudoPath() string {
	sudoOnce.Do(func() { sudoBin = fixedPathLookup("sudo") })
	return sudoBin
}

func systemctlPath() string {
	systemctlOnce.Do(func() { systemctlBin = fixedPathLookup("systemctl") })
	return systemctlBin
}
