package maps

import (
	"math"

	"github.com/worldiety/gift/geo"
)

// Camera is what a map shows: where, how close and which way up.
type Camera struct {
	// Center is the coordinate in the middle of the map.
	Center geo.Coordinate
	// Zoom is the level of detail: at zoom z the world is TileSize·2^z
	// logical pixels wide. Zero shows the world in one tile; 12 a city; 17
	// a street. It is continuous.
	Zoom float64
	// Heading is the compass direction at the top of the map in degrees,
	// clockwise from north: 0 is north up, 90 is east up, as in MapKit.
	Heading float64
}

// projection is a camera on a viewport: the arithmetic between the world and
// the screen, in float64 throughout. Screen positions are logical pixels
// relative to the viewport's top left corner.
//
// screen = R(−heading)·(world − centre)·scale + mid, with scale =
// TileSize·2^zoom. Everything that is drawn goes through world − centre
// first, so the large common part of two neighbouring positions cancels
// before anything is rounded to float32; see package geo.
type projection struct {
	c        geo.Point
	zoom     float64
	heading  float64 // radians
	w, h     float64
	scale    float64
	sin, cos float64
}

func newProjection(c geo.Point, zoom, heading, w, h float64) projection {
	s, co := math.Sincos(heading)
	return projection{
		c: c, zoom: zoom, heading: heading, w: w, h: h,
		scale: TileSize * math.Exp2(zoom), sin: s, cos: co,
	}
}

func scaleOf(zoom float64) float64 { return TileSize * math.Exp2(zoom) }

// unrotated returns the screen position p would have at heading zero: the
// space tiles are laid out in before the map is turned about its middle.
// The world copy nearest the centre is chosen, so a map panned across the
// antimeridian shows what is east of it and not the other side of the earth.
func (v projection) unrotated(p geo.Point) (x, y float64) {
	dx := p.X - v.c.X
	dx -= math.Round(dx)
	return v.w/2 + dx*v.scale, v.h/2 + (p.Y-v.c.Y)*v.scale
}

// rotate turns an unrotated screen position about the middle into the
// screen position at the camera's heading.
func (v projection) rotate(x, y float64) (float64, float64) {
	x, y = x-v.w/2, y-v.h/2
	// R(−heading): the heading points up.
	return v.w/2 + v.cos*x + v.sin*y, v.h/2 - v.sin*x + v.cos*y
}

// unrotate is the inverse of rotate.
func (v projection) unrotate(x, y float64) (float64, float64) {
	x, y = x-v.w/2, y-v.h/2
	return v.w/2 + v.cos*x - v.sin*y, v.h/2 + v.sin*x + v.cos*y
}

// toScreen returns the screen position of p.
func (v projection) toScreen(p geo.Point) (x, y float64) { return v.rotate(v.unrotated(p)) }

// toWorld returns the world point under the screen position x, y.
func (v projection) toWorld(x, y float64) geo.Point {
	ux, uy := v.unrotate(x, y)
	return geo.Point{X: v.c.X + (ux-v.w/2)/v.scale, Y: v.c.Y + (uy-v.h/2)/v.scale}
}

// worldVector returns the world vector of a screen vector: how far the world
// moves under a finger that moves by dx, dy.
func (v projection) worldVector(dx, dy float64) geo.Point {
	// R(+heading), the inverse of the rotation in rotate.
	return geo.Point{X: (v.cos*dx - v.sin*dy) / v.scale, Y: (v.sin*dx + v.cos*dy) / v.scale}
}

// corners returns the viewport's corners in the unrotated screen space,
// clockwise from the top left: the polygon visibility is computed against.
// At heading zero it is the viewport itself; turned, it is the viewport
// turned the other way about the middle, and its bounding box would hold up
// to twice the tiles.
func (v projection) corners() [4][2]float64 { return v.cornersOutset(0) }

// cornersOutset is [projection.corners] of the viewport grown by r points
// on every side.
func (v projection) cornersOutset(r float64) [4][2]float64 {
	var out [4][2]float64
	for i, p := range [4][2]float64{{-r, -r}, {v.w + r, -r}, {v.w + r, v.h + r}, {-r, v.h + r}} {
		out[i][0], out[i][1] = v.unrotate(p[0], p[1])
	}
	return out
}

// extentY is the height of the turned viewport's bounding box: what the
// world must at least cover for no pole to come into view.
func (v projection) extentY() float64 {
	return math.Abs(v.sin)*v.w + math.Abs(v.cos)*v.h
}

// clampCenterY keeps the poles out of view: the centre's Y is held where the
// viewport's top and bottom stay inside the projection, or in the middle when
// the whole world is less tall than the viewport.
func clampCenterY(y, zoom, extent float64) float64 {
	half := extent / 2 / scaleOf(zoom)
	if half >= 0.5 {
		return 0.5
	}
	return math.Max(half, math.Min(1-half, y))
}

// minZoomFor is the zoom at which the world is exactly as tall as the turned
// viewport: further out, the map would show the void past the poles.
func minZoomFor(extent float64) float64 {
	if extent <= 0 {
		return 0
	}
	return math.Max(0, math.Log2(extent/TileSize))
}

// polygonIntersectsRect reports whether the convex polygon p overlaps the
// axis aligned rectangle x0, y0, x1, y1, by the separating axis theorem: the
// rectangle's two axes and the polygon's edge normals.
func polygonIntersectsRect(p [4][2]float64, x0, y0, x1, y1 float64) bool {
	minX, minY, maxX, maxY := p[0][0], p[0][1], p[0][0], p[0][1]
	for _, q := range p[1:] {
		minX, maxX = math.Min(minX, q[0]), math.Max(maxX, q[0])
		minY, maxY = math.Min(minY, q[1]), math.Max(maxY, q[1])
	}
	if maxX <= x0 || minX >= x1 || maxY <= y0 || minY >= y1 {
		return false
	}
	rect := [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		nx, ny := b[1]-a[1], a[0]-b[0]
		pmin, pmax := math.Inf(1), math.Inf(-1)
		for _, q := range p {
			d := nx*q[0] + ny*q[1]
			pmin, pmax = math.Min(pmin, d), math.Max(pmax, d)
		}
		rmin, rmax := math.Inf(1), math.Inf(-1)
		for _, q := range rect {
			d := nx*q[0] + ny*q[1]
			rmin, rmax = math.Min(rmin, d), math.Max(rmax, d)
		}
		if rmax <= pmin || rmin >= pmax {
			return false
		}
	}
	return true
}
