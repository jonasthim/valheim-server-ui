package selfupdate

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// probeTTL is how long a unitctl capability probe is cached; the installer
// may upgrade the wrapper while the manager runs, so it is re-checked
// periodically rather than once at startup.
const probeTTL = time.Minute

// NewPrivilegedProbe returns a function reporting whether the sudo wrapper at
// unitctlPath supports apply-upgrade (root-owned binary layout). It always
// reports false when enabled is false (direct supervisor, development).
func NewPrivilegedProbe(unitctlPath string, enabled bool) func() bool {
	if !enabled || unitctlPath == "" {
		return func() bool { return false }
	}
	var (
		mu      sync.Mutex
		checked time.Time
		result  bool
	)
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		if !checked.IsZero() && time.Since(checked) < probeTTL {
			return result
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/usr/bin/sudo", "-n", unitctlPath, "capabilities").Output() //nolint:gosec // fixed sudo path, wrapper path from config
		result = err == nil && strings.Contains(string(out), "apply-upgrade")
		checked = time.Now()
		return result
	}
}
