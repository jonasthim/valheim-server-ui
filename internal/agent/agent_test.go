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
		case "time":
			// Echo the mapped query so the client's field mapping is asserted.
			q := r.URL.Query()
			_, _ = w.Write([]byte(`{"ok":true,"message":"time","data":{"skip":"` + q.Get("skip") +
				`","fraction":"` + q.Get("fraction") + `","seconds":"` + q.Get("seconds") + `"}}`))
		case "say":
			_, _ = w.Write([]byte(`{"ok":true,"message":"said as ` + r.URL.Query().Get("name") +
				`: ` + strings.TrimSpace(string(body)) + `"}`))
		case "setkey", "removekey":
			_, _ = w.Write([]byte(`{"ok":true,"message":"` + name + ` ` + r.URL.Query().Get("key") + `"}`))
		case "event":
			q := r.URL.Query()
			_, _ = w.Write([]byte(`{"ok":true,"message":"event ` + q.Get("name") +
				` at ` + q.Get("x") + `,` + q.Get("z") + `"}`))
		case "eventstop":
			_, _ = w.Write([]byte(`{"ok":true,"message":"event stopped"}`))
		default:
			w.WriteHeader(404)
		}
	})
	mux.HandleFunc("/v1/catalog", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"global_keys":["defeated_eikthyr"],"events":[{"name":"army_eikthyr","duration_seconds":90}],"server_name":"Midgard"}`))
	})
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"next":5,"messages":[{"seq":5,"at":"2026-09-10T12:00:00Z","type":"shout","sender":"Bjorn","text":"hi"}]}`))
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

func f64(v float64) *float64 { return &v }

func TestClient_NewVerbsCatalogChat(t *testing.T) {
	srv := fakeAgent(t, "secret", sampleStatus)
	c := NewClientForURL(srv.URL, "secret", nil)
	ctx := context.Background()

	// time: fraction maps to the query and Data carries it back.
	res, err := c.Command(ctx, domain.AgentCommandRequest{Command: "time", Fraction: f64(0.25)})
	if err != nil || !res.OK || !strings.Contains(string(res.Data), `"fraction":"0.25"`) {
		t.Fatalf("time fraction: %+v %v", res, err)
	}
	// time: skip=morning maps through.
	res, err = c.Command(ctx, domain.AgentCommandRequest{Command: "time", Skip: "morning"})
	if err != nil || !strings.Contains(string(res.Data), `"skip":"morning"`) {
		t.Fatalf("time skip: %+v %v", res, err)
	}
	// say: name -> query, message -> body.
	res, err = c.Command(ctx, domain.AgentCommandRequest{Command: "say", Name: "Odin", Message: "hello"})
	if err != nil || res.Message != "said as Odin: hello" {
		t.Fatalf("say: %+v %v", res, err)
	}
	// setkey: key -> query.
	res, err = c.Command(ctx, domain.AgentCommandRequest{Command: "setkey", Key: "defeated_gdking"})
	if err != nil || res.Message != "setkey defeated_gdking" {
		t.Fatalf("setkey: %+v %v", res, err)
	}
	// event: name + x/z -> query (name from the Event field).
	res, err = c.Command(ctx, domain.AgentCommandRequest{Command: "event", Event: "army_eikthyr", X: f64(10), Z: f64(-5)})
	if err != nil || res.Message != "event army_eikthyr at 10,-5" {
		t.Fatalf("event: %+v %v", res, err)
	}

	cat, err := c.Catalog(ctx)
	if err != nil || cat.ServerName != "Midgard" || len(cat.Events) != 1 || cat.Events[0].Name != "army_eikthyr" {
		t.Fatalf("catalog: %+v %v", cat, err)
	}
	ch, err := c.Chat(ctx, 0, 50)
	if err != nil || ch.Next != 5 || len(ch.Messages) != 1 || ch.Messages[0].Sender != "Bjorn" {
		t.Fatalf("chat: %+v %v", ch, err)
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
		if !strings.Contains(string(raw), `"global_keys":[]`) || !strings.Contains(string(raw), `"players":[]`) || !strings.Contains(string(raw), `"pings":[]`) {
			t.Fatalf("%s must carry empty arrays, got %s", name, raw)
		}
	}
}
