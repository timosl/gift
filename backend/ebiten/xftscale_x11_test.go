//go:build linux && !android

package ebiten

import "testing"

func TestParseXftDPI(t *testing.T) {
	for _, c := range []struct {
		res  string
		want float64
	}{
		{"Xft.antialias:\t1\nXft.dpi:\t192\nXft.hinting:\t1\n", 192},
		{"Xft.dpi:\t96", 96},
		{"Xft.antialias:\t1\n", 0},
		{"Xft.dpi:\tlarge\n", 0},
		{"", 0},
	} {
		if got := parseXftDPI(c.res); got != c.want {
			t.Errorf("parseXftDPI(%q) = %v, want %v", c.res, got, c.want)
		}
	}
}
