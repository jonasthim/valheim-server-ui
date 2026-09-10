package mapstyle

// Parchment is the unexplored ground: the game's beige paper with soft
// blotches. The fog composite samples it per pixel and uses EdgeNoise to make
// the explored boundary cloudy instead of a smooth contour.
type Parchment struct {
	tile   *Tile
	n      *noise
	ox, oz float32
}

// NewParchment uses the pack's parchment texture when present.
func NewParchment(seed int, pack *Pack) *Parchment {
	return &Parchment{
		tile: pack.Tile(RoleParchment),
		n:    baseNoise(),
		ox:   float32(seed%4096) * 131,
		oz:   float32((seed/4096)%4096) * 257,
	}
}

// At is the parchment colour at a world position (0..1 components).
func (p *Parchment) At(wx, wz float32) (r, g, b float32) {
	r1, g1, b1, _ := p.tile.Sample(wx+p.ox, wz+p.oz)
	r2, g2, b2, _ := p.tile.Sample(wx*0.23+p.oz+503, wz*0.23+p.ox+211)
	return lerp(r1, r2, 0.5), lerp(g1, g2, 0.5), lerp(b1, b2, 0.5)
}

// EdgeNoise is a slow fbm centred on zero (−0.5..0.5) at roughly 140 m,
// used to perturb the fog threshold.
func (p *Parchment) EdgeNoise(wx, wz float32) float32 {
	return p.n.fbm((wx+p.ox)/140, (wz+p.oz)/140, 2) - 0.5
}
