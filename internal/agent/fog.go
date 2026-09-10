package agent

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
)

// fogColor is what unexplored terrain is painted with in the served image.
// It matches the UI's night tone so the fog reads as part of the map.
var fogColor = color.NRGBA{R: 0x0b, G: 0x0e, B: 0x14, A: 0xff}

// compositeFog paints the fog mask over the map image and returns the result.
// The mask is the plugin's grey+alpha PNG (255 = unexplored) at any size; it
// is sampled bilinearly over the image, so a 1024² mask on a 2048² map gives
// soft edges instead of 2 px steps. The base is left untouched.
func compositeFog(base, mask image.Image) *image.NRGBA {
	b := base.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	fog := maskReader(mask)
	mw, mh := mask.Bounds().Dx(), mask.Bounds().Dy()
	if w == 0 || h == 0 || mw == 0 || mh == 0 {
		return out
	}

	// Per-column sample positions, computed once.
	type axis struct {
		i0, i1 int
		f      float32
	}
	xs := make([]axis, w)
	for x := 0; x < w; x++ {
		xs[x] = sampleAxis(x, w, mw)
	}

	src := pixelReader(base)
	fr, fg, fb := float32(fogColor.R), float32(fogColor.G), float32(fogColor.B)
	for y := 0; y < h; y++ {
		ya := sampleAxis(y, h, mh)
		row0, row1 := ya.i0*mw, ya.i1*mw
		for x := 0; x < w; x++ {
			xa := xs[x]
			top := float32(fog(row0+xa.i0))*(1-xa.f) + float32(fog(row0+xa.i1))*xa.f
			bot := float32(fog(row1+xa.i0))*(1-xa.f) + float32(fog(row1+xa.i1))*xa.f
			a := (top*(1-ya.f) + bot*ya.f) / 255
			r, g, bb := src(b.Min.X+x, b.Min.Y+y)
			o := out.Pix[y*out.Stride+x*4:]
			o[0] = uint8(float32(r)*(1-a) + fr*a + 0.5)
			o[1] = uint8(float32(g)*(1-a) + fg*a + 0.5)
			o[2] = uint8(float32(bb)*(1-a) + fb*a + 0.5)
			o[3] = 0xff
		}
	}
	return out
}

// sampleAxis maps output index i of n to the two mask indices (of m) it lies
// between and the weight of the second one.
func sampleAxis(i, n, m int) struct {
	i0, i1 int
	f      float32
} {
	pos := (float32(i)+0.5)*float32(m)/float32(n) - 0.5
	if pos < 0 {
		pos = 0
	}
	i0 := int(pos)
	if i0 > m-1 {
		i0 = m - 1
	}
	i1 := i0 + 1
	if i1 > m-1 {
		i1 = m - 1
	}
	return struct {
		i0, i1 int
		f      float32
	}{i0, i1, pos - float32(i0)}
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
func writeFoggedPNG(basePath, maskPath, outPath string) error {
	base, err := decodePNG(basePath)
	if err != nil {
		return fmt.Errorf("map image: %w", err)
	}
	mask, err := decodePNG(maskPath)
	if err != nil {
		return fmt.Errorf("fog mask: %w", err)
	}
	img := compositeFog(base, mask)
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
