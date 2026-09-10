package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// tinyPNG is the 8-byte signature plus filler: enough for the client's check.
var tinyPNG = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, []byte("fake-idat")...)

func mapAgent(t *testing.T, token string, state *atomic.Value) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return false
		}
		return true
	}
	info := func() string {
		return `{"state":"` + state.Load().(string) + `","progress":0.5,"seed":42,"size":512,"world_radius":10500,"playable_radius":10000,"sea_level":30}`
	}
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(sampleStatus))
		}
	})
	mux.HandleFunc("/v1/map/info", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(info()))
		}
	})
	mux.HandleFunc("/v1/map/objects", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			_, _ = w.Write([]byte(`{"objects":[{"type":"portal","label":"Portal","x":10,"y":31,"z":-20,"text":"home"}],"locations":[{"name":"StartTemple","x":0,"y":30,"z":0}],"updated_at":"2026-09-10T12:00:00Z"}`))
		}
	})
	mux.HandleFunc("/v1/map", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if state.Load().(string) != "ready" {
			w.WriteHeader(202)
			_, _ = w.Write([]byte(info()))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG)
	})
	mux.HandleFunc("/v1/map/render", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			state.Store("rendering")
			w.WriteHeader(202)
			_, _ = w.Write([]byte(info()))
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMap_FetchCacheAndOffline(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.6.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	var state atomic.Value
	state.Store("rendering")
	srv := mapAgent(t, cfg.Token, &state)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.6.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()
	s.tick(ctx) // connect

	// Rendering: progress, no image.
	m, err := s.Map(ctx, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Connected || m.ImageReady || m.Info == nil || m.Info.State != "rendering" || len(m.Objects) != 1 || len(m.Locations) != 1 || len(m.Players) != 2 {
		t.Fatalf("map while rendering: %+v", m)
	}
	if _, info, err := s.MapPNG(ctx, "main", false); err != ErrMapRendering || info == nil {
		t.Fatalf("expected ErrMapRendering with info, got %v %v", err, info)
	}

	// Ready: fetched once into the instance cache, then served from disk.
	state.Store("ready")
	s.mu.Lock()
	s.maps["main"].infoAt = s.now().Add(-time.Hour)
	s.mu.Unlock()
	p, info, err := s.MapPNG(ctx, "main", false)
	if err != nil || info == nil || info.State != "ready" {
		t.Fatalf("MapPNG ready: %v %v", err, info)
	}
	want := filepath.Join(MapCacheDir(paths), "map-42-512.png")
	if p != want {
		t.Fatalf("cached at %s, want %s", p, want)
	}
	if b, _ := os.ReadFile(p); string(b) != string(tinyPNG) {
		t.Fatal("cached image differs from the agent's")
	}
	m, _ = s.Map(ctx, "main", false)
	if !m.ImageReady || m.Stale {
		t.Fatalf("map after fetch: %+v", m)
	}

	// Force re-render drops the cached file and reports rendering.
	ri, err := s.RenderMap(ctx, "main", domain.MapRenderRequest{Force: true})
	if err != nil || ri.State != "rendering" {
		t.Fatalf("RenderMap: %v %v", err, ri)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("forced render should drop the cached image")
	}
	if _, err := s.RenderMap(ctx, "main", domain.MapRenderRequest{Size: 10}); err == nil {
		t.Fatal("expected a validation error for size 10")
	}

	// Agent gone: the newest cached image (write one) is served as stale.
	state.Store("ready")
	if _, _, err := s.MapPNG(ctx, "main", false); err != nil {
		t.Fatalf("refetch: %v", err)
	}
	srv.Close()
	s.tick(ctx)
	p, info, err = s.MapPNG(ctx, "main", false)
	if err != nil || p != want || info != nil {
		t.Fatalf("offline MapPNG: %s %v %v", p, info, err)
	}
	m, _ = s.Map(ctx, "main", false)
	if m.Connected || !m.ImageReady || !m.Stale {
		t.Fatalf("offline map: %+v", m)
	}
	if !strings.HasSuffix(p, ".png") {
		t.Fatal("unexpected path")
	}
}

func TestMap_OldAgentWithoutMapEndpoints(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.5.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+cfg.Token {
			_, _ = w.Write([]byte(strings.Replace(sampleStatus, `"agent_version":"1.5.0"`, `"agent_version":"1.5.0"`, 1)))
		}
	})
	// Everything under /v1/map is unknown to a 1.5.0 agent.
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.6.1"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()
	s.tick(ctx)

	m, err := s.Map(ctx, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Connected || m.MapSupported || m.AgentVersion != "1.5.0" || m.ImageReady {
		t.Fatalf("old agent: %+v", m)
	}
	_, err = s.RenderMap(ctx, "main", domain.MapRenderRequest{})
	if err == nil || !strings.Contains(err.Error(), "no map support") {
		t.Fatalf("expected the update hint, got %v", err)
	}
}

func TestExploredPNG_FetchesWhenVersionChanges(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.7.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	var maskVersion atomic.Int32
	maskVersion.Store(3)
	var fetches atomic.Int32
	mux := http.NewServeMux()
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+cfg.Token }
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if auth(r) {
			_, _ = w.Write([]byte(sampleStatus))
		}
	})
	mux.HandleFunc("/v1/map/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"ready","progress":1,"seed":42,"size":512,"world_radius":10500,"playable_radius":10000,"sea_level":30}`))
	})
	mux.HandleFunc("/v1/map/objects", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"objects":[{"type":"portal","label":"Portal","x":1,"y":31,"z":2,"text":"a","explored":false}],` +
			`"pins":[{"name":"Eikthyr","x":-500,"y":40,"z":300,"type":"boss","type_id":9,"checked":false,"author":"Bjorn"}],` +
			`"locations":[{"name":"Eikthyrnir","x":-510,"y":40,"z":310,"explored":false,"discovered":true}],"updated_at":"2026-09-10T12:00:00Z"}`))
	})
	mux.HandleFunc("/v1/map/explored/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":` + fmt.Sprint(maskVersion.Load()) + `,"size":1024,"explored_cells":5000,"total_cells":1048576,"percent":0.48,"mask_version":` + fmt.Sprint(maskVersion.Load()) + `}`))
	})
	mux.HandleFunc("/v1/map/explored", func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(append(append([]byte{}, tinyPNG...), byte(maskVersion.Load())))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.7.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()
	s.tick(ctx)

	m, err := s.Map(ctx, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if !m.FogSupported || m.Explored == nil || m.Explored.Percent != 0.48 || m.Objects[0].Explored == nil || *m.Objects[0].Explored {
		t.Fatalf("fog state: %+v obj=%+v", m.Explored, m.Objects[0])
	}
	if len(m.Pins) != 1 || m.Pins[0].Type != "boss" || m.Pins[0].Author != "Bjorn" || m.Pins[0].Name != "Eikthyr" {
		t.Fatalf("pins: %+v", m.Pins)
	}
	if len(m.Locations) != 1 || m.Locations[0].Discovered == nil || !*m.Locations[0].Discovered {
		t.Fatalf("location discovered flag: %+v", m.Locations)
	}

	p1, info, err := s.ExploredPNG(ctx, "main")
	if err != nil || info == nil || info.MaskVersion != 3 {
		t.Fatalf("first mask: %v %v", err, info)
	}
	if _, _, err := s.ExploredPNG(ctx, "main"); err != nil {
		t.Fatal(err)
	}
	if fetches.Load() != 1 {
		t.Fatalf("same version must be served from the cache, fetches=%d", fetches.Load())
	}
	maskVersion.Store(4)
	p2, _, err := s.ExploredPNG(ctx, "main")
	if err != nil || fetches.Load() != 2 || p2 != p1 {
		t.Fatalf("new version must be refetched into the same file: %v fetches=%d %s", err, fetches.Load(), p2)
	}
	b, _ := os.ReadFile(p2)
	if b[len(b)-1] != 4 {
		t.Fatal("cached file not updated to the new mask")
	}

	// Agent gone: last mask still served.
	srv.Close()
	s.tick(ctx)
	if p, _, err := s.ExploredPNG(ctx, "main"); err != nil || p != p1 {
		t.Fatalf("offline mask: %s %v", p, err)
	}
}

// TestService_ExplorationProgressPublishes covers the percent in the UI: a
// change in explored cells alone (no player moved, mask not re-encoded yet)
// must publish a fresh agent.status event carrying the new fog state.
func TestService_ExplorationProgressPublishes(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.9.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	var cells atomic.Int32
	cells.Store(100)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+cfg.Token {
			_, _ = w.Write([]byte(sampleStatus))
		}
	})
	mux.HandleFunc("/v1/map/explored/info", func(w http.ResponseWriter, r *http.Request) {
		c := cells.Load()
		_, _ = fmt.Fprintf(w, `{"version":%d,"size":1024,"explored_cells":%d,"total_cells":1048576,"percent":%.2f,"mask_version":1}`, c, c, float64(c)*100/1048576)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	bus := &fakeBus{}
	s := NewService(fi, bus, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.9.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()

	s.tick(ctx)
	s.tick(ctx)
	if len(bus.events) != 1 {
		t.Fatalf("unchanged exploration must not re-publish: %d events", len(bus.events))
	}
	cells.Store(160)
	s.tick(ctx)
	if len(bus.events) != 2 {
		t.Fatalf("exploration progress must publish an event, got %d", len(bus.events))
	}
	raw, _ := json.Marshal(bus.events[1].Data)
	if !strings.Contains(string(raw), `"explored_cells":160`) {
		t.Fatalf("event must carry the new fog state: %s", raw)
	}
}

// islandLayers is a small synthetic world for the styling path: an island
// of meadows in the ocean.
func islandLayers(size int) *mapstyle.Layers {
	l := mapstyle.NewLayers(size)
	c := float32(size) / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float32(x)+0.5-c, float32(y)+0.5-c
			d := float32(math.Hypot(float64(dx), float64(dy))) / c // 0 centre .. 1 edge
			switch {
			case d > 0.98:
				l.Set(x, y, mapstyle.OffWorld, 0, -20, 0)
			case d > 0.4:
				l.Set(x, y, mapstyle.Ocean, 0, 30-(d-0.4)*100, 2)
			default:
				l.Set(x, y, mapstyle.Meadows, 0, 34+(0.4-d)*60, 1.6)
			}
		}
	}
	return l
}

func layersAgent(t *testing.T, token string, layers []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+token }
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if auth(r) {
			_, _ = w.Write([]byte(sampleStatus))
		}
	})
	mux.HandleFunc("/v1/map/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"ready","progress":1,"seed":42,"size":64,"world_radius":10500,"playable_radius":10000,"sea_level":30,"layers":true,"layers_version":1}`))
	})
	mux.HandleFunc("/v1/map/layers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(layers)
	})
	mux.HandleFunc("/v1/map", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"ok":false,"error":"layers"}`))
	})
	mux.HandleFunc("/v1/map/objects", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"objects":[],"pins":[],"locations":[],"updated_at":"2026-09-10T12:00:00Z"}`))
	})
	mux.HandleFunc("/v1/map/explored/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":1,"size":2,"explored_cells":1,"total_cells":4,"percent":25,"mask_version":1}`))
	})
	mux.HandleFunc("/v1/map/explored", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(maskPNG(t))
	})
	mux.HandleFunc("/v1/map/render", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"rendering","progress":0,"seed":42,"size":64,"world_radius":10500,"playable_radius":10000,"sea_level":30,"layers":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestMap_LayersStyledAndCached(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.10.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	layersPNG, err := mapstyle.EncodeLayers(islandLayers(64))
	if err != nil {
		t.Fatal(err)
	}
	srv := layersAgent(t, cfg.Token, layersPNG)
	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.10.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()
	s.tick(ctx)

	m, err := s.Map(ctx, "main", false)
	if err != nil || !m.LayersSupported || m.StyleVersion != mapstyle.Version || !m.ImageReady {
		t.Fatalf("map: %+v %v", m, err)
	}
	p, info, err := s.MapPNG(ctx, "main", false)
	if err != nil || info == nil || !info.Layers {
		t.Fatalf("bare map: %q %v", p, err)
	}
	dir := MapCacheDir(paths)
	if filepath.Base(p) != "styled-42-64-v1-builtin.png" {
		t.Fatalf("styled path %q", p)
	}
	if _, err := os.Stat(filepath.Join(dir, "layers-42-64.png")); err != nil {
		t.Fatal("layers must be cached")
	}
	// A styled pixel in the island centre is meadows green; the corner is abyss.
	img, err := decodePNG(p)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(32, 32).RGBA()
	if g <= r || g <= b {
		t.Fatalf("island centre not green: %d %d %d", r>>8, g>>8, b>>8)
	}
	// Second call: served from the cache without restyling.
	st1, _ := os.Stat(p)
	if p2, _, err := s.MapPNG(ctx, "main", false); err != nil || p2 != p {
		t.Fatalf("second call: %q %v", p2, err)
	}
	if st2, _ := os.Stat(p); !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("styled map re-rendered although nothing changed")
	}
	// Fogged path uses the styled base and the parchment.
	fp, _, err := s.MapPNG(ctx, "main", true)
	if err != nil || filepath.Base(fp) != "fogstyled-42-64-v1-builtin.png" {
		t.Fatalf("fogged: %q %v", fp, err)
	}
	m, _ = s.Map(ctx, "main", false)
	if !strings.Contains(m.ImageVersion, "-v1-builtin") {
		t.Fatalf("image version %q must carry style and pack", m.ImageVersion)
	}

	// A texture pack changes the cache name and the version.
	texDir := MapTexturesDir(paths)
	if err := os.MkdirAll(texDir, 0o750); err != nil {
		t.Fatal(err)
	}
	magenta := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range magenta.Pix {
		magenta.Pix[i] = 255
	}
	for i := 1; i < len(magenta.Pix); i += 4 {
		magenta.Pix[i] = 0 // G = 0
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, magenta); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(texDir, "meadows.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	p3, _, err := s.MapPNG(ctx, "main", false)
	if err != nil || p3 == p || strings.HasSuffix(p3, "-builtin.png") {
		t.Fatalf("pack must re-key the styled map: %q %v", p3, err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("stale styled file of the old pack must be removed")
	}
	img3, _ := decodePNG(p3)
	r, g, b, _ = img3.At(32, 32).RGBA()
	if r <= g || b <= g {
		t.Fatalf("island centre should use the magenta pack texture: %d %d %d", r>>8, g>>8, b>>8)
	}

	// Force re-render drops the layers and styled files.
	if _, err := s.RenderMap(ctx, "main", domain.MapRenderRequest{Force: true}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"layers-42-64.png", filepath.Base(p3)} {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			t.Fatalf("%s must be removed on a forced re-render", n)
		}
	}
}

func TestMap_OfflineRestyleAcrossStyleVersions(t *testing.T) {
	paths := testPaths(t)
	dir := MapCacheDir(paths)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	layersPNG, _ := mapstyle.EncodeLayers(islandLayers(32))
	if err := os.WriteFile(filepath.Join(dir, "layers-7-32.png"), layersPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "styled-7-32-v0-builtin.png")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateStopped},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.10.0"))
	p, _, err := s.MapPNG(context.Background(), "main", false)
	if err != nil || filepath.Base(p) != "styled-7-32-v1-builtin.png" {
		t.Fatalf("offline restyle: %q %v", p, err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("styled file of an older style version must be removed")
	}
	m, err := s.Map(context.Background(), "main", false)
	if err != nil || !m.ImageReady {
		t.Fatalf("offline map must report an image: %+v %v", m, err)
	}
}

func TestNewestCachedMap_Prefixes(t *testing.T) {
	dir := t.TempDir()
	for i, n := range []string{"explored-1.png", "layers-1-64.png", "fogmap-1-64.png", "fogstyled-1-64-v1-x.png", "map-1-64.png", "styled-1-64-v1-builtin.png"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(filepath.Join(dir, n), mt, mt)
	}
	if got := filepath.Base(newestCachedMap(dir)); got != "styled-1-64-v1-builtin.png" {
		t.Fatalf("newest cached map %q", got)
	}
	_ = os.Remove(filepath.Join(dir, "styled-1-64-v1-builtin.png"))
	if got := filepath.Base(newestCachedMap(dir)); got != "map-1-64.png" {
		t.Fatalf("flat image must still count: %q", got)
	}
	if got := filepath.Base(newestLayers(dir)); got != "layers-1-64.png" {
		t.Fatalf("newest layers %q", got)
	}
}
