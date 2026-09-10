package mapstyle

// sampler reads the layer grid at continuous world coordinates: height with
// Catmull-Rom bicubic filtering (so hill shading stays smooth when a tile is
// drawn far below the grid's resolution), forest and mask bilinearly, and
// biomes as bilinear weights of the four nearest cells so biome boundaries
// blend instead of stepping.
type sampler struct {
	l    *Layers
	r    float32 // world radius
	step float32 // metres per cell
}

func newSampler(l *Layers, worldRadius float32) *sampler {
	return &sampler{l: l, r: worldRadius, step: 2 * worldRadius / float32(l.Size)}
}

// cell converts a world position to continuous cell coordinates where
// integer values sit on cell centres.
func (s *sampler) cell(wx, wz float32) (cx, cz float32) {
	return (wx+s.r)/s.step - 0.5, (s.r-wz)/s.step - 0.5
}

func catmull(p0, p1, p2, p3, t float32) float32 {
	return 0.5 * ((2 * p1) + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t*t + (-p0+3*p1-3*p2+p3)*t*t*t)
}

// heightAt in metres, bicubic.
func (s *sampler) heightAt(wx, wz float32) float32 {
	cx, cz := s.cell(wx, wz)
	fx, fz := floor32(cx), floor32(cz)
	tx, tz := cx-fx, cz-fz
	ix, iz := int(fx), int(fz)
	var rows [4]float32
	for j := -1; j <= 2; j++ {
		rows[j+1] = catmull(
			s.l.Height(ix-1, iz+j), s.l.Height(ix, iz+j), s.l.Height(ix+1, iz+j), s.l.Height(ix+2, iz+j), tx)
	}
	return catmull(rows[0], rows[1], rows[2], rows[3], tz)
}

func (s *sampler) bilinear(wx, wz float32, f func(x, y int) float32) float32 {
	cx, cz := s.cell(wx, wz)
	fx, fz := floor32(cx), floor32(cz)
	tx, tz := cx-fx, cz-fz
	ix, iz := int(fx), int(fz)
	return lerp(lerp(f(ix, iz), f(ix+1, iz), tx), lerp(f(ix, iz+1), f(ix+1, iz+1), tx), tz)
}

func (s *sampler) forestAt(wx, wz float32) float32 { return s.bilinear(wx, wz, s.l.Forest) }
func (s *sampler) maskAt(wx, wz float32) float32   { return s.bilinear(wx, wz, s.l.Mask) }

type biomeW struct {
	b Biome
	w float32
}

// biomes returns up to four (biome, weight) pairs summing to 1.
func (s *sampler) biomes(wx, wz float32) (out [4]biomeW, n int) {
	cx, cz := s.cell(wx, wz)
	fx, fz := floor32(cx), floor32(cz)
	tx, tz := cx-fx, cz-fz
	ix, iz := int(fx), int(fz)
	add := func(b Biome, w float32) {
		if w <= 0 {
			return
		}
		for i := 0; i < n; i++ {
			if out[i].b == b {
				out[i].w += w
				return
			}
		}
		out[n] = biomeW{b, w}
		n++
	}
	add(s.l.Biome(ix, iz), (1-tx)*(1-tz))
	add(s.l.Biome(ix+1, iz), tx*(1-tz))
	add(s.l.Biome(ix, iz+1), (1-tx)*tz)
	add(s.l.Biome(ix+1, iz+1), tx*tz)
	return
}

// relief returns the hill-shade factor and the slope (rise per metre) at a
// position, from central differences d metres apart. Light comes from the
// north-west, 45° up, like the in-game map.
func (s *sampler) relief(wx, wz, d float32) (shade, slope float32) {
	dhdx := (s.heightAt(wx+d, wz) - s.heightAt(wx-d, wz)) / (2 * d)
	dhdz := (s.heightAt(wx, wz+d) - s.heightAt(wx, wz-d)) / (2 * d)
	const exaggeration = 2.0
	nx, nz, ny := -dhdx*exaggeration, -dhdz*exaggeration, float32(1)
	inv := 1 / float32(sqrt32(nx*nx+nz*nz+ny*ny))
	nx, nz, ny = nx*inv, nz*inv, ny*inv
	// light direction (east, north, up)
	const lx, lz, ly = -0.5, 0.5, 0.7071
	shade = clamp01(nx*lx + nz*lz + ny*ly)
	slope = hypot32(dhdx, dhdz)
	return
}

func sqrt32(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton iterations are plenty for shading.
	y := x
	for i := 0; i < 6; i++ {
		y = 0.5 * (y + x/y)
	}
	return y
}
