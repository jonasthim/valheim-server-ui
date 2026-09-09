// Package launcher implements `valheim-ui launch --instance ID`, the process
// systemd (or the direct supervisor) execs to start one Valheim dedicated
// server. See docs/ARCHITECTURE.md §7.
package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/jonasthim/valheim-server-ui/internal/config"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// execFunc mirrors syscall.Exec. Overridable in tests so the exec step can be
// verified without exec-ing over the calling process.
type execFunc func(argv0 string, argv []string, envv []string) error

// Run reads <instances>/<instanceID>/launch.json, prepares the environment,
// and execs the game server binary (or the fake server, when cfg.FakeServer).
// On success it never returns: the calling process image is replaced.
func Run(ctx context.Context, cfg config.Config, instanceID string) error {
	return run(ctx, cfg, instanceID, syscall.Exec)
}

func run(ctx context.Context, cfg config.Config, instanceID string, execFn execFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if instanceID == "" {
		return errors.New("launcher: instance id is required")
	}

	paths := domain.PathsFor(cfg.InstancesDir(), instanceID)
	launch, err := loadLaunch(paths.LaunchFile())
	if err != nil {
		return err
	}

	// Resolve the fake server path relative to the process's current
	// directory *before* we chdir into the server directory.
	var fakePath string
	if cfg.FakeServer {
		fakePath, err = resolveFakePath(cfg.FakeServerPath)
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(launch.LogDir, 0o750); err != nil {
		return fmt.Errorf("launcher: create log dir: %w", err)
	}

	if err := os.Chdir(launch.ServerDir); err != nil {
		return fmt.Errorf("launcher: chdir %s: %w", launch.ServerDir, err)
	}

	env := baseEnv(os.Environ())
	if launch.BepInEx {
		env, err = applyBepInExEnv(launch.ServerDir, env)
		if err != nil {
			return err
		}
	}

	binPath := "./valheim_server.x86_64"
	if cfg.FakeServer {
		binPath = fakePath
	}
	argv := append([]string{binPath}, launch.Args...)

	fmt.Println(maskedLaunchLine(binPath, launch.Args))

	return execFn(binPath, argv, env)
}

func loadLaunch(path string) (domain.Launch, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from domain.PathsFor(cfg.InstancesDir(), instanceID), never raw user input
	if err != nil {
		return domain.Launch{}, fmt.Errorf("launcher: read %s: %w", path, err)
	}
	var l domain.Launch
	if err := json.Unmarshal(data, &l); err != nil {
		return domain.Launch{}, fmt.Errorf("launcher: parse %s: %w", path, err)
	}
	if l.ServerDir == "" || l.LogDir == "" {
		return domain.Launch{}, fmt.Errorf("launcher: %s: missing server_dir/log_dir", path)
	}
	return l, nil
}

// resolveFakePath makes p absolute against the current working directory (the
// manager's cwd at process start, not the instance server dir).
func resolveFakePath(p string) (string, error) {
	if p == "" {
		return "", errors.New("launcher: fake_server_path is not configured")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("launcher: resolve fake_server_path: %w", err)
	}
	return abs, nil
}

// baseEnv is process env + SteamAppId + LD_LIBRARY_PATH=./linux64:$LD_LIBRARY_PATH
// (ARCHITECTURE.md §7 step 1).
func baseEnv(environ []string) []string {
	order, m := envToOrdered(environ)
	if _, exists := m["LD_LIBRARY_PATH"]; !exists {
		order = append(order, "LD_LIBRARY_PATH")
	}
	m["LD_LIBRARY_PATH"] = "./linux64:" + m["LD_LIBRARY_PATH"]
	if _, exists := m["SteamAppId"]; !exists {
		order = append(order, "SteamAppId")
	}
	m["SteamAppId"] = domain.SteamGameAppID
	return orderedToEnv(order, m)
}

// applyBepInExEnv reads start_server_bepinex.sh from serverDir (an absolute
// path) and applies its doorstop exports, or the built-in defaults when the
// script is missing.
func applyBepInExEnv(serverDir string, env []string) ([]string, error) {
	scriptPath := filepath.Join(serverDir, "start_server_bepinex.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultBepInExEnv(env)
		}
		return nil, fmt.Errorf("launcher: read %s: %w", scriptPath, err)
	}
	return ResolveBepInExEnv(content, env)
}

// maskedLaunchLine renders the one-line startup banner with the -password
// value replaced.
func maskedLaunchLine(binPath string, args []string) string {
	masked := make([]string, len(args))
	copy(masked, args)
	for i, a := range masked {
		if a == "-password" && i+1 < len(masked) {
			masked[i+1] = "********"
		}
	}
	return fmt.Sprintf("[valheim-ui] launching %s %s", filepath.Base(binPath), strings.Join(masked, " "))
}
