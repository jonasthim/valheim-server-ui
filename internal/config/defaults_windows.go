//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// Windows layout: everything under %ProgramData%\valheim-ui. There is no
// systemd, so the manager (itself a Windows service) supervises the game
// processes directly; unitctl and its sudo grant do not exist.
const (
	defaultSupervisor  = "direct"
	defaultUnitctlPath = ""
	systemdAvailable   = false
)

func programData() string {
	if d := os.Getenv("ProgramData"); d != "" {
		return d
	}
	return `C:\ProgramData`
}

func defaultConfigPath() string { return filepath.Join(programData(), "valheim-ui", "config.yaml") }
func defaultDataDir() string    { return filepath.Join(programData(), "valheim-ui") }
func defaultSteamCMDPath() string {
	return filepath.Join(programData(), "valheim-ui", "steamcmd", "steamcmd.exe")
}
