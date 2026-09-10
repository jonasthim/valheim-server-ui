//go:build !windows

package main

// runAsServiceIfNeeded is a no-op outside Windows: systemd runs `serve` as an
// ordinary foreground process.
func runAsServiceIfNeeded(string, []string) (bool, error) { return false, nil }
