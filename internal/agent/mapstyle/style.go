package mapstyle

import (
	"context"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
)

// Version changes whenever Render's output changes for the same inputs; it
// is part of every cache name so a restyle invalidates old images.
const Version = 1

// Params describe the world the layers were sampled from (from MapInfo).
type Params struct {
	Seed           int
	WorldRadius    float32
	PlayableRadius float32
	SeaLevel       float32
}

// DefaultParams are Valheim's constants.
func DefaultParams(seed int) Params {
	return Params{Seed: seed, WorldRadius: 10500, PlayableRadius: 10000, SeaLevel: 30}
}

// Forest thresholds: the game's forest factor is LOW where trees grow
// (Minimap/WorldGenerator.InForest is factor < 1.15; plains < 0.8).
const (
	meadowsForestBelow = 1.15
	meadowsForestSpan  = 0.45
	plainsForestBelow  = 0.8
	plainsForestSpan   = 0.35
)

// CacheName is the styled whole-map file for these inputs.
func CacheName(seed, size int, pack *Pack) string {
	return fmt.Sprintf("styled-%d-%d-v%d-%s.png", seed, size, Version, pack.Fingerprint())
}

// Render draws the whole world square at the layers' own resolution.
func Render(ctx context.Context, l *Layers, p Params, pack *Pack) (*image.RGBA, error) {
	r := p.WorldRadius
	return RenderRegion(ctx, l, p, pack, -r, r, r, -r, l.Size, l.Size)
}

// RenderRegion draws the world rectangle from (wx0, wz0) at the top-left
// (west, north) to (wx1, wz1) at the bottom-right into a w×h image. Any
// region and any size are valid, which is what the tile pyramid relies on.
func RenderRegion(ctx context.Context, l *Layers, p Params, pack *Pack, wx0, wz0, wx1, wz1 float32, w, h int) (*image.RGBA, error) {
	if l == nil || l.Size < MinLayerSize {
		return nil, errors.New("mapstyle: no layers")
	}
	if w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return nil, fmt.Errorf("mapstyle: bad output size %dx%d", w, h)
	}
	if p.WorldRadius <= 0 {
		p = DefaultParams(p.Seed)
	}
	st := newStyler(l, p, pack)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	mppX := (wx1 - wx0) / float32(w)
	mppZ := (wz0 - wz1) / float32(h)
	mpp := mppX
	if mppZ > mpp {
		mpp = mppZ
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > h {
		workers = h
	}
	band := 8
	if h < band*workers {
		band = 1
	}
	rows := make(chan int, h/band+1)
	for y := 0; y < h; y += band {
		rows <- y
	}
	close(rows)
	var wg sync.WaitGroup
	var cancelled error
	var once sync.Once
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y0 := range rows {
				if err := ctx.Err(); err != nil {
					once.Do(func() { cancelled = err })
					return
				}
				y1 := y0 + band
				if y1 > h {
					y1 = h
				}
				for y := y0; y < y1; y++ {
					wz := wz0 - (float32(y)+0.5)*mppZ
					row := out.Pix[y*out.Stride:]
					for x := 0; x < w; x++ {
						wx := wx0 + (float32(x)+0.5)*mppX
						c := st.pixel(wx, wz, mpp)
						i := x * 4
						row[i], row[i+1], row[i+2] = c.bytes()
						row[i+3] = 255
					}
				}
			}
		}()
	}
	wg.Wait()
	if cancelled != nil {
		return nil, cancelled
	}
	return out, nil
}

type styler struct {
	s    *sampler
	p    Params
	n    *noise
	ox   float32 // per-seed texture offsets so worlds do not share blotches
	oz   float32
	tile map[Role]*Tile
}

func newStyler(l *Layers, p Params, pack *Pack) *styler {
	st := &styler{s: newSampler(l, p.WorldRadius), p: p, n: baseNoise(), tile: map[Role]*Tile{}}
	st.ox = float32(p.Seed%4096) * 131
	st.oz = float32((p.Seed/4096)%4096) * 257
	for _, r := range AllRoles {
		st.tile[r] = pack.Tile(r)
	}
	return st
}

// tex samples a role at two scales to hide the repetition of the tile.
func (st *styler) tex(role Role, wx, wz float32) rgb {
	t := st.tile[role]
	r1, g1, b1, _ := t.Sample(wx+st.ox, wz+st.oz)
	r2, g2, b2, _ := t.Sample(wx*0.37+st.oz+911, wz*0.37+st.ox+377)
	return rgb{lerp(r1, r2, 0.35), lerp(g1, g2, 0.35), lerp(b1, b2, 0.35)}
}

// pixel is the colour of the map at a world position drawn with mpp metres
// per output pixel (used for line widths and the tree level of detail).
func (st *styler) pixel(wx, wz, mpp float32) rgb {
	dist := hypot32(wx, wz)
	if dist > st.p.WorldRadius+mpp {
		return abyss
	}
	h := st.s.heightAt(wx, wz)
	var c rgb
	if h < st.p.SeaLevel {
		c = st.water(wx, wz, h, mpp)
	} else {
		c = st.land(wx, wz, h, mpp)
	}
	if dist > st.p.PlayableRadius {
		f := smoothstep(st.p.PlayableRadius, st.p.WorldRadius, dist)
		c = mix(c, ringColour, 0.6*f)
	}
	return c
}

// strokeWidth is the coastline/beach width in metres: about a pixel, never
// thinner than the game's own line reads at deep zoom.
func strokeWidth(mpp float32) float32 {
	w := 1.2 * mpp
	if w < 3 {
		w = 3
	}
	return w
}

// landWithin reports whether land lies within d metres in a cardinal direction.
func (st *styler) landWithin(wx, wz, d float32) bool {
	sea := st.p.SeaLevel
	return st.s.heightAt(wx+d, wz) >= sea || st.s.heightAt(wx-d, wz) >= sea ||
		st.s.heightAt(wx, wz+d) >= sea || st.s.heightAt(wx, wz-d) >= sea
}

func (st *styler) waterWithin(wx, wz, d float32) bool {
	sea := st.p.SeaLevel
	return st.s.heightAt(wx+d, wz) < sea || st.s.heightAt(wx-d, wz) < sea ||
		st.s.heightAt(wx, wz+d) < sea || st.s.heightAt(wx, wz-d) < sea
}

func (st *styler) water(wx, wz, h, mpp float32) rgb {
	d := st.p.SeaLevel - h
	c := mix(waterShallow, waterDeep, smoothstep(0, 35, d))
	c = mix(c, waterAbyssal, smoothstep(35, 90, d))
	if d < 3 {
		c = mix(st.tex(RoleShallows, wx, wz), c, d/3)
	}
	// faint wave crests
	wr, _, _, _ := st.tile[RoleOcean].Sample(wx+st.ox, wz+st.oz)
	c = c.mul(1 + (wr-0.5)*0.10)
	// coastline stroke, with a softer second band
	sw := strokeWidth(mpp)
	if st.landWithin(wx, wz, sw) {
		c = c.mul(0.66)
	} else if st.landWithin(wx, wz, 2*sw) {
		c = c.mul(0.86)
	}
	return c
}

func (st *styler) land(wx, wz, h, mpp float32) rgb {
	f := st.s.forestAt(wx, wz)
	m := st.s.maskAt(wx, wz)
	d := mpp
	if d < st.s.step*0.75 {
		d = st.s.step * 0.75
	}
	shade, slope := st.s.relief(wx, wz, d)

	bs, n := st.s.biomes(wx, wz)
	var c rgb
	for i := 0; i < n; i++ {
		bc := st.biomeColour(bs[i].b, wx, wz, h, f, m, slope, mpp)
		c.R += bc.R * bs[i].w
		c.G += bc.G * bs[i].w
		c.B += bc.B * bs[i].w
	}
	sw := strokeWidth(mpp)
	if st.waterWithin(wx, wz, sw) {
		c = mix(c, beachColour, 0.55)
	}
	return c.mul(0.72 + 0.5*shade)
}

func (st *styler) biomeColour(b Biome, wx, wz, h, f, m, slope, mpp float32) rgb {
	switch b {
	case Meadows, Ocean, BiomeNone:
		c := st.tex(RoleMeadows, wx, wz)
		return st.trees(c, wx, wz, mpp, clamp01((meadowsForestBelow-f)/meadowsForestSpan), 1)
	case BlackForest:
		c := st.tex(RoleBlackForest, wx, wz)
		return st.trees(c, wx, wz, mpp, 1, 0.82)
	case Plains:
		c := st.tex(RolePlains, wx, wz)
		return st.treesTinted(c, wx, wz, mpp, clamp01((plainsForestBelow-f)/plainsForestSpan), plainsTree)
	case Swamp:
		c := st.tex(RoleSwamp, wx, wz)
		if h < st.p.SeaLevel+1.2 {
			c = mix(c, swampPool, smoothstep(st.p.SeaLevel+1.2, st.p.SeaLevel, h))
		}
		return st.treesTinted(c, wx, wz, mpp, 0.22, swampDark.mul(0.75))
	case Mountain:
		rock := st.tex(RoleMountain, wx, wz)
		snow := st.tex(RoleSnow, wx, wz)
		c := mix(rock, snow, smoothstep(95, 135, h))
		if slope > 0.9 {
			c = mix(c, rock, smoothstep(0.9, 1.3, slope))
		}
		return c
	case DeepNorth:
		c := st.tex(RoleDeepNorth, wx, wz)
		if slope > 0.7 {
			c = mix(c, rockBase.mul(0.9), smoothstep(0.7, 1.1, slope))
		}
		return c
	case Mistlands:
		c := st.tex(RoleMistlands, wx, wz)
		if m > 0.5 {
			c = c.mul(1 - 0.28*smoothstep(0.5, 0.9, m))
		}
		_, _, _, ma := st.tile[RoleMist].Sample(wx+st.ox, wz+st.oz)
		mist := ma * (1 - smoothstep(40, 80, h)) * 0.75
		return mix(c, mistColour, mist)
	case AshLands:
		c := st.tex(RoleAshlands, wx, wz)
		lr, lg, lb, la := st.tile[RoleLava].Sample(wx+st.ox, wz+st.oz)
		lava := smoothstep(0.35, 0.6, m) * la
		c = mix(c, rgb{lr, lg, lb}, lava)
		return mix(c, lavaEdge, smoothstep(0.15, 0.35, m)*0.35*(1-lava))
	case OffWorld:
		return abyss
	default:
		return rgb{0.48, 0.48, 0.44}
	}
}

// trees draws the forest over a ground colour. density 0..1 says how many of
// the tile's discs to show (by priority). At overview scale discs are smaller
// than a pixel, so the forest becomes a mottled darkening instead; the two
// cross-fade between 1.5 and 3 m per pixel. tone darkens the discs.
func (st *styler) trees(ground rgb, wx, wz, mpp, density, tone float32) rgb {
	return st.treesTinted(ground, wx, wz, mpp, density, treeDark.mul(tone))
}

func (st *styler) treesTinted(ground rgb, wx, wz, mpp, density float32, disc rgb) rgb {
	if density <= 0 {
		return ground
	}
	// Far: canopy as a darkening with its own mottling.
	mot := st.n.fbm((wx+st.ox)*0.02, (wz+st.oz)*0.02, 2)
	far := mix(ground, disc, density*(0.35+0.4*mot))
	if mpp >= 3 {
		return far
	}
	// Near: individual discs.
	near := ground
	r, g, b, a, prio := st.tile[RoleForestTree].SampleNearest(wx+st.ox, wz+st.oz)
	if a > 0 && prio < density {
		near = rgb{r, g, b}
		// keep the biome's tint: the tile is drawn for meadows
		near = mix(near, disc, 0.5)
	}
	if mpp <= 1.5 {
		return near
	}
	return mix(near, far, (mpp-1.5)/1.5)
}
