package maps

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
)

// Overlay draws above the tiles of a map and below its markers: the weather
// of package weather, a route, a heat map.
//
// Both methods receive the [Projection] of the frame, the camera the tiles
// are drawn with, so an overlay stays on the ground while the map moves.
// LayoutOverlay runs in the map's layout and is where an overlay requests
// what it needs, as a view requests its pictures; it reports whether it is
// animating, and the map lays out again thirty times a second while one is,
// and at the full rate while the map moves. PaintOverlay runs in its paint
// and must draw nothing outside the map's bounds, the rule of the project
// plan, section 37.
type Overlay interface {
	LayoutOverlay(ctx *gift.LayoutContext, p Projection) (animating bool)
	PaintOverlay(ctx *gift.PaintContext, p Projection, bounds geom.Rect)
}

// Projection is the camera of one frame on the map's viewport: the
// arithmetic between world points and the map's own coordinates, in logical
// pixels from its top left corner.
type Projection struct{ v projection }

// Center returns the world point in the middle of the map.
func (p Projection) Center() geo.Point { return p.v.c }

// Zoom returns the zoom level.
func (p Projection) Zoom() float64 { return p.v.zoom }

// Heading returns the heading in radians, clockwise from north.
func (p Projection) Heading() float64 { return p.v.heading }

// Scale returns the logical pixels per world unit, TileSize·2^Zoom.
func (p Projection) Scale() float64 { return p.v.scale }

// Size returns the size of the map.
func (p Projection) Size() (w, h float64) { return p.v.w, p.v.h }

// ToScreen returns the map position of a world point, taking the copy of the
// world nearest the centre.
func (p Projection) ToScreen(w geo.Point) (x, y float64) { return p.v.toScreen(w) }

// ToWorld returns the world point under a map position.
func (p Projection) ToWorld(x, y float64) geo.Point { return p.v.toWorld(x, y) }

// Visible returns the smallest rectangle of coordinates that contains the
// whole viewport, turned or not.
func (p Projection) Visible() geo.Rect {
	var r geo.Rect
	for i, c := range [4][2]float64{{0, 0}, {p.v.w, 0}, {p.v.w, p.v.h}, {0, p.v.h}} {
		w := p.v.toWorld(c[0], c[1])
		w.Y = math.Max(0, math.Min(1, w.Y))
		cc := geo.Point{X: w.X - math.Floor(w.X), Y: w.Y}.Coordinate()
		if i == 0 {
			r = geo.Rect{Min: cc, Max: cc}
		} else {
			r = r.Extend(cc)
		}
	}
	return r
}

func (n *node) layoutOverlays(ctx *gift.LayoutContext, v projection) {
	n.f.overlayAnim = false
	for _, o := range n.v.overlays {
		if o.LayoutOverlay(ctx, Projection{v}) {
			n.f.overlayAnim = true
		}
	}
}

func (n *node) paintOverlays(ctx *gift.PaintContext, b geom.Rect) {
	for _, o := range n.v.overlays {
		o.PaintOverlay(ctx, Projection{n.f.proj}, b)
	}
}
