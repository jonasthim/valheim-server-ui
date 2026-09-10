package mapstyle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"testing"
	"time"
)

func px(img *image.RGBA, x, y int) rgb {
	i := img.PixOffset(x, y)
	return rgb{float32(img.Pix[i]) / 255, float32(img.Pix[i+1]) / 255, float32(img.Pix[i+2]) / 255}
}

// classify buckets a colour into the families the palette uses.
func classify(c rgb) string {
	l := c.lum()
	switch {
	case l > 0.85:
		return "white"
	case c.B > c.R && c.B > c.G && l < 0.55:
		return "blue"
	case c.R > 0.4 && c.R > c.G*1.6 && c.R > c.B*1.6:
		return "red"
	case c.R > 0.6 && c.G > 0.55 && c.B < 0.5 && c.R >= c.G:
		return "yellow"
	case c.G > c.R && c.G > c.B && l > 0.45:
		return "green"
	case c.G >= c.R*0.95 && c.G > c.B && l <= 0.45:
		return "dark-green"
	case abs32(c.R-c.G) < 0.08 && abs32(c.G-c.B) < 0.08:
		return "grey"
	default:
		return "other"
	}
}

// worldToPx maps world metres to pixel coordinates of a full render.
func worldToPx(size int, wx, wz float32) (int, int) {
	r := float32(10500)
	return int((wx + r) / (2 * r) * float32(size)), int((r - wz) / (2 * r) * float32(size))
}

func renderSynth(t *testing.T, size int) *image.RGBA {
	t.Helper()
	img, err := Render(context.Background(), synthLayers(size), DefaultParams(42), nil)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestRender_PaletteClasses(t *testing.T) {
	const size = 512
	img := renderSynth(t, size)
	cases := []struct {
		name   string
		wx, wz float32
		want   string
	}{
		{"meadows", -1500, -2600, "green"},
		{"black forest", -1800, 1500, "dark-green"},
		{"plains open", 2400, -700, "yellow"},
		{"ocean", -6000, -6000, "blue"},
		{"mountain snow", 1500, 1500, "white"},
		{"ashlands", -2900, -300, "red"},
		{"deep north", 0, 9800, "white"},
		{"off-world", 10400, 10400, "blue"},
	}
	for _, c := range cases {
		x, y := worldToPx(size, c.wx, c.wz)
		got := classify(px(img, x, y))
		if got != c.want {
			t.Errorf("%s at (%v,%v): %s, want %s (%+v)", c.name, c.wx, c.wz, got, c.want, px(img, x, y))
		}
	}
}

func TestRender_CoastlineStrokeAndRiver(t *testing.T) {
	const size = 1024
	img := renderSynth(t, size)
	// The lake: water just inside the shore is darker than water at the centre.
	cx, cy := worldToPx(size, -1800, -1600)
	// walk east from the centre until land, remember the last water pixel
	var last, mid rgb
	mid = px(img, cx, cy)
	for x := cx; x < size; x++ {
		c := px(img, x, cy)
		if classify(c) != "blue" && c.lum() > 0.5 {
			break
		}
		last = c
	}
	if last.lum() > mid.lum()*0.8 {
		t.Fatalf("no coastline stroke: shore %.3f vs centre %.3f", last.lum(), mid.lum())
	}
	// The river channel (x in −1800..−1750, south of the lake) shows as water
	// through the meadows: not green like the bank beside it.
	rx, ry := worldToPx(size, -1775, -2600)
	river := px(img, rx, ry)
	bank := px(img, rx+6, ry)
	if river.G-river.B > 0.12 || bank.G-bank.B < 0.15 {
		t.Fatalf("river not drawn as water: river=%+v bank=%+v", river, bank)
	}
}

func TestRender_TreesFollowForestFactor(t *testing.T) {
	// A near-scale region (0.5 m/px) inside the plains wedge: the forested
	// half must contain tree discs, the open half none.
	l := synthLayers(1024)
	p := DefaultParams(42)
	count := func(wz float32) int {
		img, err := RenderRegion(context.Background(), l, p, nil, 3200, wz+128, 3456, wz-128, 512, 512)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for y := 0; y < 512; y++ {
			for x := 0; x < 512; x++ {
				if classify(px(img, x, y)) == "dark-green" {
					n++
				}
			}
		}
		return n
	}
	forested, open := count(600), count(-900)
	if forested < 2000 {
		t.Fatalf("forested plains half has no tree discs (%d dark pixels)", forested)
	}
	if open > forested/20 {
		t.Fatalf("open plains half should have no trees: %d vs %d", open, forested)
	}
}

func TestRender_HillshadeLightsTheNorthWest(t *testing.T) {
	const size = 1024
	img := renderSynth(t, size)
	// The mountain cone is centred at (1500, 1500), radius 1600 m. Compare the
	// NW flank with the SE flank at the same distance (same height, same snow).
	nwx, nwy := worldToPx(size, 1500-800, 1500+800)
	sex, sey := worldToPx(size, 1500+800, 1500-800)
	nw, se := px(img, nwx, nwy), px(img, sex, sey)
	if nw.lum() <= se.lum()*1.05 {
		t.Fatalf("north-west flank should be brighter: nw=%.3f se=%.3f", nw.lum(), se.lum())
	}
}

// knownHashes pins the output per style Version. When the look changes on
// purpose, bump Version and add its hash here; a changed hash under the same
// Version means an unintended change.
var knownHashes = map[int]string{
	1: "4deb97d63748af2a6e261c107605eb06fa6b9ee6c66f27ce7de7b44cd6b2e874",
}

func TestRender_Deterministic(t *testing.T) {
	img := renderSynth(t, 256)
	again := renderSynth(t, 256)
	sum := sha256.Sum256(img.Pix)
	if sha256.Sum256(again.Pix) != sum {
		t.Fatal("render is not deterministic")
	}
	got := hex.EncodeToString(sum[:])
	want, ok := knownHashes[Version]
	if !ok {
		t.Fatalf("no recorded hash for style Version %d; add %q to knownHashes", Version, got)
	}
	if got != want {
		t.Fatalf("output changed for Version %d: %s (bump Version and record the new hash if intended)", Version, got)
	}
}

func TestRender_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Render(ctx, synthLayers(256), DefaultParams(1), nil); err == nil {
		t.Fatal("cancelled context must fail the render")
	}
}

func TestRenderRegion_MatchesFullRenderAndIsSeamless(t *testing.T) {
	l := synthLayers(256)
	p := DefaultParams(7)
	full, err := Render(context.Background(), l, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The top-left quadrant rendered as a region at the same scale is the
	// same pixels as the full render's quadrant.
	r := p.WorldRadius
	quad, err := RenderRegion(context.Background(), l, p, nil, -r, r, 0, 0, 128, 128)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 128; y += 7 {
		for x := 0; x < 128; x += 7 {
			if px(quad, x, y) != px(full, x, y) {
				t.Fatalf("region differs from full render at %d,%d", x, y)
			}
		}
	}
	// Two adjacent tiles share their boundary: the last column of the left
	// tile continues into the first column of the right one (no seam, i.e.
	// no duplicated or skipped pixel column).
	left, _ := RenderRegion(context.Background(), l, p, nil, -1000, 1000, 0, 0, 100, 100)
	right, _ := RenderRegion(context.Background(), l, p, nil, 0, 1000, 1000, 0, 100, 100)
	wide, _ := RenderRegion(context.Background(), l, p, nil, -1000, 1000, 1000, 0, 200, 100)
	for y := 0; y < 100; y += 9 {
		if px(left, 99, y) != px(wide, 99, y) || px(right, 0, y) != px(wide, 100, y) {
			t.Fatalf("tile seam at row %d", y)
		}
	}
}

func TestRender_Sizes(t *testing.T) {
	for _, size := range []int{MinLayerSize, 256} {
		if _, err := Render(context.Background(), synthLayers(size), DefaultParams(3), nil); err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
	}
	if testing.Short() {
		return
	}
	// 1024² keeps the race-detector run short; BenchmarkRender2048 measures the real size.
	start := time.Now()
	if _, err := Render(context.Background(), synthLayers(1024), DefaultParams(3), nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("1024² render: %s", time.Since(start))
}

func BenchmarkRender2048(b *testing.B) {
	l := synthLayers(2048)
	p := DefaultParams(3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Render(context.Background(), l, p, nil); err != nil {
			b.Fatal(err)
		}
	}
}
