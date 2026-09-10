package mapstyle

import "testing"

func TestParchment(t *testing.T) {
	p := NewParchment(42, nil)
	r, g, b := p.At(100, 100)
	if !(r > g && g > b) || r < 0.6 || r > 0.9 {
		t.Fatalf("parchment tone off: %.3f %.3f %.3f", r, g, b)
	}
	r2, _, _ := p.At(5100, -3300)
	if r2 == r {
		t.Fatal("parchment should vary across the world")
	}
	for _, wx := range []float32{0, 137, 9999, -4321} {
		n := p.EdgeNoise(wx, wx*0.7)
		if n < -0.5 || n > 0.5 {
			t.Fatalf("edge noise out of range: %v", n)
		}
	}
}
