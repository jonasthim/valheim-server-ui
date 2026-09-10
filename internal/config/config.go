// Package config loads the manager configuration from a YAML file with
// VALHEIM_UI_* environment overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen          string `yaml:"listen"`
	BaseURL         string `yaml:"base_url"`
	DataDir         string `yaml:"data_dir"`
	Supervisor      string `yaml:"supervisor"` // systemd | direct
	UnitctlPath     string `yaml:"unitctl_path"`
	SteamCMDPath    string `yaml:"steamcmd_path"`
	InsecureCookies bool   `yaml:"insecure_cookies"`
	LogLevel        string `yaml:"log_level"`
	// FakeServer makes `valheim-ui launch` exec FakeServerPath instead of the real
	// binary. Development and tests only.
	FakeServer     bool   `yaml:"fake_server"`
	FakeServerPath string `yaml:"fake_server_path"`
	// DevNoAuth disables authentication and injects a synthetic admin. The server
	// refuses to listen on anything but loopback when this is set.
	DevNoAuth bool `yaml:"dev_no_auth"`
}

// DefaultPath is the config file read when neither --config nor
// VALHEIM_UI_CONFIG is given: /etc/valheim-ui/config.yaml on Linux,
// %ProgramData%\valheim-ui\config.yaml on Windows.
var DefaultPath = defaultConfigPath()

func Default() Config {
	return Config{
		Listen:       "127.0.0.1:8080",
		DataDir:      defaultDataDir(),
		Supervisor:   defaultSupervisor,
		UnitctlPath:  defaultUnitctlPath,
		SteamCMDPath: defaultSteamCMDPath(),
		LogLevel:     "info",
	}
}

// Load reads path (missing file is fine), applies env overrides and validates.
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(b, &c); err != nil {
				return c, fmt.Errorf("parse %s: %w", path, err)
			}
		case errors.Is(err, os.ErrNotExist):
			// fall through to defaults + env
		default:
			return c, fmt.Errorf("read %s: %w", path, err)
		}
	}
	applyEnv(&c)
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func applyEnv(c *Config) {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv("VALHEIM_UI_" + key); ok {
			*dst = v
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := os.LookupEnv("VALHEIM_UI_" + key); ok {
			b, err := strconv.ParseBool(v)
			if err == nil {
				*dst = b
			}
		}
	}
	str("LISTEN", &c.Listen)
	str("BASE_URL", &c.BaseURL)
	str("DATA_DIR", &c.DataDir)
	str("SUPERVISOR", &c.Supervisor)
	str("UNITCTL_PATH", &c.UnitctlPath)
	str("STEAMCMD_PATH", &c.SteamCMDPath)
	boolean("INSECURE_COOKIES", &c.InsecureCookies)
	str("LOG_LEVEL", &c.LogLevel)
	boolean("FAKE_SERVER", &c.FakeServer)
	str("FAKE_SERVER_PATH", &c.FakeServerPath)
	boolean("DEV_NO_AUTH", &c.DevNoAuth)
}

func (c *Config) Validate() error {
	if c.Supervisor != "systemd" && c.Supervisor != "direct" {
		return fmt.Errorf("supervisor must be systemd or direct, got %q", c.Supervisor)
	}
	if c.Supervisor == "systemd" && !systemdAvailable {
		return fmt.Errorf("supervisor %q is not available on %s; use direct", c.Supervisor, runtime.GOOS)
	}
	if c.DataDir == "" {
		return errors.New("data_dir is required")
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return fmt.Errorf("data_dir: %w", err)
	}
	c.DataDir = abs
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.DevNoAuth && !strings.HasPrefix(c.Listen, "127.0.0.1:") && !strings.HasPrefix(c.Listen, "localhost:") {
		return errors.New("dev_no_auth requires listen on 127.0.0.1")
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be debug|info|warn|error, got %q", c.LogLevel)
	}
	return nil
}

// Path helpers for the data directory layout (ARCHITECTURE.md §3).
func (c Config) DBPath() string       { return filepath.Join(c.DataDir, "manager.db") }
func (c Config) InstancesDir() string { return filepath.Join(c.DataDir, "instances") }
func (c Config) JobsDir() string      { return filepath.Join(c.DataDir, "jobs") }
func (c Config) CacheDir() string     { return filepath.Join(c.DataDir, "cache") }
func (c Config) SteamCMDDir() string  { return filepath.Join(c.DataDir, "steamcmd") }
func (c Config) InstanceDir(id string) string {
	return filepath.Join(c.InstancesDir(), id)
}
