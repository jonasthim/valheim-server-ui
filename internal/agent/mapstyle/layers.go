// Package mapstyle draws the world map in the style of Valheim's own in-game
// map from the raw layers the server plugin samples once per world: biome,
// height (with the game's terrain mask) and forest factor. Everything visual
// (textures, tree stipples, hill shading, parchment) is procedural in world
// metres, so the same layers can be rendered at any zoom without pixelation.
package mapstyle

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
)

// Biome is the 4-bit biome code the plugin writes into the layers image.
type Biome uint8

// Biome codes. 10–14 are reserved; OffWorld marks pixels outside the world
// disc that the plugin did not sample.
const (
	BiomeNone   Biome = 0
	Meadows     Biome = 1
	BlackForest Biome = 2
	Swamp       Biome = 3
	Mountain    Biome = 4
	Plains      Biome = 5
	Mistlands   Biome = 6
	AshLands    Biome = 7
	DeepNorth   Biome = 8
	Ocean       Biome = 9
	OffWorld    Biome = 15
)

func (b Biome) String() string {
	switch b {
	case Meadows:
		return "meadows"
	case BlackForest:
		return "blackforest"
	case Swamp:
		return "swamp"
	case Mountain:
		return "mountain"
	case Plains:
		return "plains"
	case Mistlands:
		return "mistlands"
	case AshLands:
		return "ashlands"
	case DeepNorth:
		return "deepnorth"
	case Ocean:
		return "ocean"
	case OffWorld:
		return "offworld"
	default:
		return fmt.Sprintf("biome%d", uint8(b))
	}
}

// Layer image limits and packing constants (mirrored in the plugin).
const (
	MinLayerSize = 16
	MaxLayerSize = 4096
	heightOffset = 200.0 // metres added before packing
	heightScale  = 32.0  // packed units per metre (3 cm steps)
	forestScale  = 100.0 // packed units per unit of forest factor
	maskLevels   = 15.0
)

// Layers is the plugin's raw sample grid as decoded NRGBA bytes, 4 per pixel:
// R = mask<<4 | biome, G:B = packed height (big endian), A = forest×100.
type Layers struct {
	Size int
	Pix  []uint8
}

// NewLayers returns an all-zero grid (biome none, height −200 m, no forest).
func NewLayers(size int) *Layers {
	return &Layers{Size: size, Pix: make([]uint8, size*size*4)}
}

// PackHeight converts metres to the 16-bit layer encoding.
func PackHeight(h float32) uint16 {
	v := (float64(h) + heightOffset) * heightScale
	if v < 0 {
		return 0
	}
	if v > 65535 {
		return 65535
	}
	return uint16(v + 0.5)
}

// UnpackHeight is the inverse of PackHeight.
func UnpackHeight(v uint16) float32 {
	return float32(float64(v)/heightScale - heightOffset)
}

// Set writes one cell (tests and fixtures; the plugin writes the same layout).
func (l *Layers) Set(x, y int, b Biome, mask, h, forest float32) {
	i := (y*l.Size + x) * 4
	m := int(mask*maskLevels + 0.5)
	if m < 0 {
		m = 0
	} else if m > 15 {
		m = 15
	}
	hv := PackHeight(h)
	f := int(forest*forestScale + 0.5)
	if f < 0 {
		f = 0
	} else if f > 255 {
		f = 255
	}
	l.Pix[i] = uint8(m<<4) | uint8(b&0x0F)
	l.Pix[i+1] = uint8(hv >> 8)
	l.Pix[i+2] = uint8(hv & 0xFF) //nolint:gosec // low byte by design
	l.Pix[i+3] = uint8(f)
}

func (l *Layers) clampXY(x, y int) (int, int) {
	if x < 0 {
		x = 0
	} else if x >= l.Size {
		x = l.Size - 1
	}
	if y < 0 {
		y = 0
	} else if y >= l.Size {
		y = l.Size - 1
	}
	return x, y
}

// Biome at a cell; coordinates outside the grid clamp to the edge.
func (l *Layers) Biome(x, y int) Biome {
	x, y = l.clampXY(x, y)
	return Biome(l.Pix[(y*l.Size+x)*4] & 0x0F)
}

// Mask is the game's terrain mask alpha (0..1) at a cell.
func (l *Layers) Mask(x, y int) float32 {
	x, y = l.clampXY(x, y)
	return float32(l.Pix[(y*l.Size+x)*4]>>4) / maskLevels
}

// Height in metres at a cell.
func (l *Layers) Height(x, y int) float32 {
	x, y = l.clampXY(x, y)
	i := (y*l.Size + x) * 4
	return UnpackHeight(uint16(l.Pix[i+1])<<8 | uint16(l.Pix[i+2]))
}

// Forest is the game's forest factor at a cell (low values are forest).
func (l *Layers) Forest(x, y int) float32 {
	x, y = l.clampXY(x, y)
	return float32(l.Pix[(y*l.Size+x)*4+3]) / forestScale
}

// DecodeLayers reads the plugin's layers PNG. The bytes are taken from the
// decoded NRGBA image directly: going through color.RGBA() would premultiply
// by the forest channel and destroy the biome and height bytes.
func DecodeLayers(r io.Reader) (*Layers, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("layers: decode: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w != h || w < MinLayerSize || w > MaxLayerSize {
		return nil, fmt.Errorf("layers: unexpected size %dx%d", w, h)
	}
	l := NewLayers(w)
	switch t := img.(type) {
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			copy(l.Pix[y*w*4:(y+1)*w*4], t.Pix[y*t.Stride:y*t.Stride+w*4])
		}
	default:
		// Any other layout (a foreign encoder): convert without premultiplying.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				i := (y*w + x) * 4
				l.Pix[i], l.Pix[i+1], l.Pix[i+2], l.Pix[i+3] = c.R, c.G, c.B, c.A
			}
		}
	}
	return l, nil
}

// EncodeLayers writes the grid as the same RGBA PNG the plugin produces.
func EncodeLayers(l *Layers) ([]byte, error) {
	img := &image.NRGBA{Pix: l.Pix, Stride: l.Size * 4, Rect: image.Rect(0, 0, l.Size, l.Size)}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("layers: encode: %w", err)
	}
	return buf.Bytes(), nil
}
