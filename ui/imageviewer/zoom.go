package imageviewer

import (
	"math"
	"time"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui/motion"
)

// The motion of a zoomed picture.
var (
	// bounce brings a picture back from past its zoom limits or its edges.
	// It is critically damped with a response of 0.6 s, an angular
	// frequency of about 10.5 rad/s: the spring ScrollMechanics measured
	// against UIScrollView, slower than the scroll containers' edge spring
	// because a whole picture moves rather than a strip of a list.
	bounce = motion.Spring{Response: 600 * time.Millisecond, Damping: 1}
	// zoomSpring is a zoom the user asked for with a double tap, a key or a
	// button: quick and without overshoot.
	zoomSpring = motion.Spring{Response: 400 * time.Millisecond, Damping: 1}
	// decay is the free deceleration of a released pan.
	decay = motion.Decay{Rate: motion.DecelerationNormal}
)

// The resistance of the zoom past its limits, as the logarithm of the factor
// the rubber band approaches: a picture pinched past its largest zoom can be
// stretched by half as much again at most, and one pinched below its fit
// shrinks to half at most. iOS does not document its curve; this is
// [motion.Rubber] applied to the logarithm of the scale, which is what makes
// the resistance the same at every zoom.
var (
	overZoom  = math.Log(1.5)
	underZoom = math.Log(2)
)

// zoomMode is what a [zoomer] is doing on its own.
type zoomMode uint8

const (
	still zoomMode = iota
	// springing interpolates from one zoom and offset to another.
	springing
	// gliding decelerates a released pan and bounces off the edges.
	gliding
)

// zoomer is the zoom and pan of one picture in a viewport: the arithmetic of a
// UIScrollView with a zoomable content view, without any view.
//
// The picture's centre sits at the viewport's centre plus (ox, oy), and it is
// z times the size at which it fits the viewport. z = 1 is the fit and the
// smallest resting zoom; maxZoom the largest. A picture smaller than the
// viewport along an axis is centred on it, as Photos does.
//
// All positions are in the viewport's own coordinates, with the origin at its
// top left corner. All time is the clock of gift's painters and handlers.
type zoomer struct {
	vw, vh  float64
	aspect  float64 // width over height of the picture; 0 is unknown
	maxZoom float64

	// z, ox, oy is the state at rest, or the state a motion started from.
	z, ox, oy float64

	// gesturing is set between begin and end. rz, rx, ry are the raw values
	// under the fingers; what is shown is them with the rubber band.
	gesturing  bool
	rz, rx, ry float64

	mode   zoomMode
	t0     time.Duration
	spring motion.Spring
	to     [3]float64
	gx, gy motion.Glide
}

func newZoomer() zoomer { return zoomer{z: 1, maxZoom: 4} }

// fitted is the size of the picture at z = 1.
func (m *zoomer) fitted() (fw, fh float64) {
	if m.aspect <= 0 || m.vw <= 0 || m.vh <= 0 {
		return m.vw, m.vh
	}
	if m.vw/m.vh > m.aspect {
		return m.vh * m.aspect, m.vh
	}
	return m.vw, m.vw / m.aspect
}

// fill is the zoom at which the picture covers the viewport.
func (m *zoomer) fill() float64 {
	fw, fh := m.fitted()
	if fw <= 0 || fh <= 0 {
		return 1
	}
	return math.Max(m.vw/fw, m.vh/fh)
}

// bounds are the largest offsets at zoom z: how far the picture may move
// before an edge comes away from the viewport's edge.
func (m *zoomer) bounds(z float64) (bx, by float64) {
	fw, fh := m.fitted()
	return math.Max(0, (fw*z-m.vw)/2), math.Max(0, (fh*z-m.vh)/2)
}

// setViewport adopts a new viewport and picture aspect. A picture at rest
// keeps its zoom and is pulled back inside its edges.
func (m *zoomer) setViewport(vw, vh, aspect float64) {
	if vw == m.vw && vh == m.vh && aspect == m.aspect {
		return
	}
	m.vw, m.vh, m.aspect = vw, vh, aspect
	if m.mode == still && !m.gesturing {
		bx, by := m.bounds(m.z)
		m.ox, m.oy = clamp(m.ox, -bx, bx), clamp(m.oy, -by, by)
	}
}

// at returns the zoom and offset at now, and whether the picture is still
// moving by itself.
func (m *zoomer) at(now time.Duration) (z, ox, oy float64, moving bool) {
	switch m.mode {
	case springing:
		t := now - m.t0
		f, _ := m.spring.At(t, 0, 1, 0)
		z = math.Exp(motion.Lerp(math.Log(m.z), math.Log(m.to[0]), f))
		ox = motion.Lerp(m.ox, m.to[1], f)
		oy = motion.Lerp(m.oy, m.to[2], f)
		return z, ox, oy, !m.spring.Settled(t, 0, 1, 0, 0.001)
	case gliding:
		t := now - m.t0
		x, _ := m.gx.At(t)
		y, _ := m.gy.At(t)
		return m.z, x, y, !m.gx.Done(t, 0.25) || !m.gy.Done(t, 0.25)
	}
	return m.z, m.ox, m.oy, false
}

// rect is the picture's rectangle at zoom z and offset (ox, oy).
func (m *zoomer) rect(z, ox, oy float64) geom.Rect {
	fw, fh := m.fitted()
	w, h := fw*z, fh*z
	cx, cy := m.vw/2+ox, m.vh/2+oy
	return geom.Rc(float32(cx-w/2), float32(cy-h/2), float32(cx+w/2), float32(cy+h/2))
}

// rectAt is the picture's rectangle at now.
func (m *zoomer) rectAt(now time.Duration) geom.Rect {
	z, ox, oy, _ := m.at(now)
	return m.rect(z, ox, oy)
}

// zoomed reports whether the picture at rest is larger than its fit.
func (m *zoomer) zoomed() bool { return m.z > 1.01 || m.mode == springing && m.to[0] > 1.01 }

// settle stops any motion where it is now.
func (m *zoomer) settle(now time.Duration) {
	m.z, m.ox, m.oy, _ = m.at(now)
	m.mode = still
}

// begin takes hold of the picture for a gesture: whatever it was doing stops
// where it is, and the raw values are those that show as that.
func (m *zoomer) begin(now time.Duration) {
	m.settle(now)
	m.gesturing = true
	m.rz = m.unbandZoom(m.z)
	bx, by := m.bounds(m.z)
	m.rx = unband(m.ox, -bx, bx, m.vw)
	m.ry = unband(m.oy, -by, by, m.vh)
}

// pan moves the picture under a finger by (dx, dy), with the rubber band past
// its edges. When overflow is set it is offered the part of dx that went past
// an edge, and the picture stops at the edge if it takes it: a pager hands
// that part on to the next page rather than rubber banding the picture.
func (m *zoomer) pan(dx, dy float64, overflow func(rest float64) bool) {
	bx, by := m.bounds(m.z)
	want := m.rx + dx
	if overflow != nil {
		c := clamp(want, -bx, bx)
		if rest := want - c; rest != 0 && overflow(rest) {
			want = c
		}
	}
	m.rx, m.ry = want, m.ry+dy
	m.ox = motion.Band(m.rx, -bx, bx, m.vw)
	m.oy = motion.Band(m.ry, -by, by, m.vh)
}

// pinch scales the picture by the factor f about the fingers: the point of
// the picture that was under the centroid c0 ends up under the centroid c1,
// which pans the picture with fingers that move together.
func (m *zoomer) pinch(c0, c1 geom.Point, f float64) {
	px := (float64(c0.X) - (m.vw/2 + m.ox)) / m.z
	py := (float64(c0.Y) - (m.vh/2 + m.oy)) / m.z
	m.rz *= f
	m.z = m.bandZoom(m.rz)
	m.rx = float64(c1.X) - px*m.z - m.vw/2
	m.ry = float64(c1.Y) - py*m.z - m.vh/2
	bx, by := m.bounds(m.z)
	m.ox = motion.Band(m.rx, -bx, bx, m.vw)
	m.oy = motion.Band(m.ry, -by, by, m.vh)
}

// end lets go of the picture at now with the velocity (vx, vy) of the
// fingers. A zoom past its limits springs back to the nearest limit about
// anchor; otherwise the pan glides on and bounces off the edges.
func (m *zoomer) end(now time.Duration, vx, vy float64, anchor geom.Point) {
	m.gesturing = false
	switch {
	case m.z < 1-1e-6 || m.z > m.maxZoom+1e-6:
		zt := clamp(m.z, 1, m.maxZoom)
		ox, oy := m.anchored(zt, anchor)
		m.animate(now, zt, ox, oy, bounce)
	default:
		bx, by := m.bounds(m.z)
		m.gx = motion.NewGlide(m.ox, vx, -bx, bx, decay, bounce)
		m.gy = motion.NewGlide(m.oy, vy, -by, by, decay, bounce)
		m.t0, m.mode = now, gliding
		if _, _, _, moving := m.at(now + time.Millisecond); !moving {
			m.mode = still
		}
	}
}

// anchored is the offset at zoom zt that keeps the point of the picture under
// p where it is, pulled inside the edges.
func (m *zoomer) anchored(zt float64, p geom.Point) (ox, oy float64) {
	px := (float64(p.X) - (m.vw/2 + m.ox)) / m.z
	py := (float64(p.Y) - (m.vh/2 + m.oy)) / m.z
	ox = float64(p.X) - px*zt - m.vw/2
	oy = float64(p.Y) - py*zt - m.vh/2
	bx, by := m.bounds(zt)
	return clamp(ox, -bx, bx), clamp(oy, -by, by)
}

// zoomTo animates to zoom zt about p.
func (m *zoomer) zoomTo(now time.Duration, zt float64, p geom.Point) {
	m.settle(now)
	zt = clamp(zt, 1, m.maxZoom)
	ox, oy := m.anchored(zt, p)
	m.animate(now, zt, ox, oy, zoomSpring)
}

// toggle is the double tap of Photos: a picture at its fit zooms in about the
// tap, far enough to fill the screen and at least to twice its fit; any other
// zoom goes back to the fit.
func (m *zoomer) toggle(now time.Duration, p geom.Point, target float64) {
	m.settle(now)
	if m.z > 1.01 {
		m.animate(now, 1, 0, 0, zoomSpring)
		return
	}
	if target <= 0 {
		target = math.Max(m.fill(), 2)
	}
	m.zoomTo(now, target, p)
}

// scale zooms at once by f about p, inside the limits: a wheel notch.
func (m *zoomer) scale(now time.Duration, f float64, p geom.Point) {
	m.settle(now)
	zt := clamp(m.z*f, 1, m.maxZoom)
	m.ox, m.oy = m.anchored(zt, p)
	m.z = zt
}

// reset puts the picture back at its fit, at once.
func (m *zoomer) reset() {
	m.z, m.ox, m.oy, m.mode, m.gesturing = 1, 0, 0, still, false
}

func (m *zoomer) animate(now time.Duration, z, ox, oy float64, s motion.Spring) {
	m.to = [3]float64{z, ox, oy}
	m.t0, m.spring, m.mode = now, s, springing
}

// bandZoom and unbandZoom are the rubber band of the zoom past its limits.
func (m *zoomer) bandZoom(z float64) float64 {
	switch {
	case z > m.maxZoom:
		return m.maxZoom * math.Exp(motion.Rubber(math.Log(z/m.maxZoom), overZoom))
	case z < 1 && z > 0:
		return math.Exp(motion.Rubber(math.Log(z), underZoom))
	}
	return z
}

func (m *zoomer) unbandZoom(z float64) float64 {
	switch {
	case z > m.maxZoom:
		return m.maxZoom * math.Exp(motion.RubberInverse(math.Log(z/m.maxZoom), overZoom))
	case z < 1 && z > 0:
		return math.Exp(motion.RubberInverse(math.Log(z), underZoom))
	}
	return z
}

// unband is the raw position that [motion.Band] shows as x.
func unband(x, lo, hi, dim float64) float64 {
	switch {
	case x < lo:
		return lo + motion.RubberInverse(x-lo, dim)
	case x > hi:
		return hi + motion.RubberInverse(x-hi, dim)
	}
	return x
}

func clamp(x, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, x)) }
