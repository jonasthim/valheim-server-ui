package mapstyle

import (
	"math"
	"sync"
)

// noise is seeded value noise on a 256² lattice with smooth interpolation.
// It is cheap, deterministic and tiles at 256 units, which is all the map
// textures need.
type noise struct {
	tab [256 * 256]float32
}

func newNoise(seed uint64) *noise {
	n := &noise{}
	s := seed*0x9E3779B97F4A7C15 + 0x632BE59BD9B4E019
	for i := range n.tab {
		// xorshift64*
		s ^= s >> 12
		s ^= s << 25
		s ^= s >> 27
		n.tab[i] = float32((s*0x2545F4914F6CDD1D)>>40) / float32(1<<24)
	}
	return n
}

func (n *noise) at(ix, iy int) float32 {
	return n.tab[(iy&255)<<8|(ix&255)]
}

// value samples the lattice with smoothstep weights; output 0..1.
func (n *noise) value(x, y float32) float32 {
	fx := float32(math.Floor(float64(x)))
	fy := float32(math.Floor(float64(y)))
	ix, iy := int(fx), int(fy)
	tx := smooth(x - fx)
	ty := smooth(y - fy)
	a := n.at(ix, iy)
	b := n.at(ix+1, iy)
	c := n.at(ix, iy+1)
	d := n.at(ix+1, iy+1)
	return lerp(lerp(a, b, tx), lerp(c, d, tx), ty)
}

// fbm sums octaves of value noise (lacunarity 2, gain 0.5), normalised 0..1.
func (n *noise) fbm(x, y float32, octaves int) float32 {
	var sum, amp, norm float32 = 0, 0.5, 0
	for i := 0; i < octaves; i++ {
		sum += amp * n.value(x, y)
		norm += amp
		amp *= 0.5
		x = x*2 + 17.3
		y = y*2 + 31.7
	}
	return sum / norm
}

// ridged turns fbm into sharp veins: 1 at the ridge, falling off both sides.
func (n *noise) ridged(x, y float32, octaves int) float32 {
	v := n.fbm(x, y, octaves)
	return 1 - abs32(2*v-1)
}

var baseNoise = sync.OnceValue(func() *noise { return newNoise(0x5eed) })

func smooth(t float32) float32 { return t * t * (3 - 2*t) }

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func smoothstep(e0, e1, x float32) float32 {
	if e1 == e0 {
		if x < e0 {
			return 0
		}
		return 1
	}
	t := clamp01((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func hypot32(x, y float32) float32 { return float32(math.Hypot(float64(x), float64(y))) }

func floor32(x float32) float32 { return float32(math.Floor(float64(x))) }
