package maps

import (
	"math"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/ui/motion"
)

// The motion of the camera.
var (
	// bounce brings the zoom back from past its limits and the heading to
	// north: the spring of the image viewer, critically damped, 0.6 s.
	bounce = motion.Spring{Response: 600 * time.Millisecond, Damping: 1}
	// glide is the free deceleration of a released pan, UIScrollView's
	// normal rate, as MapKit's.
	glide = motion.Decay{Rate: motion.DecelerationNormal}
)

// overZoom is how far a pinch can stretch the zoom past its limits, in zoom
// levels, approached with growing resistance; see [motion.Rubber].
const overZoom = 0.5

// northSnap is the heading, in radians, under which a released rotation
// turns back to north: about seven degrees, the dead zone of Apple Maps.
const northSnap = 7 * math.Pi / 180

// moveMode is what a [mover] is doing on its own.
type moveMode uint8

const (
	still moveMode = iota
	// gliding decelerates a released pan.
	gliding
	// springing brings the zoom back about an anchor and the heading to
	// north.
	springing
	// flying follows the path of van Wijk and Nuij from one camera to
	// another; see [flight].
	flying
	// zoomingAbout eases the zoom to a target and keeps a world point under
	// a screen point, for a double tap or the wheel.
	zoomingAbout
)

// mover is the camera of one map and its motion: the arithmetic of a
// MapKit view without any view. Positions are world points (package geo),
// the zoom is in levels and the heading in radians. All time is the clock of
// gift's painters and handlers.
type mover struct {
	// c, z, hd is the camera at rest, or where a motion started.
	c     geo.Point
	z, hd float64

	// minZ and maxZ are the zoom limits; extent is the height of the turned
	// viewport, w and h the viewport.
	minZ, maxZ float64
	w, h       float64

	// gesturing is set between begin and end; rz is the raw zoom under the
	// fingers, of which z is the rubber banded value.
	gesturing bool
	rz        float64

	mode moveMode
	t0   time.Duration
	// gliding: the velocity in world units per second.
	vx, vy float64
	// springing and zoomingAbout: the zoom and heading targets, and the
	// anchor kept in place, a world point and its screen position.
	toZ, toH float64
	anchorW  geo.Point
	anchorX  float64
	anchorY  float64
	dur      time.Duration
	// flying.
	fl flight
}

func newMover() mover { return mover{c: geo.Point{X: 0.5, Y: 0.5}, z: 2, minZ: 0, maxZ: 20} }

// at returns the camera at now and whether it is still moving.
func (m *mover) at(now time.Duration) (c geo.Point, z, hd float64, moving bool) {
	t := now - m.t0
	switch m.mode {
	case gliding:
		// Frozen where the speed drops below five points a second, so that
		// the camera a later gesture starts from is the one last drawn.
		stop := glide.Duration(math.Hypot(m.vx, m.vy)*scaleOf(m.z), 5)
		t = min(t, stop)
		dx, _ := glide.At(t, 0, m.vx)
		dy, _ := glide.At(t, 0, m.vy)
		c = geo.Point{X: m.c.X + dx, Y: m.c.Y + dy}
		c.Y = clampCenterY(c.Y, m.z, m.extent())
		return c, m.z, m.hd, now-m.t0 < stop
	case springing:
		z, _ = bounce.At(t, m.z, m.toZ, 0)
		hd, _ = bounce.At(t, m.hd, m.toH, 0)
		c = m.anchored(z, hd)
		done := bounce.Settled(t, m.z, m.toZ, 0, 1e-3) && bounce.Settled(t, m.hd, m.toH, 0, 1e-4)
		return c, z, hd, !done
	case zoomingAbout:
		t = min(t, m.dur)
		f := easeInOut(float64(t) / float64(m.dur))
		z = motion.Lerp(m.z, m.toZ, f)
		hd = motion.Lerp(m.hd, m.toH, f)
		return m.anchored(z, hd), z, hd, t < m.dur
	case flying:
		if t >= m.fl.dur {
			return m.fl.c1, m.fl.z1, m.fl.h1, false
		}
		c, z, hd = m.fl.at(t)
		return c, z, hd, true
	}
	return m.c, m.z, m.hd, false
}

// anchored is the centre that keeps the anchor's world point under its
// screen position at zoom z and heading hd.
func (m *mover) anchored(z, hd float64) geo.Point {
	v := newProjection(geo.Point{}, z, hd, m.w, m.h)
	d := v.worldVector(m.anchorX-m.w/2, m.anchorY-m.h/2)
	c := m.anchorW.Sub(d)
	c.Y = clampCenterY(c.Y, z, v.extentY())
	return c
}

func (m *mover) extent() float64 {
	s, c := math.Sincos(m.hd)
	return math.Abs(s)*m.w + math.Abs(c)*m.h
}

func (m *mover) projection() projection { return newProjection(m.c, m.z, m.hd, m.w, m.h) }

// settle stops any motion where it is at now.
func (m *mover) settle(now time.Duration) {
	m.c, m.z, m.hd, _ = m.at(now)
	m.mode = still
}

// setViewport tells the mover the size of the map, and with it the zoom at
// which the world just fills it.
func (m *mover) setViewport(w, h, minZoom, maxZoom float64) {
	m.w, m.h = w, h
	m.minZ = math.Max(minZoom, minZoomFor(m.extent()))
	m.maxZ = math.Max(m.minZ, maxZoom)
	if !m.gesturing && m.mode == still {
		m.z = math.Max(m.minZ, math.Min(m.maxZ, m.z))
		m.c.Y = clampCenterY(m.c.Y, m.z, m.extent())
	}
}

// begin starts a gesture: any motion stops where it is.
func (m *mover) begin(now time.Duration) {
	m.settle(now)
	m.gesturing, m.rz = true, m.z
}

// pan moves the map by a screen vector, as a finger moving by dx, dy does.
func (m *mover) pan(dx, dy float64) {
	d := m.projection().worldVector(dx, dy)
	m.c = m.c.Sub(d)
	m.c.Y = clampCenterY(m.c.Y, m.z, m.extent())
}

// pinch zooms by factor f about the screen point x0, y0, which then moves to
// x1, y1: the world point under the fingers stays under them.
func (m *mover) pinch(x0, y0, x1, y1, f float64) {
	v := m.projection()
	a := v.toWorld(x0, y0)
	m.rz += math.Log2(f)
	m.z = motion.Band(m.rz, m.minZ, m.maxZ, overZoom)
	m.anchorW, m.anchorX, m.anchorY = a, x1, y1
	m.c = m.anchored(m.z, m.hd)
}

// rotate turns the map by delta radians, clockwise on the screen positive,
// about the screen point x, y.
func (m *mover) rotate(x, y, delta float64) {
	a := m.projection().toWorld(x, y)
	m.hd = normalizeAngle(m.hd - delta)
	m.anchorW, m.anchorX, m.anchorY = a, x, y
	m.c = m.anchored(m.z, m.hd)
}

// end releases a gesture with a pan velocity in screen units per second,
// about the screen point where the fingers were. A zoom past its limits
// springs back about that point; a heading close to north turns to it;
// otherwise the pan glides on.
func (m *mover) end(now time.Duration, vx, vy, x, y float64) {
	m.gesturing = false
	m.t0 = now
	toZ := math.Max(m.minZ, math.Min(m.maxZ, m.z))
	toH := m.hd
	if math.Abs(m.hd) < northSnap {
		toH = 0
	}
	if toZ != m.z || toH != m.hd {
		m.anchorW, m.anchorX, m.anchorY = m.projection().toWorld(x, y), x, y
		if toZ == m.z {
			// A heading that snaps back turns about the middle, as the
			// compass does.
			m.anchorW, m.anchorX, m.anchorY = m.c, m.w/2, m.h/2
		}
		m.toZ, m.toH, m.mode = toZ, toH, springing
		return
	}
	d := m.projection().worldVector(vx, vy)
	m.vx, m.vy = -d.X, -d.Y
	if math.Hypot(vx, vy) < 50 {
		m.mode = still
		return
	}
	m.mode = gliding
}

// zoomAbout eases to zoom z and heading hd over d, keeping the world point
// under the screen point x, y where it is.
func (m *mover) zoomAbout(now time.Duration, z, hd, x, y float64, d time.Duration) {
	m.settle(now)
	m.t0, m.dur = now, d
	m.toZ = math.Max(m.minZ, math.Min(m.maxZ, z))
	m.toH = m.hd + normalizeAngle(hd-m.hd)
	m.anchorW, m.anchorX, m.anchorY = m.projection().toWorld(x, y), x, y
	m.mode = zoomingAbout
}

// flyTo moves to another camera along the path of van Wijk and Nuij, or
// jumps there when animated is false.
func (m *mover) flyTo(now time.Duration, c geo.Point, z, hd float64, animated bool) {
	m.settle(now)
	z = math.Max(m.minZ, math.Min(m.maxZ, z))
	if !animated || m.w <= 0 {
		m.c, m.z, m.hd, m.mode = c, z, normalizeAngle(hd), still
		return
	}
	m.t0 = now
	m.fl = newFlight(m.c, m.z, m.hd, c, z, hd, m.w)
	m.mode = flying
}

func normalizeAngle(a float64) float64 {
	a = math.Mod(a+math.Pi, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a - math.Pi
}

func easeInOut(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// flight is the zoom and pan path of J. J. van Wijk and W. A. A. Nuij,
// "Smooth and efficient zooming and panning" (InfoVis 2003): out, across and
// in again, the path along which the perceived speed is constant. It is the
// path of Mapbox's flyTo and of d3.interpolateZoom, whose formulation this
// follows. w is the viewport width in world units, W/scale.
type flight struct {
	ux0, uy0, w0 float64
	dx, dy       float64
	w1           float64
	r0, s        float64
	pureZoom     bool
	h0, dh       float64
	viewW        float64
	dur          time.Duration
	// c1, z1, h1 is the destination, returned exactly once the flight is
	// over rather than as the end of a path computed in floating point.
	c1     geo.Point
	z1, h1 float64
}

// rho is the curvature of the path: how far out it zooms, √2 in d3 and 1.42
// in Mapbox.
const rho = 1.42

func newFlight(c0 geo.Point, z0, h0 float64, c1 geo.Point, z1, h1, viewW float64) flight {
	f := flight{ux0: c0.X, uy0: c0.Y, viewW: viewW, h0: h0, dh: normalizeAngle(h1 - h0),
		c1: c1, z1: z1, h1: h0 + normalizeAngle(h1-h0)}
	f.w0, f.w1 = viewW/scaleOf(z0), viewW/scaleOf(z1)
	dx := c1.X - c0.X
	dx -= math.Round(dx) // the short way round the world
	f.dx, f.dy = dx, c1.Y-c0.Y
	d2 := f.dx*f.dx + f.dy*f.dy
	if d2 < 1e-24 {
		f.pureZoom = true
		f.s = math.Abs(math.Log(f.w1/f.w0)) / rho
	} else {
		d1 := math.Sqrt(d2)
		b0 := (f.w1*f.w1 - f.w0*f.w0 + rho*rho*rho*rho*d2) / (2 * f.w0 * rho * rho * d1)
		b1 := (f.w1*f.w1 - f.w0*f.w0 - rho*rho*rho*rho*d2) / (2 * f.w1 * rho * rho * d1)
		f.r0 = math.Log(math.Sqrt(b0*b0+1) - b0)
		r1 := math.Log(math.Sqrt(b1*b1+1) - b1)
		f.s = (r1 - f.r0) / rho
	}
	// Mapbox's default speed is 1.2 "screenfuls" per second along the path;
	// clamped so that a hop is not instant and a flight across the world
	// does not keep the user waiting.
	secs := math.Max(0.35, math.Min(3, f.s/1.2))
	if f.s == 0 {
		secs = 0.35
	}
	f.dur = time.Duration(secs * float64(time.Second))
	return f
}

// at returns the camera t into the flight.
func (f flight) at(t time.Duration) (geo.Point, float64, float64) {
	u := easeInOut(float64(t) / float64(f.dur))
	hd := f.h0 + f.dh*u
	if f.pureZoom {
		w := f.w0 * math.Exp(math.Log(f.w1/f.w0)*u)
		return geo.Point{X: f.ux0, Y: f.uy0}, math.Log2(f.viewW / (w * TileSize)), hd
	}
	s := u * f.s
	d1 := math.Hypot(f.dx, f.dy)
	coshr0 := math.Cosh(f.r0)
	k := f.w0 / (rho * rho * d1) * (coshr0*math.Tanh(rho*s+f.r0) - math.Sinh(f.r0))
	w := f.w0 * coshr0 / math.Cosh(rho*s+f.r0)
	c := geo.Point{X: f.ux0 + k*f.dx, Y: f.uy0 + k*f.dy}
	return c, math.Log2(f.viewW / (w * TileSize)), hd
}
