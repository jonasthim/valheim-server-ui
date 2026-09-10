package agent

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
)

// compositeFog paints the fog of war over the map image the way the game
// does: unexplored terrain becomes parchment, the boundary is a soft cloudy
// edge rather than a smooth contour (the mask threshold is perturbed by slow
// noise), and the explored side of the edge carries a faint shadow. The mask
// is the plugin's grey+alpha PNG (255 = unexplored) at any size, sampled
// bilinearly over the image. A fully unexplored mask cell always ends up
// fully parchment (the noise can never expose it), and a fully explored one
// keeps the map untouched. The base is left as is.
func compositeFog(base, mask image.Image, fog *mapstyle.Parchment, worldRadius float32) *image.NRGBA {
	if worldRadius <= 0 {
		worldRadius = 10500
	}
	return compositeFogRect(base, mask, fog, worldRadius, -worldRadius, worldRadius, worldRadius, -worldRadius)
}

// compositeFogRect is compositeFog for a base image that covers only the
// world rectangle from (wx0, wz0) (west, north) to (wx1, wz1): what a map
// tile needs. The mask always covers the whole world square.
func compositeFogRect(base, mask image.Image, fog *mapstyle.Parchment, worldRadius, wx0, wz0, wx1, wz1 float32) *image.NRGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	maskAt := maskReader(mask)
	mw, mh := mask.Bounds().Dx(), mask.Bounds().Dy()
	if w == 0 || h == 0 || mw == 0 || mh == 0 {
		return out
	}
	if fog == nil {
		fog = mapstyle.NewParchment(0, nil)
	}
	if worldRadius <= 0 {
		worldRadius = 10500
	}

	// Mask cell coordinates of each output column/row (bilinear taps).
	type axis struct {
		i0, i1 int
		f      float32
	}
	tap := func(world, n float32) axis {
		pos := world*n - 0.5
		if pos < 0 {
			pos = 0
		}
		i0 := int(pos)
		if i0 > int(n)-1 {
			i0 = int(n) - 1
		}
		i1 := i0 + 1
		if i1 > int(n)-1 {
			i1 = int(n) - 1
		}
		return axis{i0, i1, pos - float32(i0)}
	}
	mppX := (wx1 - wx0) / float32(w)
	mppZ := (wz0 - wz1) / float32(h)
	xs := make([]axis, w)
	wxs := make([]float32, w)
	for x := 0; x < w; x++ {
		wxs[x] = wx0 + (float32(x)+0.5)*mppX
		xs[x] = tap((wxs[x]+worldRadius)/(2*worldRadius), float32(mw))
	}

	src := pixelReader(base)
	for y := 0; y < h; y++ {
		wz := wz0 - (float32(y)+0.5)*mppZ
		ya := tap((worldRadius-wz)/(2*worldRadius), float32(mh))
		row0, row1 := ya.i0*mw, ya.i1*mw
		for x := 0; x < w; x++ {
			xa := xs[x]
			top := float32(maskAt(row0+xa.i0))*(1-xa.f) + float32(maskAt(row0+xa.i1))*xa.f
			bot := float32(maskAt(row1+xa.i0))*(1-xa.f) + float32(maskAt(row1+xa.i1))*xa.f
			a := (top*(1-ya.f) + bot*ya.f) / 255
			wx := wxs[x]
			r, g, bb := src(b.Min.X+x, b.Min.Y+y)
			o := out.Pix[y*out.Stride+x*4:]
			if a <= 0 {
				o[0], o[1], o[2], o[3] = r, g, bb, 0xff
				continue
			}
			// Cloudy edge: the noise shifts where the boundary falls, never
			// beyond the fully explored or fully unexplored extremes.
			a2 := smoothstep(0.30, 0.70, a+0.30*fog.EdgeNoise(wx, wz))
			// Shadow on the explored side of the edge.
			sh := (1 - a2) * smoothstep(0.02, 0.45, a) * 0.28
			pr, pg, pb := fog.At(wx, wz)
			fr := float32(r) / 255 * (1 - sh)
			fg := float32(g) / 255 * (1 - sh)
			fb := float32(bb) / 255 * (1 - sh)
			o[0] = toByte(fr + (pr-fr)*a2)
			o[1] = toByte(fg + (pg-fg)*a2)
			o[2] = toByte(fb + (pb-fb)*a2)
			o[3] = 0xff
		}
	}
	return out
}

func smoothstep(e0, e1, x float32) float32 {
	t := (x - e0) / (e1 - e0)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

func toByte(f float32) uint8 {
	if f <= 0 {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return uint8(f*255 + 0.5)
}

// maskReader returns the fog value (0..255) at a linear mask index. The
// plugin writes grey+alpha with both channels equal; Go decodes that as
// NRGBA, so the alpha byte is read directly; other layouts go through At.
func maskReader(m image.Image) func(i int) uint8 {
	w := m.Bounds().Dx()
	switch t := m.(type) {
	case *image.NRGBA:
		return func(i int) uint8 { return t.Pix[(i/w)*t.Stride+(i%w)*4+3] }
	case *image.Gray:
		return func(i int) uint8 { return t.Pix[(i/w)*t.Stride+(i%w)] }
	default:
		min := m.Bounds().Min
		return func(i int) uint8 {
			_, _, _, a := m.At(min.X+i%w, min.Y+i/w).RGBA()
			return uint8(a >> 8) //nolint:gosec // RGBA() is 16-bit; >>8 fits a byte
		}
	}
}

// pixelReader returns the 8-bit RGB of a base pixel; fast paths for the
// layouts image/png produces for RGB and RGBA files.
func pixelReader(m image.Image) func(x, y int) (r, g, b uint8) {
	switch t := m.(type) {
	case *image.RGBA:
		return func(x, y int) (uint8, uint8, uint8) {
			p := t.Pix[t.PixOffset(x, y):]
			return p[0], p[1], p[2]
		}
	case *image.NRGBA:
		return func(x, y int) (uint8, uint8, uint8) {
			p := t.Pix[t.PixOffset(x, y):]
			return p[0], p[1], p[2]
		}
	default:
		return func(x, y int) (uint8, uint8, uint8) {
			r, g, b, _ := m.At(x, y).RGBA()
			return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8) //nolint:gosec // RGBA() is 16-bit; >>8 fits a byte
		}
	}
}

// writeFoggedPNG composites basePath under maskPath into outPath, written
// atomically so a concurrent reader never sees a partial file.
func writeFoggedPNG(basePath, maskPath, outPath string, fog *mapstyle.Parchment, worldRadius float32) error {
	base, err := decodePNG(basePath)
	if err != nil {
		return fmt.Errorf("map image: %w", err)
	}
	mask, err := decodePNG(maskPath)
	if err != nil {
		return fmt.Errorf("fog mask: %w", err)
	}
	img := compositeFog(base, mask, fog, worldRadius)
	tmp := outPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640) //nolint:gosec // cache file in the instance dir
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(f, img); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("encode fogged map: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path) //nolint:gosec // cache file the service wrote
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(io.Reader(f))
	if err != nil {
		return nil, err
	}
	return img, nil
}
