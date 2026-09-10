package steam

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/events"
)

func fixtureAppInfo(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/app_info.vdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func clientAlwaysReturning(t *testing.T, output []byte) *Client {
	t.Helper()
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		_, _ = out.Write(output)
		return nil
	}
	return New(writeFakeSteamCMD(t, "exit 0"), nil, WithCommandRunner(run))
}

// writeManifest drops a fake appmanifest with the given buildid into
// <dir>/steamapps, mimicking one installed instance.
func writeManifest(t *testing.T, dir, buildID string) {
	t.Helper()
	steamapps := filepath.Join(dir, "steamapps")
	if err := os.MkdirAll(steamapps, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `"AppState"
{
	"appid"		"896660"
	"buildid"		"` + buildID + `"
}
`
	if err := os.WriteFile(filepath.Join(steamapps, "appmanifest_896660.acf"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChecker_CheckNow_PublishesWhenDifferent(t *testing.T) {
	client := clientAlwaysReturning(t, fixtureAppInfo(t)) // latest = 22222222
	bus := events.NewBus()
	sub := bus.Subscribe("")
	defer sub.Close()

	instA := t.TempDir()
	writeManifest(t, instA, "11111111") // stale -> update available
	instB := t.TempDir()
	writeManifest(t, instB, "22222222") // current -> no update

	var stored []struct{ id, installed, latest string }
	var mu sync.Mutex
	store := func(ctx context.Context, instanceID, installed, latest string, at time.Time) error {
		mu.Lock()
		stored = append(stored, struct{ id, installed, latest string }{instanceID, installed, latest})
		mu.Unlock()
		return nil
	}
	listInstalled := func(ctx context.Context) ([]InstalledRef, error) {
		return []InstalledRef{{ID: "a", InstallDir: instA}, {ID: "b", InstallDir: instB}}, nil
	}

	checker := NewUpdateChecker(client, func() time.Duration { return 0 }, listInstalled, store, bus)
	if err := checker.CheckNow(context.Background()); err != nil {
		t.Fatalf("CheckNow: %v", err)
	}

	infoA := checker.Info("a")
	if infoA == nil || !infoA.UpdateAvailable || infoA.InstalledBuildID != "11111111" || infoA.LatestBuildID != "22222222" {
		t.Fatalf("unexpected info for a: %+v", infoA)
	}
	infoB := checker.Info("b")
	if infoB == nil || infoB.UpdateAvailable {
		t.Fatalf("expected no update available for b, got %+v", infoB)
	}
	if checker.Info("unknown") != nil {
		t.Fatal("expected nil Info for an instance never checked")
	}

	mu.Lock()
	gotStored := len(stored)
	mu.Unlock()
	if gotStored != 2 {
		t.Fatalf("expected store called for both instances, got %d calls", gotStored)
	}

	select {
	case ev := <-sub.C:
		if ev.Name != domain.EventUpdateAvailable {
			t.Fatalf("expected update.available, got %s", ev.Name)
		}
		info, ok := ev.Data.(domain.UpdateInfo)
		if !ok || info.InstanceID != "a" {
			t.Fatalf("expected UpdateInfo for instance a, got %+v", ev.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected an update.available event for instance a")
	}

	select {
	case ev := <-sub.C:
		t.Fatalf("expected no second event (instance b is up to date), got %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUpdateChecker_Run_TicksAndStops(t *testing.T) {
	client := clientAlwaysReturning(t, fixtureAppInfo(t))
	instDir := t.TempDir()
	writeManifest(t, instDir, "00000000")

	var checks int32
	var mu sync.Mutex
	listInstalled := func(ctx context.Context) ([]InstalledRef, error) {
		mu.Lock()
		checks++
		mu.Unlock()
		return []InstalledRef{{ID: "only", InstallDir: instDir}}, nil
	}

	checker := NewUpdateChecker(client, func() time.Duration { return 10 * time.Millisecond }, listInstalled, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { checker.Run(ctx); close(done) }()

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := checks
		mu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected at least 2 ticks, got %d", n)
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after ctx cancellation")
	}
}

func TestUpdateChecker_Run_DisabledDoesNotCheck(t *testing.T) {
	client := clientAlwaysReturning(t, fixtureAppInfo(t))
	var checked atomic.Int32
	listInstalled := func(ctx context.Context) ([]InstalledRef, error) {
		checked.Add(1)
		return nil, nil
	}
	checker := NewUpdateChecker(client, func() time.Duration { return 0 }, listInstalled, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	checker.Run(ctx) // interval<=0 polls every minute; should return via ctx timeout without ever checking
	if checked.Load() != 0 {
		t.Fatalf("expected no checks while disabled, got %d", checked.Load())
	}
}

func TestUpdateChecker_CheckNow_PropagatesLatestBuildIDError(t *testing.T) {
	run := func(ctx context.Context, path string, args []string, out io.Writer) error {
		return errors.New("boom")
	}
	client := New(writeFakeSteamCMD(t, "exit 0"), nil, WithCommandRunner(run))
	checker := NewUpdateChecker(client, func() time.Duration { return 0 }, nil, nil, nil)
	if err := checker.CheckNow(context.Background()); err == nil {
		t.Fatal("expected CheckNow to propagate the steamcmd error")
	}
}
