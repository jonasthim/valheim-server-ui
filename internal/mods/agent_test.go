package mods

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeAgentBundle serves a Thunderstore-style agent zip from a temp file.
type fakeAgentBundle struct {
	path    string
	version string
	fetches int
}

func newFakeAgentBundle(t *testing.T, version string) *fakeAgentBundle {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"manifest.json", `{"name":"ValheimUI_Agent","author":"jonasthim","version_number":"` + version + `","dependencies":[]}`},
		{"README.md", "agent"},
		{"plugins/ValheimUI.Agent.dll", "MZ agent " + version},
	} {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "valheim-ui-agent.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return &fakeAgentBundle{path: p, version: version}
}

func (b *fakeAgentBundle) Fetch(context.Context) (string, error) { b.fetches++; return b.path, nil }
func (b *fakeAgentBundle) Version() string                       { return b.version }

func TestInstallBepInEx_AlsoInstallsTheAgent(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	bundle := newFakeAgentBundle(t, "1.5.0")
	modSvc.SetAgentBundle(bundle)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatalf("EnqueueBepInExInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/core/BepInEx.Preloader.dll", "preloader")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ agent 1.5.0")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/manifest.json",
		`{"name":"ValheimUI_Agent","author":"jonasthim","version_number":"1.5.0","dependencies":[]}`)
	if bundle.fetches != 1 {
		t.Fatalf("expected one bundle fetch, got %d", bundle.fetches)
	}

	ov, err := modSvc.Overview(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	var agent *domain.Mod
	for i := range ov.Mods {
		if ov.Mods[i].Name == domain.AgentModName {
			agent = &ov.Mods[i]
		}
	}
	if agent == nil {
		t.Fatalf("agent not recorded as a mod: %+v", ov.Mods)
	}
	if agent.Source != domain.ModSourceBundled || agent.Owner != domain.AgentModOwner || agent.Version != "1.5.0" || !agent.Enabled {
		t.Fatalf("unexpected agent row: %+v", agent)
	}
}

func TestEnqueueAgentInstall_UpdatesInPlaceAndGuards(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, sup, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	// Without BepInEx the agent cannot be installed.
	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.5.0"))
	if _, err := modSvc.EnqueueAgentInstall(ctx, "main", false, "tester"); err == nil {
		t.Fatal("expected bepinex_missing")
	} else if de := domain.AsError(err); de.Code != domain.CodeBepInExMissing {
		t.Fatalf("expected bepinex_missing, got %v", de.Code)
	}

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, runner, job.ID)

	// A newer manager ships a newer plugin: the standalone job replaces it,
	// keeping a single mod row, and refuses while running unless asked to stop.
	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.6.0"))
	sup.setStatus("main", supervisor.Status{State: supervisor.StateRunning})
	if _, err := modSvc.EnqueueAgentInstall(ctx, "main", false, "tester"); err == nil {
		t.Fatal("expected instance_running without stop_if_running")
	}
	job, err = modSvc.EnqueueAgentInstall(ctx, "main", true, "tester")
	if err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ agent 1.6.0")
	ov, err := modSvc.Overview(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range ov.Mods {
		if m.Name == domain.AgentModName {
			count++
			if m.Version != "1.6.0" {
				t.Fatalf("expected 1.6.0, got %s", m.Version)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one agent row, got %d", count)
	}
}
