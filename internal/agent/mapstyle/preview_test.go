package mapstyle

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"testing"
	"time"
)

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// TestStylePreview writes a styled preview for a look check. It is skipped
// unless MAPSTYLE_PREVIEW names the output file. MAPSTYLE_LAYERS may point at
// a real layers-<seed>-<size>.png copied from a server's cache; otherwise the
// synthetic world is used. MAPSTYLE_ZOOM=<z> MAPSTYLE_AT=<wx>,<wz> renders a
// 1024 px tile-like region at that zoom around a point instead.
func TestStylePreview(t *testing.T) {
	out := os.Getenv("MAPSTYLE_PREVIEW")
	if out == "" {
		t.Skip("set MAPSTYLE_PREVIEW=<path.png> to write a preview")
	}
	var l *Layers
	if src := os.Getenv("MAPSTYLE_LAYERS"); src != "" {
		f, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		l, err = DecodeLayers(f)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		size := 1024
		if v, err := strconv.Atoi(os.Getenv("MAPSTYLE_SIZE")); err == nil {
			size = v
		}
		l = synthLayers(size)
	}
	p := DefaultParams(42)
	var img *image.RGBA
	var err error
	start := time.Now()
	if z := os.Getenv("MAPSTYLE_ZOOM"); z != "" {
		zoom, _ := strconv.Atoi(z)
		var cx, cz float32
		if at := os.Getenv("MAPSTYLE_AT"); at != "" {
			var a, b float64
			if _, serr := fmtSscanf(at, &a, &b); serr == nil {
				cx, cz = float32(a), float32(b)
			}
		}
		half := p.WorldRadius / float32(int(1)<<uint(zoom)) * 2 // a 1024 px window = 4 tiles
		img, err = RenderRegion(context.Background(), l, p, nil, cx-half, cz+half, cx+half, cz-half, 1024, 1024)
	} else {
		img, err = Render(context.Background(), l, p, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered %dx%d in %s", img.Bounds().Dx(), img.Bounds().Dy(), time.Since(start))
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func fmtSscanf(s string, a, b *float64) (int, error) {
	return fmt.Sscanf(s, "%g,%g", a, b)
}
