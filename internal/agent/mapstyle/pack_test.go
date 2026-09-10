package mapstyle

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePNG(t *testing.T, path string, c color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestPack_OverrideAndFallback(t *testing.T) {
	dir := t.TempDir()
	empty, err := LoadPack(dir)
	if err != nil || empty.Fingerprint() != "builtin" {
		t.Fatalf("empty dir: %v %q", err, empty.Fingerprint())
	}
	if p, _ := LoadPack(filepath.Join(dir, "missing")); p.Fingerprint() != "builtin" {
		t.Fatal("missing dir must be builtin")
	}

	writePNG(t, filepath.Join(dir, "meadows.png"), color.NRGBA{255, 0, 255, 255})
	if err := os.WriteFile(filepath.Join(dir, "plains.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), []byte(`{"metres_per_tile":{"meadows":100}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Fingerprint() == "builtin" || len(p.Fingerprint()) != 12 {
		t.Fatalf("fingerprint %q", p.Fingerprint())
	}
	if len(p.Problems) != 1 {
		t.Fatalf("corrupt plains.png must be reported once: %v", p.Problems)
	}
	if p.Tile(RolePlains) != builtinTile(RolePlains) {
		t.Fatal("corrupt file must fall back to the builtin tile")
	}
	if mt := p.Tile(RoleMeadows); mt == builtinTile(RoleMeadows) || mt.Metres != 100 {
		t.Fatalf("meadows override not used: %+v", mt)
	}

	// Rendered meadows come out magenta-ish; plains stay yellow.
	img, err := Render(context.Background(), synthLayers(256), DefaultParams(1), p)
	if err != nil {
		t.Fatal(err)
	}
	mx, my := worldToPx(256, -1500, -2600)
	if c := px(img, mx, my); !(c.R > 0.5 && c.B > 0.4 && c.G < 0.3) {
		t.Fatalf("meadows should use the magenta override: %+v", c)
	}
	// The fingerprint follows the files.
	fp1 := p.Fingerprint()
	time.Sleep(20 * time.Millisecond)
	writePNG(t, filepath.Join(dir, "meadows.png"), color.NRGBA{0, 255, 255, 255})
	fp2, err := Fingerprint(dir)
	if err != nil || fp2 == fp1 {
		t.Fatalf("fingerprint must change with the file: %q %q %v", fp1, fp2, err)
	}
	if CacheName(5, 64, p) != "styled-5-64-v1-"+fp1+".png" {
		t.Fatalf("cache name %q", CacheName(5, 64, p))
	}
	if (*Pack)(nil).Tile(RoleSnow) != builtinTile(RoleSnow) {
		t.Fatal("nil pack must serve builtin tiles")
	}
}
