package agent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// basePNG is a 4×4 RGB map: every pixel pure green.
func basePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{0, 200, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// maskPNG is a 2×2 grey+alpha mask with the top-left cell explored (0) and
// the other three unexplored (255), as the plugin writes it.
func maskPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			v := uint8(255)
			if x == 0 && y == 0 {
				v = 0
			}
			img.Set(x, y, color.NRGBA{v, v, v, v})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCompositeFog(t *testing.T) {
	base, err := png.Decode(bytes.NewReader(basePNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	mask, err := png.Decode(bytes.NewReader(maskPNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	out := compositeFog(base, mask)
	at := func(x, y int) color.NRGBA { return out.NRGBAAt(x, y) }
	// Centre of the explored cell: the map shows through untouched.
	if c := at(0, 0); c.G != 200 || c.R != 0 {
		t.Fatalf("explored pixel changed: %+v", c)
	}
	// Centre of an unexplored cell: solid fog, nothing of the map left.
	if c := at(3, 3); c != fogColor {
		t.Fatalf("unexplored pixel not fog: %+v", c)
	}
	// Between cells the edge is blended, not stepped.
	if c := at(1, 1); c.G == 200 || c.G == fogColor.G {
		t.Fatalf("edge pixel should be blended: %+v", c)
	}
	if out.Bounds().Dx() != 4 || out.Bounds().Dy() != 4 {
		t.Fatal("output must keep the map's size")
	}
}

// TestMapPNG_FogIsServerSide covers what viewers get: the composite, never
// the bare map while a mask is pending, and the bare map for operators.
func TestMapPNG_FogIsServerSide(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.9.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	var maskReady bool
	mux := http.NewServeMux()
	auth := func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+cfg.Token }
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if auth(r) {
			_, _ = w.Write([]byte(sampleStatus))
		}
	})
	mux.HandleFunc("/v1/map/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":"ready","progress":1,"seed":7,"size":4,"world_radius":10500,"playable_radius":10000,"sea_level":30}`))
	})
	mux.HandleFunc("/v1/map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(basePNG(t))
	})
	mux.HandleFunc("/v1/map/objects", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"objects":[],"pins":[],"locations":[],"updated_at":"2026-09-10T12:00:00Z"}`))
	})
	mux.HandleFunc("/v1/map/explored/info", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":2,"size":2,"explored_cells":1,"total_cells":4,"percent":25,"mask_version":2}`))
	})
	mux.HandleFunc("/v1/map/explored", func(w http.ResponseWriter, r *http.Request) {
		if !maskReady {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"version":2,"size":2,"explored_cells":1,"total_cells":4,"percent":25,"mask_version":0}`))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(maskPNG(t))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.9.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	ctx := context.Background()
	s.tick(ctx)

	// Mask not encoded yet: a viewer gets "rendering", never the bare map.
	if p, _, err := s.MapPNG(ctx, "main", true); err != ErrMapRendering || p != "" {
		t.Fatalf("want ErrMapRendering before the first mask, got %q %v", p, err)
	}
	// An operator asking for the bare render gets it regardless.
	if p, _, err := s.MapPNG(ctx, "main", false); err != nil || !strings.HasSuffix(p, mapFileName(7, 4)) {
		t.Fatalf("bare map: %q %v", p, err)
	}

	maskReady = true
	p, info, err := s.MapPNG(ctx, "main", true)
	if err != nil || info == nil {
		t.Fatalf("fogged map: %v", err)
	}
	if filepath.Base(p) != foggedFileName(7, 4) {
		t.Fatalf("fogged path %q", p)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(3, 3).RGBA()
	if uint8(r>>8) != fogColor.R || uint8(g>>8) != fogColor.G || uint8(b>>8) != fogColor.B {
		t.Fatalf("served image is not fogged at an unexplored pixel: %d %d %d", r>>8, g>>8, b>>8)
	}
	if _, g, _, _ := img.At(0, 0).RGBA(); uint8(g>>8) != 200 {
		t.Fatalf("explored pixel must show the map, got g=%d", g>>8)
	}

	// Same inputs: served from the composite without rebuilding.
	fi1, _ := os.Stat(p)
	if p2, _, err := s.MapPNG(ctx, "main", true); err != nil || p2 != p {
		t.Fatalf("second call: %q %v", p2, err)
	}
	fi2, _ := os.Stat(p)
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("composite rebuilt although nothing changed")
	}

	// The map answer carries a version that keys the image URL.
	m, err := s.Map(ctx, "main", false)
	if err != nil || m.ImageVersion == "" {
		t.Fatalf("image version: %q %v", m.ImageVersion, err)
	}

	// Agent gone: the fogged composite is still what viewers get.
	srv.Close()
	s.tick(ctx)
	if p3, _, err := s.MapPNG(ctx, "main", true); err != nil || p3 != p {
		t.Fatalf("offline fogged map: %q %v", p3, err)
	}
}
