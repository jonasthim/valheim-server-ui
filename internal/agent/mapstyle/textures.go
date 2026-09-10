package mapstyle

import "sync"

// Role names one texture slot of the style. Each has a builtin procedural
// tile and may be replaced by a PNG of the same name in a texture pack.
type Role string

const (
	RoleParchment   Role = "parchment"
	RoleMeadows     Role = "meadows"
	RoleBlackForest Role = "blackforest"
	RoleForestTree  Role = "forest_tree"
	RoleSwamp       Role = "swamp"
	RoleMountain    Role = "mountain"
	RoleSnow        Role = "snow"
	RolePlains      Role = "plains"
	RoleMistlands   Role = "mistlands"
	RoleMist        Role = "mist"
	RoleAshlands    Role = "ashlands"
	RoleLava        Role = "lava"
	RoleDeepNorth   Role = "deepnorth"
	RoleOcean       Role = "ocean"
	RoleShallows    Role = "shallows"
)

// AllRoles in a fixed order (the pack fingerprint depends on it).
var AllRoles = [...]Role{
	RoleParchment, RoleMeadows, RoleBlackForest, RoleForestTree, RoleSwamp, RoleMountain,
	RoleSnow, RolePlains, RoleMistlands, RoleMist, RoleAshlands, RoleLava, RoleDeepNorth,
	RoleOcean, RoleShallows,
}

// defaultMetres is how many world metres one repetition of a role covers.
func defaultMetres(role Role) float32 {
	switch role {
	case RoleParchment:
		return 3072
	case RoleForestTree:
		return 256
	case RoleMist, RoleLava:
		return 768
	case RoleShallows:
		return 256
	default:
		return 512
	}
}

var builtinCache sync.Map // Role -> *Tile

// builtinTile returns the procedural texture for a role, generated once.
func builtinTile(role Role) *Tile {
	if v, ok := builtinCache.Load(role); ok {
		return v.(*Tile)
	}
	t := generate(role)
	actual, _ := builtinCache.LoadOrStore(role, t)
	return actual.(*Tile)
}

func generate(role Role) *Tile {
	n := baseNoise()
	switch role {
	case RoleParchment:
		return mottled(n, 1024, defaultMetres(role), 0, parchmentBase, parchmentBase, parchmentBlotch, 0.007, 4, 0.02)
	case RoleMeadows:
		return mottled(n, 512, defaultMetres(role), 100, meadowsBase, meadowsLight, meadowsDark, 0.03, 3, 0.03)
	case RoleBlackForest:
		return mottled(n, 512, defaultMetres(role), 200, blackForestBase, blackForestBase, blackForestDark, 0.035, 3, 0.03)
	case RoleForestTree:
		return treeTile(n)
	case RoleSwamp:
		return swampTile(n)
	case RoleMountain:
		return rockTile(n)
	case RoleSnow:
		return mottled(n, 512, defaultMetres(role), 500, snowBase, snowBase, snowBlue, 0.02, 2, 0.01)
	case RolePlains:
		return mottled(n, 512, defaultMetres(role), 600, plainsBase, plainsLight, plainsDark, 0.06, 3, 0.03)
	case RoleMistlands:
		return mottled(n, 512, defaultMetres(role), 700, mistlandsBase, mistlandsBase, mistlandsDark, 0.04, 3, 0.03)
	case RoleMist:
		return mistTile(n)
	case RoleAshlands:
		return mottled(n, 512, defaultMetres(role), 900, ashlandsBase, ashlandsBase, ashlandsDark, 0.035, 3, 0.03)
	case RoleLava:
		return lavaTile(n)
	case RoleDeepNorth:
		return mottled(n, 512, defaultMetres(role), 1100, deepNorthBase, deepNorthBase, deepNorthBlue, 0.02, 2, 0.01)
	case RoleOcean:
		return oceanTile(n)
	case RoleShallows:
		return shallowsTile(n)
	}
	return mottled(n, 64, 512, 0, rgb{0.5, 0.5, 0.5}, rgb{0.5, 0.5, 0.5}, rgb{0.5, 0.5, 0.5}, 0.1, 1, 0)
}

// mottled is the workhorse: base colour pushed toward light and dark by
// fbm, plus fine grain. freq is lattice units per texel. The texture wraps
// because the lattice period (256) divides the sample span.
func mottled(n *noise, size int, metres float32, off float32, base, light, dark rgb, freq float32, oct int, grain float32) *Tile {
	t := newTile(size, size, metres)
	period := float32(256) / (freq * float32(size)) // repetitions of the lattice across the tile
	_ = period
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float32(x) / float32(size) * 256 * freq * float32(size) / 256
			fz := float32(y) / float32(size) * 256 * freq * float32(size) / 256
			// Force periodicity: sample on a lattice whose period is an
			// integer number of texels.
			v := n.fbm(fx+off, fz+off, oct)
			g := n.value(float32(x)*0.9+off+7, float32(y)*0.9+off+3)
			c := base
			if v > 0.5 {
				c = mix(base, light, (v-0.5)*2)
			} else {
				c = mix(dark, base, v*2)
			}
			c = c.mul(1 + (g-0.5)*grain*2)
			t.set(x, y, c.R, c.G, c.B, 1)
		}
	}
	makeSeamless(t)
	return t
}

// makeSeamless cross-fades the outer 12 % of a tile with the opposite edge so
// procedural noise that does not repeat exactly still wraps without a visible
// seam.
func makeSeamless(t *Tile) {
	blend := func(size int, i int) float32 {
		band := float32(size) * 0.12
		f := float32(i)
		if f < band {
			return 0.5 + 0.5*(f/band) // 0.5 at the edge, 1 inside
		}
		if f > float32(size)-band {
			return 0.5 + 0.5*((float32(size)-f)/band)
		}
		return 1
	}
	src := make([]float32, len(t.Pix))
	copy(src, t.Pix)
	get := func(x, y int) (float32, float32, float32, float32) {
		i := (y*t.W + x) * 4
		return src[i], src[i+1], src[i+2], src[i+3]
	}
	for y := 0; y < t.H; y++ {
		wy := blend(t.H, y)
		for x := 0; x < t.W; x++ {
			wx := blend(t.W, x)
			if wx == 1 && wy == 1 {
				continue
			}
			r, g, b, a := get(x, y)
			ox, oy := (x+t.W/2)%t.W, (y+t.H/2)%t.H
			r2, g2, b2, a2 := get(ox, oy)
			w := wx * wy
			t.set(x, y, lerp(r2, r, w), lerp(g2, g, w), lerp(b2, b, w), lerp(a2, a, w))
		}
	}
}

// treeTile draws jittered tree crowns; alpha is coverage, Aux the crown's
// priority (0..1) so the styler can show a fraction of them for sparser
// forest. 512 texels over 256 m: half-metre texels, crowns 7–12 m across on a
// 7 m grid, so a full forest closes into a canopy like the game's, and the
// repeat is long enough not to read as a pattern at the deepest zoom.
func treeTile(n *noise) *Tile {
	const size = 512
	t := newTile(size, size, defaultMetres(RoleForestTree))
	t.Aux = make([]float32, size*size)
	const cells = 36
	cell := float32(size) / cells
	rnd := newNoise(0x7ee5)
	for cy := 0; cy < cells; cy++ {
		for cx := 0; cx < cells; cx++ {
			jx := rnd.at(cx*3, cy*3)
			jz := rnd.at(cx*3+1, cy*3+1)
			pr := rnd.at(cx*3+2, cy*3+2)
			rad := 7.0 + 5.0*rnd.at(cx*5+1, cy*7+3)
			px := (float32(cx)+0.5)*cell + (jx-0.5)*cell*0.9
			pz := (float32(cy)+0.5)*cell + (jz-0.5)*cell*0.9
			r := int(rad) + 1
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					x := wrap(int(px)+dx, size)
					y := wrap(int(pz)+dy, size)
					ddx := float32(int(px)+dx) + 0.5 - px
					ddz := float32(int(pz)+dy) + 0.5 - pz
					d := hypot32(ddx, ddz)
					if d > rad {
						continue
					}
					c := treeDark
					if ddx+ddz < -rad*0.25 {
						c = treeLight // lit from the north-west
					}
					if d > rad-1.6 {
						c = treeRim
					}
					i := y*size + x
					if t.Pix[i*4+3] > 0 && t.Aux[i] < pr {
						continue // an existing disc with lower priority stays on top
					}
					t.set(x, y, c.R, c.G, c.B, 1)
					t.Aux[i] = pr
				}
			}
		}
	}
	return t
}

func swampTile(n *noise) *Tile {
	t := mottled(n, 512, defaultMetres(RoleSwamp), 300, swampBase, swampBase, swampDark, 0.03, 3, 0.03)
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.fbm(float32(x)*0.02+300, float32(y)*0.02+300, 3)
			if v > 0.6 {
				r, g, b, _ := t.get(x, y)
				c := mix(rgb{r, g, b}, swampPool, smoothstep(0.6, 0.68, v))
				t.set(x, y, c.R, c.G, c.B, 1)
			}
		}
	}
	makeSeamless(t)
	return t
}

// rockTile hatches the rock with a directional streak, as the game's
// mountain texture reads at map scale.
func rockTile(n *noise) *Tile {
	t := newTile(512, 512, defaultMetres(RoleMountain))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.fbm(float32(x)*0.03+400, float32(y)*0.03+400, 3)
			s := n.value(float32(x)*0.25+400, float32(y)*0.02+400)
			c := mix(rockDark, rockBase, v)
			c = c.mul(0.9 + 0.2*s)
			t.set(x, y, c.R, c.G, c.B, 1)
		}
	}
	makeSeamless(t)
	return t
}

// mistTile is colour plus a soft alpha of large blobs.
func mistTile(n *noise) *Tile {
	t := newTile(512, 512, defaultMetres(RoleMist))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.fbm(float32(x)*0.012+800, float32(y)*0.012+800, 3)
			t.set(x, y, mistColour.R, mistColour.G, mistColour.B, smoothstep(0.48, 0.78, v))
		}
	}
	makeSeamless(t)
	return t
}

// lavaTile: bright veins from ridged noise, alpha marks where lava is.
func lavaTile(n *noise) *Tile {
	t := newTile(512, 512, defaultMetres(RoleLava))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.ridged(float32(x)*0.02+1000, float32(y)*0.02+1000, 3)
			c := mix(lavaEdge, lavaCore, smoothstep(0.7, 0.95, v))
			if v > 0.97 {
				c = lavaGlow
			}
			t.set(x, y, c.R, c.G, c.B, smoothstep(0.62, 0.8, v))
		}
	}
	makeSeamless(t)
	return t
}

// oceanTile carries faint wave crests in its luminance; the styler uses it
// as a ±modulation on the depth colour.
func oceanTile(n *noise) *Tile {
	t := newTile(512, 512, defaultMetres(RoleOcean))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.value(float32(x)*0.05+1200, float32(y)*0.2+1200)
			w := 0.5 + 0.5*(v-0.5)
			t.set(x, y, w, w, w, 1)
		}
	}
	makeSeamless(t)
	return t
}

func shallowsTile(n *noise) *Tile {
	t := newTile(256, 256, defaultMetres(RoleShallows))
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			v := n.fbm(float32(x)*0.06+1300, float32(y)*0.06+1300, 2)
			c := mix(shallowsSand.mul(0.92), shallowsSand.mul(1.06), v)
			t.set(x, y, c.R, c.G, c.B, 1)
		}
	}
	makeSeamless(t)
	return t
}
