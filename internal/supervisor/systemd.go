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
		"--property=ActiveState,SubState,MainPID,ExecMainStartTimestamp,Result,UnitFileState")
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

	switch props["UnitFileState"] {
	case "enabled", "enabled-runtime", "static":
		return Status{State: state, PID: pid, Since: since, Autostart: true, Detail: detail}
	default:
		return Status{State: state, PID: pid, Since: since, Autostart: false, Detail: detail}
	}
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
