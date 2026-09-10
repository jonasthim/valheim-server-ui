// Package agent integrates the Valheim UI Agent, the server-side BepInEx
// plugin built from plugin/, into the manager: it writes the plugin's config
// (port and token) before a start, polls the plugin's loopback API while an
// instance runs, publishes what it learns on the event bus and forwards the
// admin commands the UI issues. See docs/ARCHITECTURE.md §20.
package agent

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

const (
	defaultBind       = "127.0.0.1"
	defaultIntervalMs = 500
	tokenBytes        = 32
)

// PluginDir is where the mod installer places the agent package.
func PluginDir(paths domain.InstancePaths) string {
	return filepath.Join(paths.BepInExDir(), "plugins", domain.AgentModOwner+"-"+domain.AgentModName)
}

// PluginPath is the plugin assembly itself; its presence means "installed".
func PluginPath(paths domain.InstancePaths) string {
	return filepath.Join(PluginDir(paths), domain.AgentPluginDLL)
}

// ConfigPath is the BepInEx config file the plugin reads on start.
func ConfigPath(paths domain.InstancePaths) string {
	return filepath.Join(paths.BepInExDir(), "config", domain.AgentConfigFile)
}

// Installed reports whether the agent plugin is present in the instance.
func Installed(paths domain.InstancePaths) bool {
	fi, err := os.Stat(PluginPath(paths))
	return err == nil && fi.Mode().IsRegular()
}

// Config is the [Server] section of the plugin's BepInEx config file.
type Config struct {
	Port       int
	Bind       string
	Token      string
	IntervalMs int
}

// ReadConfig parses the plugin config. A missing file yields zero values and
// no error, so callers can treat "never written" and "empty" alike.
func ReadConfig(paths domain.InstancePaths) (Config, error) {
	var c Config
	f, err := os.Open(ConfigPath(paths))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("agent: read config: %w", err)
	}
	defer func() { _ = f.Close() }()

	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "Server" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "Port":
			c.Port, _ = strconv.Atoi(value)
		case "BindAddress":
			c.Bind = value
		case "Token":
			c.Token = value
		case "SnapshotIntervalMs":
			c.IntervalMs, _ = strconv.Atoi(value)
		}
	}
	return c, sc.Err()
}

// EnsureConfig writes the plugin config for a start on gamePort: the port the
// API listens on (TCP on the game port, so it never collides with another
// instance), loopback binding, and a token. An existing token is kept so a
// restart does not invalidate it; a missing or short one is regenerated.
// Returns the effective config.
func EnsureConfig(paths domain.InstancePaths, gamePort int) (Config, error) {
	c, err := ReadConfig(paths)
	if err != nil {
		return c, err
	}
	c.Port = gamePort
	if c.Bind == "" {
		c.Bind = defaultBind
	}
	if c.IntervalMs <= 0 {
		c.IntervalMs = defaultIntervalMs
	}
	if len(c.Token) < 2*tokenBytes {
		buf := make([]byte, tokenBytes)
		if _, err := rand.Read(buf); err != nil {
			return c, fmt.Errorf("agent: generate token: %w", err)
		}
		c.Token = hex.EncodeToString(buf)
	}
	if err := writeConfig(paths, c); err != nil {
		return c, err
	}
	return c, nil
}

// writeConfig renders the file in BepInEx's own format; the plugin rewrites
// it with its descriptions on load and keeps these values.
func writeConfig(paths domain.InstancePaths, c Config) error {
	path := ConfigPath(paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("agent: create config dir: %w", err)
	}
	body := fmt.Sprintf("## Written by Valheim Server UI before each start; the token is the manager's credential for the agent API.\n\n"+
		"[Server]\n\n"+
		"## TCP port of the loopback API (the game port; TCP, so no collision with the game's UDP ports).\n"+
		"Port = %d\n\n"+
		"## Address to listen on. Keep it on loopback; the manager runs on the same host.\n"+
		"BindAddress = %s\n\n"+
		"## Bearer token the manager must present.\n"+
		"Token = %s\n\n"+
		"## How often the world state is captured for GET /v1/status.\n"+
		"SnapshotIntervalMs = %d\n", c.Port, c.Bind, c.Token, c.IntervalMs)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return fmt.Errorf("agent: write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("agent: install config: %w", err)
	}
	return nil
}
