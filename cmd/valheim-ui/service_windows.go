//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"

	"github.com/jonasthim/valheim-server-ui/internal/config"
)

// serviceName is the Windows service deploy/install.ps1 registers.
const serviceName = "valheim-ui"

// runAsServiceIfNeeded runs `serve` under the service control manager when
// the process was started by it (install.ps1 registers the binary with no
// arguments, so cmd is "serve"). Interactive invocations return false and
// run exactly as on Linux.
func runAsServiceIfNeeded(cmd string, args []string) (bool, error) {
	if cmd != "serve" {
		return false, nil
	}
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false, nil
	}
	return true, svc.Run(serviceName, &serviceHandler{args: args})
}

type serviceHandler struct {
	args []string
}

// Execute implements svc.Handler. A clean exit reports SERVICE_STOPPED; a
// serve error, or the process exiting on its own after a self-upgrade (the
// upgrade job calls os.Exit(0) exactly as on Linux), makes the SCM count the
// service as failed so the recovery actions install.ps1 configures restart
// it on the new binary.
func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	cfg, err := parseServe(h.args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return true, 1
	}
	logOut, closeLog := serviceLog(cfg)
	defer closeLog()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- serve(ctx, cfg, logOut) }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-errc:
			if err != nil {
				fmt.Fprintln(logOut, "error:", err)
				return true, 1
			}
			return false, 0
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-errc; err != nil {
					fmt.Fprintln(logOut, "error:", err)
					return true, 1
				}
				return false, 0
			}
		}
	}
}

// serviceLog opens <data_dir>\manager.log for the service's slog output;
// a service has no stderr anyone can read. Falls back to io.Discard.
func serviceLog(cfg config.Config) (io.Writer, func()) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return io.Discard, func() {}
	}
	f, err := os.OpenFile(filepath.Join(cfg.DataDir, "manager.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640) //nolint:gosec // path is the configured data dir
	if err != nil {
		return io.Discard, func() {}
	}
	return f, func() { _ = f.Close() }
}
