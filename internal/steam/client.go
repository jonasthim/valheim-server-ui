// Package steam wraps steamcmd: installing/updating the Valheim dedicated
// server (app 896660), reading the installed build id from the app manifest,
// and checking the latest public build id. See ARCHITECTURE.md §9/§11.
package steam

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// successMarker is printed by steamcmd once app_update actually completed.
var successMarker = fmt.Sprintf("Success! App '%s' fully installed", domain.SteamAppID)

// alreadyUpToDateMarker is printed instead of successMarker when nothing
// needed downloading.
const alreadyUpToDateMarker = "already up to date"

// flakeMarkers are well-known transient steamcmd failures worth retrying
// once (ARCHITECTURE.md / WORKPLAN.md WP-04).
var flakeMarkers = []string{
	"Error! State is 0x6 after update job",
	"0x202",
	"0x602",
}

// CommandRunner executes steamcmdPath with args, streaming combined
// stdout/stderr to out, and returns the process's error (nil on exit 0). It
// must honour ctx: cancelling ctx should kill the whole process group. Tests
// inject a fake implementation (or point Client at a fake shell script and
// keep the default runner).
type CommandRunner func(ctx context.Context, steamcmdPath string, args []string, out io.Writer) error

// runOptions carries per-invocation environment settings to the runner.
type runOptions struct {
	home string
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithCommandRunner overrides the default os/exec-based runner.
func WithCommandRunner(run CommandRunner) Option {
	return func(c *Client) { c.run = run }
}

// WithHome pins HOME for every steamcmd invocation. SteamCMD writes ~/Steam
// (logs, appcache, its own updates) and the manager's systemd unit denies
// access to /home (ProtectHome=true), so HOME must point inside the data
// directory regardless of the valheim account's passwd entry.
func WithHome(dir string) Option {
	return func(c *Client) { c.home = dir }
}

// Client talks to one steamcmd installation.
type Client struct {
	path string
	home string
	log  *slog.Logger
	run  CommandRunner

	mu          sync.Mutex
	latestBuild string
	latestAt    time.Time
}

// New builds a Client for the steamcmd executable at steamcmdPath (typically
// config.Config.SteamCMDPath).
func New(steamcmdPath string, log *slog.Logger, opts ...Option) *Client {
	if log == nil {
		log = slog.Default()
	}
	c := &Client{path: steamcmdPath, log: log}
	for _, o := range opts {
		o(c)
	}
	if c.run == nil {
		home := c.home
		c.run = func(ctx context.Context, path string, args []string, out io.Writer) error {
			return runCommandEnv(ctx, path, args, out, runOptions{home: home})
		}
	}
	return c
}

// Installed reports whether the configured steamcmd path exists and is
// executable.
func (c *Client) Installed() bool {
	info, err := os.Stat(c.path)
	if err != nil || info.IsDir() {
		return false
	}
	return isExecutable(c.path, info)
}

func (c *Client) missingErr() error {
	return domain.E(domain.CodeSteamCMDMissing, "steamcmd is not installed at "+c.path)
}

func (c *Client) installArgs(installDir string) []string {
	return []string{
		"+@sSteamCmdForcePlatformType", steamPlatformType,
		"+force_install_dir", installDir,
		"+login", "anonymous",
		"+app_update", domain.SteamAppID, "validate",
		"+quit",
	}
}

// InstallOrUpdate runs steamcmd's app_update against installDir, streaming
// output to out. It retries once if steamcmd prints one of the well-known
// transient failure markers; any other failure (non-zero exit or a missing
// success marker) is returned immediately with the last lines of output.
func (c *Client) InstallOrUpdate(ctx context.Context, installDir string, out io.Writer) error {
	if !c.Installed() {
		return c.missingErr()
	}
	args := c.installArgs(installDir)

	const maxAttempts = 2
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var buf bytes.Buffer
		var w io.Writer = &buf
		if out != nil {
			w = io.MultiWriter(out, &buf)
		}
		runErr := c.run(ctx, c.path, args, w)
		output := buf.String()

		if ctx.Err() != nil {
			return fmt.Errorf("steamcmd install/update cancelled: %w", ctx.Err())
		}
		if runErr == nil && (strings.Contains(output, successMarker) || strings.Contains(output, alreadyUpToDateMarker)) {
			return nil
		}
		if attempt < maxAttempts && isFlake(output) {
			c.log.Warn("steamcmd flaked, retrying once", "install_dir", installDir, "attempt", attempt)
			continue
		}
		if strings.Contains(output, "Disk write failure") {
			home := c.home
			if home == "" {
				home = os.Getenv("HOME")
			}
			return fmt.Errorf("steamcmd install/update failed: %s: %s", diskWriteHint(home, installDir), lastLines(output, 20))
		}
		if runErr != nil {
			return fmt.Errorf("steamcmd install/update failed: %w: %s", runErr, lastLines(output, 20))
		}
		return fmt.Errorf("steamcmd install/update failed: success marker not found: %s", lastLines(output, 20))
	}
	return fmt.Errorf("steamcmd install/update failed after retry")
}

// InstalledBuildID reads steamapps/appmanifest_896660.acf under installDir.
// A missing file (not yet installed) returns ("", nil), not an error.
func (c *Client) InstalledBuildID(installDir string) (string, error) {
	path := filepath.Join(installDir, "steamapps", "appmanifest_"+domain.SteamAppID+".acf")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read appmanifest: %w", err)
	}
	m := buildIDPattern.FindSubmatch(b)
	if m == nil {
		return "", nil
	}
	return string(m[1]), nil
}

// LatestBuildID runs `steamcmd +app_info_update 1 +app_info_print 896660` and
// parses the public branch's buildid, caching the result for Latest().
func (c *Client) LatestBuildID(ctx context.Context) (string, error) {
	if !c.Installed() {
		return "", c.missingErr()
	}
	args := []string{"+login", "anonymous", "+app_info_update", "1", "+app_info_print", domain.SteamAppID, "+quit"}
	var buf bytes.Buffer
	if err := c.run(ctx, c.path, args, &buf); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("steamcmd app_info_print cancelled: %w", ctx.Err())
		}
		return "", fmt.Errorf("steamcmd app_info_print failed: %w: %s", err, lastLines(buf.String(), 20))
	}
	id, err := parseLatestBuildID(buf.String())
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.latestBuild = id
	c.latestAt = time.Now().UTC()
	c.mu.Unlock()
	return id, nil
}

// Latest returns the last value LatestBuildID computed, and when.
func (c *Client) Latest() (buildid string, checkedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latestBuild, c.latestAt
}

func isFlake(output string) bool {
	for _, m := range flakeMarkers {
		if strings.Contains(output, m) {
			return true
		}
	}
	return false
}

// lastLines returns the last n lines of s, joined with "\n".
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// runCommandEnv is the default CommandRunner: it execs steamcmdPath in its own
// process group so ctx cancellation (via cmd.Cancel) can kill the whole group,
// not just the direct child (steamcmd itself forks helpers). When opts.home is
// set, HOME (and Steam's own HOME-derived caches) point there so every write
// lands inside a directory the manager is allowed to touch.
func runCommandEnv(ctx context.Context, steamcmdPath string, args []string, out io.Writer, opts runOptions) error {
	cmd := exec.CommandContext(ctx, steamcmdPath, args...) //nolint:gosec // steamcmd path/args are server-configured, not user input
	if opts.home != "" {
		cmd.Env = append(envWithout(os.Environ(), "HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"),
			"HOME="+opts.home,
			"XDG_CONFIG_HOME="+opts.home+"/.config",
			"XDG_DATA_HOME="+opts.home+"/.local/share",
			"XDG_CACHE_HOME="+opts.home+"/.cache",
		)
	}
	cmd.Stdout = out
	cmd.Stderr = out
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}

// envWithout returns environ minus the named variables.
func envWithout(environ []string, names ...string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		skip := false
		for _, n := range names {
			if strings.HasPrefix(kv, n+"=") {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, kv)
		}
	}
	return out
}

// diskWriteHint explains Valve's misleading "Disk write failure", which is
// what steamcmd reports when it cannot write to $HOME or the install dir
// (permissions or systemd sandboxing), far more often than a full disk.
func diskWriteHint(home, installDir string) string {
	return fmt.Sprintf("steamcmd reported a disk write failure. This usually means it could not write to HOME (%s) or the install directory (%s) rather than a full disk: check ownership by the valheim user, free space, and that the systemd unit allows writes there (ReadWritePaths / ProtectHome)", home, installDir)
}
