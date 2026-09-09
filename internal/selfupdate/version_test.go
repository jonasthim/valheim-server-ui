package selfupdate

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"1.2.3", "v1.2.3", 0}, // "v" prefix optional
		{"v1.2.4", "v1.2.3", 1},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.3.0", "v1.2.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0", "v2.0.0", -1},
		{"v1.2.0-rc.1", "v1.2.0", -1}, // pre-release sorts before the release
		{"v1.2.0", "v1.2.0-rc.1", 1},
		{"v1.2.0-rc.1", "v1.2.0-rc.1", 0},
		{"dev", "v1.0.0", -1}, // dev is always older than a real version
		{"v1.0.0", "dev", 1},
		{"dev", "dev", 0},
		{"garbage", "v1.0.0", -1},
		{"garbage", "garbage2", 0}, // two unparsable strings compare equal
		{"", "v0.0.1", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
