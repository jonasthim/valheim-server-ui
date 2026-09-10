package mapstyle

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestCloudsPNG(t *testing.T) {
	data := CloudsPNG()
	if !bytes.Equal(data, CloudsPNG()) {
		t.Fatal("clouds must be generated once")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 512 {
		t.Fatalf("clouds decode: %v", err)
	}
	// Both clear and cloudy texels exist, and the tile wraps: the first and
	// last columns are close in alpha.
	var lo, hi int
	var seam float64
	for y := 0; y < 512; y += 8 {
		_, _, _, a0 := img.At(0, y).RGBA()
		_, _, _, a1 := img.At(511, y).RGBA()
		seam += float64(int64(a0>>8) - int64(a1>>8))
		if a0>>8 < 20 {
			lo++
		}
		if a0>>8 > 200 {
			hi++
		}
	}
	if lo == 0 || hi == 0 {
		t.Fatalf("clouds need clear and cloudy texels: lo=%d hi=%d", lo, hi)
	}
	if seam/64 > 24 || seam/64 < -24 {
		t.Fatalf("clouds must wrap at the edge, mean seam %v", seam/64)
	}
}

func TestWaterMask(t *testing.T) {
	l := synthLayers(256)
	p := DefaultParams(1)
	// Fog: left half unexplored.
	mask := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			v := uint8(0)
			if x < 2 {
				v = 255
			}
			mask.Set(x, y, color.NRGBA{v, v, v, v})
		}
	}
	m := WaterMask(l, p, mask, 128)
	at := func(wx, wz float32) uint8 {
		x, y := worldToPx(128, wx, wz)
		return m.GrayAt(x, y).Y
	}
	if at(6000, -6000) < 200 {
		t.Fatalf("explored ocean must be marked: %d", at(6000, -6000))
	}
	if at(-6000, -6000) != 0 {
		t.Fatalf("ocean under the fog must be 0: %d", at(-6000, -6000))
	}
	if at(500, -500) != 0 {
		t.Fatalf("land must be 0: %d", at(500, -500))
	}
	if at(10400, 10400) != 0 {
		t.Fatal("off-world must be 0")
	}
	if WaterMask(l, p, nil, 64).GrayAt(50, 14).Y < 200 {
		t.Fatal("without a fog mask all water counts")
	}
}
