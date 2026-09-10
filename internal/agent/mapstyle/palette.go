package mapstyle

import "strconv"

// rgb is a linear-ish working colour, components 0..1 (sRGB values used
// directly; the map is a stylised drawing, not a physically lit scene).
type rgb struct{ R, G, B float32 }

func col(s string) rgb {
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return rgb{1, 0, 1}
	}
	return rgb{float32(v>>16&255) / 255, float32(v>>8&255) / 255, float32(v&255) / 255}
}

func (c rgb) mul(f float32) rgb { return rgb{c.R * f, c.G * f, c.B * f} }

func (c rgb) lum() float32 { return 0.299*c.R + 0.587*c.G + 0.114*c.B }

func mix(a, b rgb, t float32) rgb {
	return rgb{lerp(a.R, b.R, t), lerp(a.G, b.G, t), lerp(a.B, b.B, t)}
}

func (c rgb) bytes() (uint8, uint8, uint8) {
	return toByte(c.R), toByte(c.G), toByte(c.B)
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

// Palette, sampled from the in-game map: parchment ground, light
// yellow-green meadows with dark tree clusters, darker black forest,
// grey/white mountains, blue-grey water.
var (
	parchmentBase   = col("#cdc3a8")
	parchmentBlotch = col("#b8ad92")
	abyss           = col("#1a2938")

	meadowsBase  = col("#93a65c")
	meadowsLight = col("#a4b56a")
	meadowsDark  = col("#7f9350")
	treeDark     = col("#3f5a2d")
	treeLight    = col("#5a763b")
	treeRim      = col("#2a3f20")

	blackForestBase = col("#5b7443")
	blackForestDark = col("#4b6237")

	swampBase = col("#6a6a49")
	swampDark = col("#4b5140")
	swampPool = col("#3e4a3f")

	rockBase = col("#9c9b95")
	rockDark = col("#7f7e79")
	snowBase = col("#eef0ee")
	snowBlue = col("#dbe2e6")

	plainsBase  = col("#cbb86a")
	plainsLight = col("#d8c778")
	plainsDark  = col("#b7a45a")
	plainsTree  = col("#6d7e3c")

	mistlandsBase = col("#5f5d67")
	mistlandsDark = col("#4b4a53")
	mistColour    = col("#c1c4cb")

	ashlandsBase = col("#8a3c2a")
	ashlandsDark = col("#4a2018")
	lavaCore     = col("#e8642a")
	lavaEdge     = col("#b4461c")
	lavaGlow     = col("#f2a04a")

	deepNorthBase = col("#e2e8ee")
	deepNorthBlue = col("#cbd7e2")

	waterShallow = col("#7d9cb0")
	waterDeep    = col("#4a6b85")
	waterAbyssal = col("#34506a")
	shallowsSand = col("#a7b7a6")
	beachColour  = col("#d1c290")
	ringColour   = col("#243748")
)
