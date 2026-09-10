//go:build !windows

package launcher

import "testing"

// On Linux doorstop is driven by the environment alone; arguments pass through.
func TestPlatformDoorstopPassesArgsThrough(t *testing.T) {
	env, args := platformDoorstop(domainLaunch(t.TempDir(), true, []string{"-name", "x"}), []string{"A=1"})
	if !equal(args, []string{"-name", "x"}) || !equal(env, []string{"A=1"}) {
		t.Fatalf("got args %q env %q", args, env)
	}
}
