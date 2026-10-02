package mods

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/agent"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/supervisor"
)

// fakeAgentBundle serves a Thunderstore-style plugin zip from a temp file; it
// backs both the agent and the gameplay bundle in these tests (both satisfy
// mods.AgentBundle identically).
type fakeAgentBundle struct {
	path    string
	version string
	fetches int
	// fetchErr, when set, makes Fetch fail instead of returning path (used
	// to simulate a gameplay bundle that cannot be resolved).
	fetchErr error
}

// newFakeBundle builds a fake bundle for a plugin package whose manifest
// "name" is name (e.g. "ValheimUI_Agent" or "ValheimUI_Gameplay") and whose
// plugin DLL is dll, at version.
func newFakeBundle(t *testing.T, name, dll, version string) *fakeAgentBundle {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct{ name, body string }{
		{"manifest.json", `{"name":"` + name + `","author":"jonasthim","version_number":"` + version + `","dependencies":[]}`},
		{"README.md", name},
		{"plugins/" + dll, "MZ " + name + " " + version},
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
	p := filepath.Join(t.TempDir(), name+".zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return &fakeAgentBundle{path: p, version: version}
}

// newFakeAgentBundle keeps the original helper name/shape used throughout
// this file for the agent plugin specifically.
func newFakeAgentBundle(t *testing.T, version string) *fakeAgentBundle {
	return newFakeBundle(t, "ValheimUI_Agent", "ValheimUI.Agent.dll", version)
}

// newFakeGameplayBundle is the gameplay-plugin equivalent.
func newFakeGameplayBundle(t *testing.T, version string) *fakeAgentBundle {
	return newFakeBundle(t, "ValheimUI_Gameplay", "ValheimUI.Gameplay.dll", version)
}

// newFailingBundle never resolves a package (e.g. no release asset found).
func newFailingBundle(version string) *fakeAgentBundle {
	return &fakeAgentBundle{version: version, fetchErr: errors.New("no package available")}
}

func (b *fakeAgentBundle) Fetch(context.Context) (string, error) {
	b.fetches++
	if b.fetchErr != nil {
		return "", b.fetchErr
	}
	return b.path, nil
}
func (b *fakeAgentBundle) Version() string { return b.version }

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
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ ValheimUI_Agent 1.5.0")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/manifest.json",
		`{"name":"ValheimUI_Agent","author":"jonasthim","version_number":"1.5.0","dependencies":[]}`)
	if bundle.fetches != 1 {
		t.Fatalf("expected one bundle fetch, got %d", bundle.fetches)
	}

	ov, err := modSvc.Overview(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	var agentMod *domain.Mod
	for i := range ov.Mods {
		if ov.Mods[i].Name == domain.AgentModName {
			agentMod = &ov.Mods[i]
		}
	}
	if agentMod == nil {
		t.Fatalf("agent not recorded as a mod: %+v", ov.Mods)
	}
	if agentMod.Source != domain.ModSourceBundled || agentMod.Owner != domain.AgentModOwner || agentMod.Version != "1.5.0" || !agentMod.Enabled {
		t.Fatalf("unexpected agent row: %+v", agentMod)
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
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ ValheimUI_Agent 1.6.0")
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

// TestInstallBepInEx_AlsoInstallsTheGameplayPluginAndDefaultCfg covers card
// M-1 step 6: BepInEx install installs both bundled plugins and writes the
// gameplay plugin's default cfg (EnsureGameplayConfig) once it is installed.
func TestInstallBepInEx_AlsoInstallsTheGameplayPluginAndDefaultCfg(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.18.0"))
	gameplayBundle := newFakeGameplayBundle(t, "1.18.0")
	modSvc.SetGameplayBundle(gameplayBundle)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatalf("EnqueueBepInExInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_gameplay/ValheimUI.Gameplay.dll", "MZ ValheimUI_Gameplay 1.18.0")
	if gameplayBundle.fetches != 1 {
		t.Fatalf("expected one gameplay bundle fetch, got %d", gameplayBundle.fetches)
	}

	ov, err := modSvc.Overview(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	var gameplayMod *domain.Mod
	for i := range ov.Mods {
		if ov.Mods[i].Name == domain.BundledGameplay.Name {
			gameplayMod = &ov.Mods[i]
		}
	}
	if gameplayMod == nil {
		t.Fatalf("gameplay plugin not recorded as a mod: %+v", ov.Mods)
	}
	if gameplayMod.Source != domain.ModSourceBundled || gameplayMod.Owner != domain.BundledGameplay.Owner || gameplayMod.Version != "1.18.0" || !gameplayMod.Enabled {
		t.Fatalf("unexpected gameplay row: %+v", gameplayMod)
	}

	cfgBody, err := os.ReadFile(agent.GameplayConfigPath(paths))
	if err != nil {
		t.Fatalf("default gameplay cfg not written: %v", err)
	}
	for _, want := range []string{"[Autofeed]", "Enabled = false", "[Raids]", "Enabled = true"} {
		if !strings.Contains(string(cfgBody), want) {
			t.Fatalf("gameplay cfg missing %q:\n%s", want, cfgBody)
		}
	}
}

// TestEnqueueAgentInstall_UpdatesBothPluginsAndJobTitle covers card M-1 step
// 6: the agent_install job updates both bundled plugins and its title says
// "plugins" (not just the agent) now that it covers two.
func TestEnqueueAgentInstall_UpdatesBothPluginsAndJobTitle(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.18.0"))
	modSvc.SetGameplayBundle(newFakeGameplayBundle(t, "1.18.0"))

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, runner, job.ID)

	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.19.0"))
	modSvc.SetGameplayBundle(newFakeGameplayBundle(t, "1.19.0"))
	job, err = modSvc.EnqueueAgentInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if job.Title != "Install/update Valheim UI plugins" {
		t.Fatalf("unexpected job title: %q", job.Title)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ ValheimUI_Agent 1.19.0")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_gameplay/ValheimUI.Gameplay.dll", "MZ ValheimUI_Gameplay 1.19.0")
}

// TestInstallBepInEx_FailingGameplayBundleIsOnlyAWarning covers card M-1
// step 6: a failing gameplay bundle (Fetch error) leaves the agent
// installed and only logs a warning; the BepInEx install job still succeeds.
func TestInstallBepInEx_FailingGameplayBundleIsOnlyAWarning(t *testing.T) {
	ts, _ := newMiniThunderstore(t)
	modSvc, instSvc, _, runner := newTestModsService(t, ts)
	modSvc.SetAgentBundle(newFakeAgentBundle(t, "1.18.0"))
	modSvc.SetGameplayBundle(newFailingBundle("1.18.0"))
	ctx := context.Background()
	createTestInstance(t, instSvc, "main")
	markInstalled(t, instSvc, "main")

	job, err := modSvc.EnqueueBepInExInstall(ctx, "main", false, "tester")
	if err != nil {
		t.Fatalf("EnqueueBepInExInstall: %v", err)
	}
	mustSucceed(t, runner, job.ID)

	paths := instSvc.Paths("main")
	assertHasFile(t, paths.Server, "BepInEx/plugins/jonasthim-valheimui_agent/ValheimUI.Agent.dll", "MZ ValheimUI_Agent 1.18.0")
	if _, err := os.Stat(filepath.Join(paths.Server, "BepInEx/plugins/jonasthim-valheimui_gameplay")); err == nil {
		t.Fatal("gameplay plugin should not have been installed")
	}

	lines, err := runner.Log(ctx, job.ID)
	if err != nil {
		t.Fatalf("runner.Log: %v", err)
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "warning") && strings.Contains(l, domain.BundledGameplay.Title) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a gameplay warning in the job log, got: %v", lines)
	}
}
