//go:build !windows

package config

// Linux layout (ARCHITECTURE.md §3): data under /var/lib/valheim, config in
// /etc, instances supervised by systemd through the sudo wrapper.
const (
	defaultSupervisor  = "systemd"
	defaultUnitctlPath = "/usr/local/lib/valheim-ui/unitctl"
	systemdAvailable   = true
)

func defaultConfigPath() string   { return "/etc/valheim-ui/config.yaml" }
func defaultDataDir() string      { return "/var/lib/valheim" }
func defaultSteamCMDPath() string { return "/var/lib/valheim/steamcmd/steamcmd.sh" }
