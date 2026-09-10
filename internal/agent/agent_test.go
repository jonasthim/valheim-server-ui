package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func testPaths(t *testing.T) domain.InstancePaths {
	t.Helper()
	return domain.PathsFor(filepath.Join(t.TempDir(), "instances"), "main")
}

func installPlugin(t *testing.T, paths domain.InstancePaths, version string) {
	t.Helper()
	dir := PluginDir(paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, domain.AgentPluginDLL), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"ValheimUI_Agent","version_number":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureConfig_WritesAndKeepsToken(t *testing.T) {
	paths := testPaths(t)
	c1, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Port != 2456 || c1.Bind != "127.0.0.1" || len(c1.Token) != 64 || c1.IntervalMs != 500 {
		t.Fatalf("unexpected config: %+v", c1)
	}
	body, err := os.ReadFile(ConfigPath(paths))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[Server]", "Port = 2456", "BindAddress = 127.0.0.1", "Token = " + c1.Token, "SnapshotIntervalMs = 500"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("config missing %q:\n%s", want, body)
		}
	}
	// A second start on another port keeps the token.
	c2, err := EnsureConfig(paths, 2466)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Token != c1.Token || c2.Port != 2466 {
		t.Fatalf("token not kept or port not updated: %+v", c2)
	}
	// BepInEx rewrites the file with comments and typed hints; values survive.
	rewritten := "## Settings file was created by plugin Valheim UI Agent\n[Server]\n\n## desc\n# Setting type: Int32\n# Default value: 0\nPort = 2466\n\nBindAddress = 127.0.0.1\n\nToken = " + c1.Token + "\n\nSnapshotIntervalMs = 250\n"
	if err := os.WriteFile(ConfigPath(paths), []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}
	c3, err := ReadConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if c3.Token != c1.Token || c3.Port != 2466 || c3.IntervalMs != 250 {
		t.Fatalf("parse of BepInEx-formatted file: %+v", c3)
	}
}

// fakeAgent mimics the plugin's HTTP API.
func fakeAgent(t *testing.T, token string, status string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"ok":false,"error":"unauthorized"}`))
			return false
		}
		return true
	}
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(status))
	})
	mux.HandleFunc("/v1/commands/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/commands/")
		body, _ := io.ReadAll(r.Body)
		switch name {
		case "kick":
			if r.URL.Query().Get("target") == "" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"ok":false,"message":"target (player name or host id) is required"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"message":"kicked ` + r.URL.Query().Get("target") + `"}`))
		case "broadcast":
			_, _ = w.Write([]byte(`{"ok":true,"message":"broadcast ` + strings.TrimSpace(string(body)) + `"}`))
		default:
			w.WriteHeader(404)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const sampleStatus = `{"agent_version":"1.5.0","game_version":"1.0.7","uptime_seconds":12.5,"ready":true,
"captured_at":"2026-09-10T12:00:00Z","world":{"name":"Midgard","seed":42,"day":7,"day_fraction":0.5,"is_night":false,"weather":"Clear","time_seconds":12600},
"global_keys":["defeated_eikthyr"],
"players":[{"uid":1,"name":"Bjorn","host":"765611980000","visible":true,"position":{"x":10,"y":30,"z":-5}},
{"uid":2,"name":"Freya","host":"765611980001","visible":false,"position":{"x":100,"y":31,"z":200}}]}`

func TestClient_StatusAndCommands(t *testing.T) {
	srv := fakeAgent(t, "secret", sampleStatus)
	c := NewClientForURL(srv.URL, "secret", nil)
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.World.Name != "Midgard" || len(st.Players) != 2 || st.Players[1].Position.Z != 200 {
		t.Fatalf("status decoded wrongly: %+v", st)
	}
	res, err := c.Command(context.Background(), domain.AgentCommandRequest{Command: "kick", Target: "Bjorn"})
	if err != nil || !res.OK || res.Message != "kicked Bjorn" {
		t.Fatalf("kick: %+v %v", res, err)
	}
	// A refusal (400 with a message) is a result, not an error.
	res, err = c.Command(context.Background(), domain.AgentCommandRequest{Command: "kick"})
	if err != nil || res.OK || !strings.Contains(res.Message, "target") {
		t.Fatalf("refused kick: %+v %v", res, err)
	}
	res, err = c.Command(context.Background(), domain.AgentCommandRequest{Command: "broadcast", Message: "Restart in 5 minutes"})
	if err != nil || !res.OK || res.Message != "broadcast Restart in 5 minutes" {
		t.Fatalf("broadcast: %+v %v", res, err)
	}
	bad := NewClientForURL(srv.URL, "wrong", nil)
	if _, err := bad.Status(context.Background()); err == nil {
		t.Fatal("expected an error with the wrong token")
	}
}

type fakeInstances struct {
	paths domain.InstancePaths
	inst  domain.Instance
}

func (f *fakeInstances) List(context.Context) ([]domain.Instance, error) {
	return []domain.Instance{f.inst}, nil
}
func (f *fakeInstances) Get(context.Context, string) (*domain.Instance, error) {
	in := f.inst
	return &in, nil
}
func (f *fakeInstances) Paths(string) domain.InstancePaths { return f.paths }

type fakeBus struct{ events []domain.Event }

func (b *fakeBus) Publish(ev domain.Event) { b.events = append(b.events, ev) }

type fixedVersion string

func (v fixedVersion) Version() string { return string(v) }

func TestService_PollInfoAndHiddenPositions(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.4.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	srv := fakeAgent(t, cfg.Token, sampleStatus)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	bus := &fakeBus{}
	s := NewService(fi, bus, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.5.0"))
	// Point the poller at the fake instead of 127.0.0.1:<port>.
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }

	s.tick(context.Background())

	info, err := s.Info(context.Background(), "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Installed || !info.Connected || info.InstalledVersion != "1.4.0" || info.BundledVersion != "1.5.0" || !info.UpdateAvailable {
		t.Fatalf("info: %+v", info)
	}
	if info.Status == nil || len(info.Status.Players) != 2 {
		t.Fatalf("status missing: %+v", info.Status)
	}
	if info.Status.Players[0].Position == nil || info.Status.Players[1].Position != nil {
		t.Fatalf("viewer must not see the hidden player's position: %+v", info.Status.Players)
	}
	full, _ := s.Info(context.Background(), "main", true)
	if full.Status.Players[1].Position == nil {
		t.Fatal("operator must see every position")
	}
	if len(bus.events) != 1 || bus.events[0].Name != domain.EventAgentStatus || bus.events[0].InstanceID != "main" {
		t.Fatalf("expected one agent.status event, got %+v", bus.events)
	}
	// The bus payload never carries hidden positions.
	raw, _ := json.Marshal(bus.events[0].Data)
	if strings.Contains(string(raw), `"z":200`) {
		t.Fatalf("hidden position leaked on the bus: %s", raw)
	}
	// Unchanged poll within publishEvery: no new event.
	s.tick(context.Background())
	if len(bus.events) != 1 {
		t.Fatalf("unchanged status re-published: %d events", len(bus.events))
	}

	st := &domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning}
	s.Enrich(context.Background(), st)
	if !st.AgentConnected {
		t.Fatal("enricher should mark the agent connected")
	}

	// Server goes away: one disconnect event, Connected false.
	srv.Close()
	s.tick(context.Background())
	info, _ = s.Info(context.Background(), "main", false)
	if info.Connected || info.LastError == "" || len(bus.events) != 2 {
		t.Fatalf("disconnect not reported: %+v events=%d", info, len(bus.events))
	}
}

func TestService_PreStartWritesConfigOnlyWhenRelevant(t *testing.T) {
	paths := testPaths(t)
	s := NewService(&fakeInstances{paths: paths}, nil, nil, nil)
	if err := s.PreStart(context.Background(), "main", paths, domain.InstanceConfig{Port: 2456, BepInExEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ConfigPath(paths)); !os.IsNotExist(err) {
		t.Fatal("config must not be written when the plugin is absent")
	}
	installPlugin(t, paths, "1.5.0")
	if err := s.PreStart(context.Background(), "main", paths, domain.InstanceConfig{Port: 2456, BepInExEnabled: true}); err != nil {
		t.Fatal(err)
	}
	c, _ := ReadConfig(paths)
	if c.Port != 2456 || c.Token == "" {
		t.Fatalf("config not written: %+v", c)
	}
}

func TestService_CommandRequiresRunningAgent(t *testing.T) {
	paths := testPaths(t)
	fi := &fakeInstances{paths: paths, inst: domain.Instance{ID: "main", Status: domain.InstanceStatus{State: domain.StateStopped}}}
	s := NewService(fi, nil, nil, nil)
	if _, err := s.Command(context.Background(), "main", domain.AgentCommandRequest{Command: "save"}); err == nil {
		t.Fatal("expected an error without the plugin")
	}
	if _, err := s.Command(context.Background(), "main", domain.AgentCommandRequest{Command: "nuke"}); err == nil {
		t.Fatal("expected validation error for an unknown command")
	}
}

func TestBundle_Version(t *testing.T) {
	if got := NewBundle("v1.5.0", t.TempDir(), nil, nil).Version(); got != "1.5.0" {
		t.Fatalf("Version = %q", got)
	}
	if got := NewBundle("v1.0.1-16-gabc-dirty", t.TempDir(), nil, nil).Version(); got != "0.0.0" {
		t.Fatalf("dev Version = %q", got)
	}
}

func TestBundle_OverrideAndMissing(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "agent.zip")
	if err := os.WriteFile(zipPath, []byte("PK"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := NewBundle("v1.5.0", dir, nil, nil)
	b.override = zipPath
	if p, err := b.Fetch(context.Background()); err != nil || p != zipPath {
		t.Fatalf("override: %q %v", p, err)
	}
	b.override = ""
	if _, err := b.Fetch(context.Background()); err == nil {
		t.Fatal("expected an error with no embedded zip and no release source")
	}
	_ = time.Second
}

// TestService_StatusArraysNeverNull: before the world loads the plugin
// answers {"ready":false}; the contract still promises global_keys and
// players as arrays, so neither may reach a client as null.
func TestService_StatusArraysNeverNull(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.9.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	srv := fakeAgent(t, cfg.Token, `{"ready":false}`)
	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	bus := &fakeBus{}
	s := NewService(fi, bus, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.9.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	s.tick(context.Background())

	info, err := s.Info(context.Background(), "main", false)
	if err != nil || info.Status == nil {
		t.Fatalf("info: %+v %v", info, err)
	}
	for name, v := range map[string]any{"info": info, "event": bus.events[0].Data} {
		raw, _ := json.Marshal(v)
		if !strings.Contains(string(raw), `"global_keys":[]`) || !strings.Contains(string(raw), `"players":[]`) {
			t.Fatalf("%s must carry empty arrays, got %s", name, raw)
		}
	}
}
