package supervisor

import (
	"fmt"
	"log/slog"
)

// Options are the inputs the implementations need.
type Options struct {
	// SelfPath is the absolute path of the valheim-ui binary (for `launch`).
	SelfPath string
	// InstancesDir is <data_dir>/instances.
	InstancesDir string
	// UnitctlPath is the sudo wrapper (systemd only).
	UnitctlPath string
	Log         *slog.Logger
}

// New returns the supervisor for kind ("systemd" | "direct").
func New(kind string, o Options) (Supervisor, error) {
	switch kind {
	case "systemd":
		return NewSystemd(o), nil
	case "direct":
		return NewDirect(o), nil
	default:
		return nil, fmt.Errorf("unknown supervisor kind %q", kind)
	}
}
