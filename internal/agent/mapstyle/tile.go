package mapstyle

import "image"

// Tile is a texture that repeats in world space every Metres metres, stored
// as float RGBA. Aux carries per-pixel side data where a role needs it (tree
// discs store their priority there so density can be thresholded).
type Tile struct {
	W, H   int
	Pix    []float32 // RGBA, 0..1, non-premultiplied
	Aux    []float32 // optional, len W*H
	Metres float32
}

func newTile(w, h int, metres float32) *Tile {
	return &Tile{W: w, H: h, Pix: make([]float32, w*h*4), Metres: metres}
}

func (t *Tile) set(x, y int, r, g, b, a float32) {
	i := (y*t.W + x) * 4
	t.Pix[i], t.Pix[i+1], t.Pix[i+2], t.Pix[i+3] = r, g, b, a
}

func (t *Tile) get(x, y int) (r, g, b, a float32) {
	i := (y*t.W + x) * 4
	return t.Pix[i], t.Pix[i+1], t.Pix[i+2], t.Pix[i+3]
}

func wrap(i, n int) int {
	i %= n
	if i < 0 {
		i += n
	}
	return i
}

// Sample reads the tile at a world position with bilinear filtering.
func (t *Tile) Sample(wx, wz float32) (r, g, b, a float32) {
	u := wx / t.Metres * float32(t.W)
	v := wz / t.Metres * float32(t.H)
	fu, fv := floor32(u), floor32(v)
	tu, tv := u-fu, v-fv
	x0, y0 := wrap(int(fu), t.W), wrap(int(fv), t.H)
	x1, y1 := wrap(x0+1, t.W), wrap(y0+1, t.H)
	r00, g00, b00, a00 := t.get(x0, y0)
	r10, g10, b10, a10 := t.get(x1, y0)
	r01, g01, b01, a01 := t.get(x0, y1)
	r11, g11, b11, a11 := t.get(x1, y1)
	r = lerp(lerp(r00, r10, tu), lerp(r01, r11, tu), tv)
	g = lerp(lerp(g00, g10, tu), lerp(g01, g11, tu), tv)
	b = lerp(lerp(b00, b10, tu), lerp(b01, b11, tu), tv)
	a = lerp(lerp(a00, a10, tu), lerp(a01, a11, tu), tv)
	return
}

// SampleNearest reads the nearest texel (crisp shapes such as tree discs)
// and its Aux value.
func (t *Tile) SampleNearest(wx, wz float32) (r, g, b, a, aux float32) {
	x := wrap(int(floor32(wx/t.Metres*float32(t.W))), t.W)
	y := wrap(int(floor32(wz/t.Metres*float32(t.H))), t.H)
	r, g, b, a = t.get(x, y)
	if t.Aux != nil {
		aux = t.Aux[y*t.W+x]
	}
	return
}

// tileFromImage converts a decoded PNG into a tile. Alpha is kept; RGB is
// not premultiplied.
func tileFromImage(img image.Image, metres float32) *Tile {
	b := img.Bounds()
	t := newTile(b.Dx(), b.Dy(), metres)
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			r, g, bb, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a == 0 {
				t.set(x, y, 0, 0, 0, 0)
				continue
			}
			// RGBA() is premultiplied 16-bit; undo the multiplication.
			af := float32(a)
			t.set(x, y, float32(r)/af, float32(g)/af, float32(bb)/af, af/65535)
		}
	}
	return t
}
