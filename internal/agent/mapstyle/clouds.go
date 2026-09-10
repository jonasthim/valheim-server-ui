package mapstyle

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

// CloudsPNG is a seamless 512² grey+alpha cloud texture the UI scrolls over
// the unexplored parchment (and, faintly, over explored water), so the fog
// drifts as it does on the in-game map. Generated once per process.
func CloudsPNG() []byte {
	cloudsOnce.Do(func() {
		const size = 512
		n := newNoise(0xc10d)
		img := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				// Periodic by construction: the lattice repeats every 256 units
				// and the tile spans exactly two periods at the base octave.
				fx := float32(x) / size * 512 / 64
				fy := float32(y) / size * 512 / 64
				v := n.fbm(fx, fy, 4)
				a := smoothstep(0.42, 0.78, v)
				g := uint8(200 + 40*v)
				img.SetNRGBA(x, y, color.NRGBA{g, g, g, uint8(a*255 + 0.5)})
			}
		}
		t := tileFromImage(img, 1)
		makeSeamless(t)
		out := image.NewNRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				r, _, _, a := t.get(x, y)
				out.SetNRGBA(x, y, color.NRGBA{toByte(r), toByte(r), toByte(r), toByte(a)})
			}
		}
		var buf bytes.Buffer
		_ = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&buf, out)
		cloudsBytes = buf.Bytes()
	})
	return cloudsBytes
}

var (
	cloudsOnce  sync.Once
	cloudsBytes []byte
)

// WaterMask is a size² grey image, 255 where the map shows explored water
// (0 under the fog, on land and off-world), used by the UI to confine the
// water shimmer. mask may be nil when the agent has no fog: all water then.
func WaterMask(l *Layers, p Params, mask image.Image, size int) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, size, size))
	if l == nil || size <= 0 {
		return out
	}
	if p.WorldRadius <= 0 {
		p = DefaultParams(p.Seed)
	}
	s := newSampler(l, p.WorldRadius)
	fog := maskSampler(mask)
	mpp := 2 * p.WorldRadius / float32(size)
	for y := 0; y < size; y++ {
		wz := p.WorldRadius - (float32(y)+0.5)*mpp
		v := (float32(y) + 0.5) / float32(size)
		for x := 0; x < size; x++ {
			wx := -p.WorldRadius + (float32(x)+0.5)*mpp
			if hypot32(wx, wz) > p.WorldRadius {
				continue
			}
			h := s.heightAt(wx, wz)
			if h >= p.SeaLevel {
				continue
			}
			u := (float32(x) + 0.5) / float32(size)
			explored := 1 - fog(u, v)
			// fade out over the first metres of depth so the shore stays calm
			depth := smoothstep(0.5, 4, p.SeaLevel-h)
			out.Pix[y*out.Stride+x] = toByte(explored * depth)
		}
	}
	return out
}

// maskSampler reads the fog mask (255 = unexplored) bilinearly at map
// fractions; without a mask everything counts as explored.
func maskSampler(mask image.Image) func(u, v float32) float32 {
	if mask == nil {
		return func(u, v float32) float32 { return 0 }
	}
	b := mask.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return func(u, v float32) float32 { return 0 }
	}
	at := func(x, y int) float32 {
		if x < 0 {
			x = 0
		} else if x >= w {
			x = w - 1
		}
		if y < 0 {
			y = 0
		} else if y >= h {
			y = h - 1
		}
		switch t := mask.(type) {
		case *image.NRGBA:
			return float32(t.Pix[y*t.Stride+x*4+3]) / 255
		case *image.Gray:
			return float32(t.Pix[y*t.Stride+x]) / 255
		default:
			_, _, _, a := mask.At(b.Min.X+x, b.Min.Y+y).RGBA()
			return float32(a) / 65535
		}
	}
	return func(u, v float32) float32 {
		px := u*float32(w) - 0.5
		py := v*float32(h) - 0.5
		fx, fy := floor32(px), floor32(py)
		tx, ty := px-fx, py-fy
		ix, iy := int(fx), int(fy)
		return lerp(lerp(at(ix, iy), at(ix+1, iy), tx), lerp(at(ix, iy+1), at(ix+1, iy+1), tx), ty)
	}
}
