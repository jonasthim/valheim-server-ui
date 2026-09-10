package mapstyle

import (
	"math"
	"testing"
)

// synthLayers builds a deterministic world for tests and previews: an island
// in the ocean with a lake and a river, wedges of black forest and plains
// (one half forested, one half open), a mountain cone, a swamp, mistlands
// with mask spots, ashlands with a mask vein, a deep-north cap, and the
// off-world ring.
func synthLayers(size int) *Layers {
	l := NewLayers(size)
	R := float32(10500)
	step := 2 * R / float32(size)
	for y := 0; y < size; y++ {
		wz := R - (float32(y)+0.5)*step
		for x := 0; x < size; x++ {
			wx := -R + (float32(x)+0.5)*step
			dist := hypot32(wx, wz)
			if dist > R {
				l.Set(x, y, OffWorld, 0, -20, 0)
				continue
			}
			ang := float32(math.Atan2(float64(wz), float64(wx))) // -π..π
			// island: radius varies with angle so the coast is irregular
			islandR := 4200 + 900*float32(math.Sin(float64(ang)*3)) + 500*float32(math.Cos(float64(ang)*7))
			if dist > islandR {
				depth := (dist - islandR) / 60
				h := 30 - depth
				if h < -60 {
					h = -60
				}
				b := Ocean
				if wz > 9000 {
					b = DeepNorth
					h = 45 + (wz-9000)/40
				}
				l.Set(x, y, b, 0, h, 2.0)
				continue
			}
			// land height: rises toward the centre, with a mountain cone NE
			// whose height depends on the distance to its peak only (so the
			// hill-shade test compares equal heights on opposite flanks)
			h := 32 + (islandR-dist)/40
			mx, mz := float32(1500), float32(1500)
			md := hypot32(wx-mx, wz-mz)
			if md < 1600 {
				h = 40 + (1600-md)/8
			}
			b := Meadows
			f := float32(1.5) // open meadows by default
			mask := float32(0)
			switch {
			case ang > 2.0 && ang < 2.9: // black forest wedge, NW
				b, f = BlackForest, 0.4
			case ang > -0.6 && ang < 0.4: // plains wedge, E
				b = Plains
				if wz > -100 {
					f = 0.5 // forested half
				} else {
					f = 1.6
				}
			case ang > -1.5 && ang < -0.9 && dist > 2500: // swamp, SSE
				b = Swamp
				h = 31
			case ang > 0.9 && ang < 1.5 && dist > 2600: // mistlands, N
				b = Mistlands
				mask = 0
				if int(wx/150)%3 == 0 && int(wz/150)%2 == 0 {
					mask = 0.9
				}
			case ang > -3.1 && ang < -2.6: // ashlands, W
				b = AshLands
				mask = 0
				if abs32(wz-(wx+3000)*0.3) < 60 {
					mask = 1
				}
			}
			if md < 1600 {
				b = Mountain
			}
			// lake SW and a river from it straight south to the coast
			if hypot32(wx+1800, wz+1600) < 600 {
				h = 26
			}
			if wx > -1800 && wx < -1750 && wz < -1600 {
				h = 28
			}
			l.Set(x, y, b, mask, h, f)
		}
	}
	return l
}

func TestLayersPacking(t *testing.T) {
	l := NewLayers(16)
	cases := []struct {
		b    Biome
		mask float32
		h    float32
		f    float32
	}{
		{Meadows, 0, 30, 1.15}, {Ocean, 0.5, -200, 0}, {OffWorld, 1, 500, 2.55}, {AshLands, 1, 0, 0.8},
	}
	for i, c := range cases {
		l.Set(i, 0, c.b, c.mask, c.h, c.f)
		if l.Biome(i, 0) != c.b {
			t.Fatalf("biome %d: got %v want %v", i, l.Biome(i, 0), c.b)
		}
		if got := l.Mask(i, 0); abs32(got-c.mask) > 0.04 {
			t.Fatalf("mask %d: got %v want %v", i, got, c.mask)
		}
		if got := l.Height(i, 0); abs32(got-c.h) > 0.02 {
			t.Fatalf("height %d: got %v want %v", i, got, c.h)
		}
		if got := l.Forest(i, 0); abs32(got-c.f) > 0.006 {
			t.Fatalf("forest %d: got %v want %v", i, got, c.f)
		}
	}
	// Round trip through the PNG encoding the plugin writes.
	png, err := EncodeLayers(l)
	if err != nil {
		t.Fatal(err)
	}
	back, err := DecodeLayers(bytesReader(png))
	if err != nil {
		t.Fatal(err)
	}
	for i := range l.Pix {
		if l.Pix[i] != back.Pix[i] {
			t.Fatalf("byte %d changed through PNG: %d -> %d", i, l.Pix[i], back.Pix[i])
		}
	}
}
