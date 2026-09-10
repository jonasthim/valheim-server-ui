package agent

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func tileService(t *testing.T) (*Service, domain.InstancePaths) {
	t.Helper()
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
	s.tick(context.Background())
	return s, paths
}

func TestTiles_RenderCacheAndFog(t *testing.T) {
	old := prewarmZoom
	prewarmZoom = 2 // 21 tiles instead of 341 under the race detector
	t.Cleanup(func() { prewarmZoom = old })
	s, paths := tileService(t)
	ctx := context.Background()

	for _, bad := range [][3]int{{-1, 0, 0}, {MaxZoom + 1, 0, 0}, {1, 2, 0}, {1, 0, -1}} {
		if _, _, _, err := s.TilePNG(ctx, "main", bad[0], bad[1], bad[2], false); domain.AsError(err).Code != domain.CodeNotFound {
			t.Fatalf("tile %v must be not found, got %v", bad, err)
		}
	}

	data, etag, info, err := s.TilePNG(ctx, "main", 0, 0, 0, false)
	if err != nil || info == nil || !strings.HasPrefix(etag, `"42-64-v1-builtin/0/0/0`) {
		t.Fatalf("tile 0/0/0: %v etag=%s", err, etag)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != TileSize || img.Bounds().Dy() != TileSize {
		t.Fatalf("tile decode: %v %v", err, img.Bounds())
	}
	r, g, b, _ := img.At(128, 128).RGBA()
	if g <= r || g <= b {
		t.Fatalf("island centre should be green: %d %d %d", r>>8, g>>8, b>>8)
	}
	tilePath := filepath.Join(TileDir(paths), "42-64-v1-builtin", "0", "0", "0.png")
	st1, err := os.Stat(tilePath)
	if err != nil {
		t.Fatal("tile must be cached on disk")
	}
	again, _, _, err := s.TilePNG(ctx, "main", 0, 0, 0, false)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatal("cached tile must serve the same bytes")
	}
	if st2, _ := os.Stat(tilePath); !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("cached tile must not be rewritten")
	}

	// Fogged: the mask explores only the top-left cell, so the bottom-right
	// quarter tile at z=1 is parchment while the top-left one shows terrain
	// at its centre.
	fogBR, etagBR, _, err := s.TilePNG(ctx, "main", 1, 1, 1, true)
	if err != nil || !strings.Contains(etagBR, "-m") {
		t.Fatalf("fogged tile: %v etag=%s", err, etagBR)
	}
	imgBR, _ := png.Decode(bytes.NewReader(fogBR))
	r, g, b, _ = imgBR.At(128, 128).RGBA()
	if r <= g || g <= b {
		t.Fatalf("unexplored tile must be parchment: %d %d %d", r>>8, g>>8, b>>8)
	}
	fogTL, _, _, err := s.TilePNG(ctx, "main", 1, 0, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	imgTL, _ := png.Decode(bytes.NewReader(fogTL))
	bare, _, _, _ := s.TilePNG(ctx, "main", 1, 0, 0, false)
	imgBare, _ := png.Decode(bytes.NewReader(bare))
	// Deep inside the explored cell (the 2×2 mask fades from its centre
	// outwards) the fogged tile equals the bare one; toward the cell's far
	// corner the parchment blends in.
	if imgTL.At(20, 20) != imgBare.At(20, 20) {
		t.Fatal("explored area of a fogged tile must equal the bare tile")
	}
	if imgTL.At(250, 250) == imgBare.At(250, 250) {
		t.Fatal("the fog edge must blend toward the unexplored cells")
	}

	// The map answer advertises the pyramid with a version that carries the
	// style, the pack and the mask version.
	m, err := s.Map(ctx, "main", false)
	if err != nil || m.Tiles == nil || m.Tiles.MaxZoom != MaxZoom || m.Tiles.TileSize != TileSize {
		t.Fatalf("tiles info: %+v %v", m.Tiles, err)
	}
	if !strings.HasPrefix(m.Tiles.Version, "42-64-v1-builtin-m") {
		t.Fatalf("tiles version %q", m.Tiles.Version)
	}

	// Pre-warm produced the low levels in the background.
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(TileDir(paths), "42-64-v1-builtin", "2", "3", "3.png")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pre-warm did not render level 2")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestTiles_SeamlessAndMatchRegion(t *testing.T) {
	s, _ := tileService(t)
	ctx := context.Background()
	left, _, _, err := s.TilePNG(ctx, "main", 2, 1, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	right, _, _, err := s.TilePNG(ctx, "main", 2, 2, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	li, _ := png.Decode(bytes.NewReader(left))
	ri, _ := png.Decode(bytes.NewReader(right))
	// The same two tiles as one region render: pixels must match exactly.
	l := islandLayers(64)
	p := mapstyle.DefaultParams(42)
	ts := 2 * p.WorldRadius / 4
	wide, err := mapstyle.RenderRegion(ctx, l, p, nil, -p.WorldRadius+ts, p.WorldRadius-ts, -p.WorldRadius+3*ts, p.WorldRadius-2*ts, 2*TileSize, TileSize)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < TileSize; y += 31 {
		for x := 0; x < TileSize; x += 37 {
			if li.At(x, y) != wide.At(x, y) || ri.At(x, y) != wide.At(x+TileSize, y) {
				t.Fatalf("tile pixel %d,%d differs from the region render", x, y)
			}
		}
	}
}

func TestTiles_OldAgentUnsupported(t *testing.T) {
	paths := testPaths(t)
	installPlugin(t, paths, "1.9.0")
	cfg, err := EnsureConfig(paths, 2456)
	if err != nil {
		t.Fatal(err)
	}
	var state atomic.Value
	state.Store("ready")
	srv := mapAgent(t, cfg.Token, &state)
	fi := &fakeInstances{paths: paths, inst: domain.Instance{
		ID: "main", Config: domain.InstanceConfig{Port: 2456, BepInExEnabled: true},
		Status: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning},
	}}
	s := NewService(fi, &fakeBus{}, slog.New(slog.NewTextHandler(io.Discard, nil)), fixedVersion("1.10.0"))
	s.http = srv.Client()
	s.baseURL = func(int) string { return srv.URL }
	s.tick(context.Background())
	if _, _, _, err := s.TilePNG(context.Background(), "main", 0, 0, 0, true); domain.AsError(err).Code != domain.CodeConflict {
		t.Fatalf("old agent must be reported as unsupported, got %v", err)
	}
	m, _ := s.Map(context.Background(), "main", false)
	if m.Tiles != nil || m.LayersSupported {
		t.Fatalf("old agent must not advertise tiles: %+v", m)
	}
}

func TestPruneTileCache(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, "k", "1", "0", string(rune('a'+i))+".png")
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Now().Add(time.Duration(i-5) * time.Minute)
		_ = os.Chtimes(p, mt, mt)
	}
	pruneTileCache(dir, 350)
	left := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			left++
		}
		return nil
	})
	if left != 2 {
		t.Fatalf("expected the oldest tiles pruned to 80%% of the cap (2 left), got %d", left)
	}
	if _, err := os.Stat(filepath.Join(dir, "k", "1", "0", "e.png")); err != nil {
		t.Fatal("the newest tile must survive")
	}
}
