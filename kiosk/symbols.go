package kiosk

import (
	"math"

	"github.com/worldiety/gift/internal/icon"
	"github.com/worldiety/gift/ui"
)

// The symbols the icon sets of gift do not have, drawn here as strokes on
// the 24 unit box of the Flowbite set, so that they sit beside its icons.
var (
	// wifiSymbol is three arcs over a dot.
	wifiSymbol = symbol(func(e *icon.Encoder) {
		for _, r := range []float32{14, 9.5, 5} {
			e.StrokeFigure(2, icon.CapRound, icon.JoinRound)
			arc(e, 12, 19, r, -135, -45)
		}
		e.Figure(icon.PaintFill)
		circle(e, 12, 19, 1.6)
	})
	// networkSymbol is three nodes of a wired network.
	networkSymbol = symbol(func(e *icon.Encoder) {
		e.StrokeFigure(2, icon.CapRound, icon.JoinRound)
		rect(e, 9, 3, 6, 5)
		rect(e, 3, 16, 6, 5)
		rect(e, 15, 16, 6, 5)
		e.MoveTo(12, 8)
		e.LineTo(12, 12)
		e.MoveTo(6, 16)
		e.LineTo(6, 12)
		e.LineTo(18, 12)
		e.LineTo(18, 16)
	})
	// powerSymbol is the power sign: an open circle with a bar.
	powerSymbol = symbol(func(e *icon.Encoder) {
		e.StrokeFigure(2, icon.CapRound, icon.JoinRound)
		arc(e, 12, 13, 8, -60, 240)
		e.MoveTo(12, 3)
		e.LineTo(12, 11)
	})
)

func symbol(draw func(e *icon.Encoder)) ui.Symbol {
	var e icon.Encoder
	draw(&e)
	b, err := e.Bytes()
	if err != nil {
		panic("kiosk: a symbol does not encode: " + err.Error())
	}
	return ui.NewSymbol(b, 24)
}

func rect(e *icon.Encoder, x, y, w, h float32) {
	e.MoveTo(x, y)
	e.LineTo(x+w, y)
	e.LineTo(x+w, y+h)
	e.LineTo(x, y+h)
	e.Close()
}

func circle(e *icon.Encoder, cx, cy, r float32) {
	arc(e, cx, cy, r, 0, 360)
	e.Close()
}

// arc draws a circular arc from angle a0 to a1, in degrees clockwise from
// the right as the y axis points down, with one cubic per quarter.
func arc(e *icon.Encoder, cx, cy, r, a0, a1 float32) {
	pt := func(a float64) (float32, float32) {
		return cx + r*float32(math.Cos(a)), cy + r*float32(math.Sin(a))
	}
	n := int(math.Ceil(math.Abs(float64(a1-a0)) / 90))
	step := float64(a1-a0) / float64(n) * math.Pi / 180
	a := float64(a0) * math.Pi / 180
	x, y := pt(a)
	e.MoveTo(x, y)
	k := float32(4.0 / 3 * math.Tan(step/4))
	for range n {
		b := a + step
		x0, y0 := pt(a)
		x1, y1 := pt(b)
		c1x := x0 - k*r*float32(math.Sin(a))
		c1y := y0 + k*r*float32(math.Cos(a))
		c2x := x1 + k*r*float32(math.Sin(b))
		c2y := y1 - k*r*float32(math.Cos(b))
		e.CubeTo(c1x, c1y, c2x, c2y, x1, y1)
		a = b
	}
}
