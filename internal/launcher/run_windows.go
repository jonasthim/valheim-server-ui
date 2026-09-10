//go:build windows

package launcher

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Stop protocol between the direct supervisor and this proxy, spoken over the
// proxy's stdin (one word per line):
//
//	stop   send Ctrl+C to the console the game shares with the proxy, so the
//	       dedicated server saves the world and exits (exit code propagated)
//	kill   terminate the game immediately
//
// EOF on stdin (the manager is gone) is treated as "stop": nothing supervises
// the game any more, and a clean save beats an orphaned server holding the
// ports when the manager comes back.
const (
	stopWord = "stop"
	killWord = "kill"
)

// platformExec spawns the game as a child in a kill-on-close job object and
// waits for it. It never returns on success: the proxy exits with the game's
// exit code, mirroring exec on Linux.
var platformExec execFunc = spawnAndForward

func addPlatformEnv(func(key, value string), map[string]string, string) {}

// platformDoorstop drives BepInEx's Doorstop through its command-line options
// (Doorstop reads doorstop_config.ini on Windows, not the environment the
// pack's shell script exports, so the environment alone cannot switch it):
// enabled with the preloader as target when the instance has mods on, and
// explicitly disabled whenever the pack's winhttp.dll proxy is present but
// mods are off, since the DLL would otherwise load with the ini's defaults.
func platformDoorstop(launch domain.Launch, env []string) ([]string, []string) {
	args := append([]string(nil), launch.Args...)
	switch {
	case launch.BepInEx:
		target := filepath.Join(launch.ServerDir, "BepInEx", "core", "BepInEx.Preloader.dll")
		args = append(args, "--doorstop-enabled", "true", "--doorstop-target-assembly", target)
	case fileExists(filepath.Join(launch.ServerDir, "winhttp.dll")):
		args = append(args, "--doorstop-enabled", "false")
		env = append(env, "DOORSTOP_ENABLED=0")
	}
	return env, args
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// jobHandle keeps the job object alive for the proxy's lifetime: closing the
// last handle to a KILL_ON_JOB_CLOSE job terminates the game, which is
// exactly what should happen if this proxy dies.
var jobHandle windows.Handle

func spawnAndForward(argv0 string, argv []string, envv []string) error {
	// The Ctrl+C generated for the game reaches every process on the shared
	// console, this proxy included; it must survive it to report the exit.
	// A registered handler is what keeps a Go process alive on Windows
	// (os/signal: "If Notify is called for os.Interrupt, ^C ... will not
	// exit"); signal.Ignore alone still lets the default handler terminate
	// the process with STATUS_CONTROL_C_EXIT.
	interrupts := make(chan os.Signal, 4)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for range interrupts {
		}
	}()
	// A service has no console; without one GenerateConsoleCtrlEvent has
	// nothing to deliver to. The call fails harmlessly when a console exists.
	_ = allocConsole()

	cmd := exec.Command(argv0, argv[1:]...) //nolint:gosec,noctx // the binary and args come from launch.json written by the manager
	cmd.Env = envv
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launcher: start %s: %w", filepath.Base(argv0), err)
	}
	if err := confineToJob(cmd.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "[valheim-ui] warning: job object not applied: %v\n", err)
	}

	go forwardStopRequests(cmd.Process)

	err := cmd.Wait()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		fmt.Fprintf(os.Stderr, "[valheim-ui] wait: %v\n", err)
		code = 1
	}
	if code < 0 {
		code = 1
	}
	os.Exit(code)
	return nil
}

// forwardStopRequests reads the supervisor's one-word commands from stdin.
func forwardStopRequests(proc *os.Process) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch strings.TrimSpace(sc.Text()) {
		case stopWord:
			requestGracefulStop(proc)
		case killWord:
			_ = proc.Kill()
			return
		}
	}
	requestGracefulStop(proc)
}

// requestGracefulStop delivers Ctrl+C to every process on this console (the
// game and this proxy, which ignores it). If no console event can be sent
// the game is terminated instead: the supervisor would escalate to kill after
// its grace period anyway, and a hung stop is worse than a lost save.
func requestGracefulStop(proc *os.Process) {
	if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_C_EVENT, 0); err != nil {
		fmt.Fprintf(os.Stderr, "[valheim-ui] console Ctrl+C failed (%v); terminating the server\n", err)
		_ = proc.Kill()
	}
}

// confineToJob puts pid into a job object that kills its members when the
// last handle closes, so the game never outlives this proxy.
func confineToJob(pid int) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("configure job object: %w", err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid)) //nolint:gosec // pid is a fresh child
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("open child process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("assign child to job object: %w", err)
	}
	jobHandle = job
	return nil
}

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procAllocConsole = kernel32.NewProc("AllocConsole")
)

func allocConsole() error {
	r, _, err := procAllocConsole.Call()
	if r == 0 {
		return err
	}
	return nil
}
