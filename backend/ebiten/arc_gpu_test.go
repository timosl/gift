//go:build giftgpu

package ebiten

import (
	"image/color"
	"math"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// arcList draws one black arc in the circle inscribed in (10,10)-(70,70),
// ten wide, on white: its centre line has a radius of 25 around (40,40).
func arcList(t *testing.T, start, sweep float32) *eb.Image {
	t.Helper()
	return drawList(t, 80, 80, color.RGBA{255, 255, 255, 255}, func(l *render.List) {
		l.Add(render.Op{
			Kind:        render.OpArc,
			Bounds:      geom.Rc(10, 10, 70, 70),
			StrokeWidth: 10,
			Color:       render.RGB(0, 0, 0),
			Material:    l.AddMaterial(render.Arc{Start: start, Sweep: sweep}.Material()),
		})
	})
}

func lum(img *eb.Image, x, y int) int { return int(img.At(x, y).(color.RGBA).R) }

func TestAnArcCoversItsSweepWithRoundEnds(t *testing.T) {
	// From twelve o'clock clockwise to six: the right half.
	dst := arcList(t, 0, math.Pi)
	for _, c := range []struct {
		x, y  int
		black bool
		what  string
	}{
		{65, 40, true, "the middle of the arc, at three o'clock"},
		{15, 40, false, "nine o'clock, outside the sweep"},
		{40, 40, false, "the centre of the ring"},
		{56, 40, false, "inside the ring's hole"},
		{40, 15, true, "the start, at twelve o'clock"},
		{40, 65, true, "the end, at six o'clock"},
		{37, 15, true, "the round end, three pixels before the start"},
		{33, 15, false, "past the round end, seven pixels before the start"},
	} {
		got := lum(dst, c.x, c.y)
		if c.black && got > 60 || !c.black && got < 200 {
			t.Errorf("%s at (%d,%d) has luminance %d, want it %s", c.what, c.x, c.y, got,
				map[bool]string{true: "black", false: "white"}[c.black])
		}
	}
	// A whole turn is the whole ring.
	ring := arcList(t, 0, 2*math.Pi)
	if got := lum(ring, 15, 40); got > 60 {
		t.Errorf("a full ring at nine o'clock has luminance %d, want black", got)
	}
	if got := lum(ring, 40, 40); got < 200 {
		t.Errorf("a full ring's centre has luminance %d, want white", got)
	}
}
