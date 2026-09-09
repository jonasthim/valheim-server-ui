package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// stopGrace is how long Stop waits for a SIGINT to end the process before
// escalating to SIGKILL (ARCHITECTURE.md §6).
const stopGrace = 120 * time.Second

// direct is the development/test Supervisor: it spawns
// `<SelfPath> launch --instance <id>` as a child process instead of going
// through systemd. Instances die when the manager process exits.
type direct struct {
	o Options

	mu    sync.Mutex
	procs map[string]*directProc
}

type directProc struct {
	mu        sync.Mutex
	cmd       *exec.Cmd
	pid       int
	since     time.Time
	autostart bool
	state     State
	detail    string
	stopping  bool
	done      chan struct{}
}

// NewDirect returns the development Supervisor.
func NewDirect(o Options) Supervisor {
	return &direct{o: o, procs: map[string]*directProc{}}
}

func (d *direct) Kind() string { return "direct" }

func (d *direct) get(id string) *directProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.procs[id]
}

func (d *direct) getOrCreate(id string) *directProc {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.procs[id]
	if !ok {
		p = &directProc{state: StateStopped}
		d.procs[id] = p
	}
	return p
}

func (d *direct) Start(ctx context.Context, id string) error {
	p := d.getOrCreate(id)
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd != nil && (p.state == StateRunning || p.state == StateStarting) {
		return nil // already running: no-op
	}

	paths := domain.PathsFor(d.o.InstancesDir, id)
	if err := os.MkdirAll(paths.Logs, 0o750); err != nil {
		return domain.Wrap(domain.CodeInternal, "create log dir", err)
	}
	logFile, err := os.OpenFile(paths.ConsoleLog(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return domain.Wrap(domain.CodeInternal, "open console.log", err)
	}

	// Intentionally exec.Command, not CommandContext: the spawned game process
	// must outlive the (request-scoped) ctx passed to Start; it is stopped
	// only via an explicit Stop/Restart call, never by ctx cancellation.
	cmd := exec.Command(d.o.SelfPath, "launch", "--instance", id) //nolint:noctx
	cmd.Dir = paths.Server
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// cmd.Env left nil: inherit the manager process's environment, including
	// VALHEIM_UI_CONFIG, so `launch` reads the same config file.

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return domain.Wrap(domain.CodeInternal, fmt.Sprintf("start instance %s", id), err)
	}

	p.cmd = cmd
	p.pid = cmd.Process.Pid
	p.since = time.Now()
	p.state = StateRunning
	p.detail = ""
	p.stopping = false
	done := make(chan struct{})
	p.done = done

	go d.reap(id, p, cmd, logFile, done)
	return nil
}

// reap waits for the child to exit and records the resulting state. It never
// blocks callers of Start/Stop/Status.
func (d *direct) reap(id string, p *directProc, cmd *exec.Cmd, logFile *os.File, done chan struct{}) {
	err := cmd.Wait()
	_ = logFile.Close()

	p.mu.Lock()
	defer p.mu.Unlock()
	defer close(done)

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	switch exitCode {
	case 0:
		p.state = StateStopped
		p.detail = ""
	default:
		p.state = StateFailed
		p.detail = fmt.Sprintf("exit status %d", exitCode)
		if d.o.Log != nil {
			d.o.Log.Warn("instance exited unexpectedly", "instance", id, "detail", p.detail)
		}
	}
}

func (d *direct) Stop(ctx context.Context, id string) error {
	p := d.get(id)
	if p == nil {
		return nil
	}
	p.mu.Lock()
	if p.cmd == nil || p.state == StateStopped || p.state == StateFailed {
		p.mu.Unlock()
		return nil
	}
	proc := p.cmd.Process
	done := p.done
	p.state = StateStopping
	p.stopping = true
	p.mu.Unlock()

	if proc == nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGINT); err != nil && !isProcessDone(err) {
		return domain.Wrap(domain.CodeInternal, fmt.Sprintf("signal instance %s", id), err)
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(stopGrace):
	}

	if err := proc.Signal(syscall.SIGKILL); err != nil && !isProcessDone(err) {
		return domain.Wrap(domain.CodeInternal, fmt.Sprintf("kill instance %s", id), err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (d *direct) Restart(ctx context.Context, id string) error {
	if err := d.Stop(ctx, id); err != nil {
		return err
	}
	return d.Start(ctx, id)
}

func (d *direct) Status(ctx context.Context, id string) (Status, error) {
	p := d.get(id)
	if p == nil {
		return Status{State: StateStopped}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return Status{
		State:     p.state,
		PID:       pidIfRunning(p),
		Since:     p.since,
		Autostart: p.autostart,
		Detail:    p.detail,
	}, nil
}

func pidIfRunning(p *directProc) int {
	if p.state == StateRunning || p.state == StateStarting || p.state == StateStopping {
		return p.pid
	}
	return 0
}

func (d *direct) SetAutostart(ctx context.Context, id string, on bool) error {
	p := d.getOrCreate(id)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.autostart = on
	return nil
}

// isProcessDone reports whether err from Process.Signal indicates the process
// has already exited (os.ErrProcessDone, or ESRCH on some platforms).
func isProcessDone(err error) bool {
	return err == os.ErrProcessDone
}
