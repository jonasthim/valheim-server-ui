package agent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
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

// isParchment: the beige of the unexplored map (red > green > blue, light).
func isParchment(c color.NRGBA) bool {
	return c.R > c.G && c.G > c.B && c.R > 150 && c.R < 235 && c.B > 100
}

func lum(c color.NRGBA) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func TestCompositeFog(t *testing.T) {
	// 8×8 green map, 4×4 mask with the top-left 2×2 cells explored.
	base := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			base.Set(x, y, color.RGBA{0, 200, 0, 255})
		}
	}
	mask := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			v := uint8(255)
			if x < 2 && y < 2 {
				v = 0
			}
			mask.Set(x, y, color.NRGBA{v, v, v, v})
		}
	}
	out := compositeFog(base, mask, mapstyle.NewParchment(42, nil), 10500)
	at := func(x, y int) color.NRGBA { return out.NRGBAAt(x, y) }
	// Deep inside the explored area: the map shows through untouched.
	if c := at(0, 0); c.G != 200 || c.R != 0 || c.B != 0 {
		t.Fatalf("explored pixel changed: %+v", c)
	}
	// Deep inside the unexplored area: parchment, nothing of the map left.
	for _, p := range [][2]int{{7, 7}, {6, 1}, {1, 6}} {
		if c := at(p[0], p[1]); !isParchment(c) || c.G > c.R {
			t.Fatalf("unexplored pixel %v not parchment: %+v", p, c)
		}
	}
	// The explored side of the edge carries the rim shadow: at least one
	// pixel a quarter cell inside the boundary is darker than the map.
	darker := false
	for y := 0; y < 4; y++ {
		if lum(at(3, y)) < lum(at(0, 0)) {
			darker = true
		}
	}
	if !darker {
		t.Fatal("no rim shadow on the explored side of the edge")
	}
	// Across the boundary the transition is blended, not stepped.
	if c := at(4, 1); c == at(0, 0) || isParchment(c) && c.G < 160 && lum(c) > lum(at(7, 7))-1 {
		t.Fatalf("edge pixel should be blended: %+v", c)
	}
	if out.Bounds().Dx() != 8 || out.Bounds().Dy() != 8 {
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
	if !isParchment(color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}) {
		t.Fatalf("served image is not parchment at an unexplored pixel: %d %d %d", r>>8, g>>8, b>>8)
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

// TestFogPreview composites a synthetic, irregular explored area over a base
// map for a look check. Skipped unless FOG_PREVIEW_BASE (a map PNG, e.g. the
// mapstyle preview) and FOG_PREVIEW_OUT are set.
func TestFogPreview(t *testing.T) {
	basePath, out := os.Getenv("FOG_PREVIEW_BASE"), os.Getenv("FOG_PREVIEW_OUT")
	if basePath == "" || out == "" {
		t.Skip("set FOG_PREVIEW_BASE and FOG_PREVIEW_OUT")
	}
	base, err := decodePNG(basePath)
	if err != nil {
		t.Fatal(err)
	}
	const n = 1024
	mask := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			// explored: a blob around the island centre plus a corridor
			dx, dy := float64(x)-560, float64(y)-520
			r := 150 + 40*math.Sin(math.Atan2(dy, dx)*5)
			v := uint8(255)
			if math.Hypot(dx, dy) < r || (math.Abs(dy-dx*0.3) < 18 && dx > 0 && dx < 300) {
				v = 0
			}
			mask.Set(x, y, color.NRGBA{v, v, v, v})
		}
	}
	img := compositeFog(base, mask, mapstyle.NewParchment(42, nil), 10500)
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
