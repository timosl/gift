package ui

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// paintArc strokes the arc of the circle around c whose outer edge has the
// radius r, w wide with round ends, from start sweeping sweep clockwise, both
// in radians from twelve o'clock; see [render.OpArc].
func paintArc(ctx *gift.PaintContext, c geom.Point, r, w, start, sweep float32, col Color) {
	if !(sweep > 0) || col.IsTransparent() {
		return
	}
	ctx.Add(render.Op{
		Kind:        render.OpArc,
		Bounds:      geom.Rc(c.X-r, c.Y-r, c.X+r, c.Y+r),
		Color:       col,
		StrokeWidth: w,
		Material:    ctx.AddMaterial(render.Arc{Start: start, Sweep: sweep}.Material()),
	})
}

// onCircle is the point at angle a, clockwise from twelve o'clock, on the
// circle around c with radius r.
func onCircle(c geom.Point, r, a float32) geom.Point {
	s, k := math.Sincos(float64(a))
	return geom.Pt(c.X+r*float32(s), c.Y-r*float32(k))
}

// Metrics of the spinner of UIActivityIndicatorView at its medium size, 20
// points across; a larger one scales them.
const (
	spinnerSpokes = 8
	spinnerInner  = float32(4.5)
	spinnerOuter  = float32(9.5)
	spinnerWidth  = float32(1.25)
	spinnerStep   = 90 * time.Millisecond
	spinnerSize   = float32(20)
)

// paintSpinner draws the spinner around c at scale k of its medium size:
// shown of its spokes from twelve o'clock, solid, or, turning, all of them
// with the newest solid and the ones behind it fading. A turning spinner
// asks for the frames it needs.
func paintSpinner(ctx *gift.PaintContext, c geom.Point, k float32, col Color, shown int, turning bool) {
	head := 0
	if turning {
		shown = spinnerSpokes
		head = int(ctx.Now()/spinnerStep) % spinnerSpokes
		ctx.Animate(spinnerStep)
	}
	for i := range shown {
		a := 2 * math.Pi * float32(i) / spinnerSpokes
		alpha := float32(1)
		if turning {
			age := (head - i + spinnerSpokes) % spinnerSpokes
			alpha = 1 - 0.75*float32(age)/spinnerSpokes
		}
		paintBar(ctx, onCircle(c, spinnerInner*k, a), onCircle(c, spinnerOuter*k, a), spinnerWidth*k, scaleColor(col, alpha))
	}
}
