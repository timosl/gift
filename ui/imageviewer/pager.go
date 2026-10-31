package imageviewer

import (
	"math"
	"time"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui/motion"
)

// The motion of the pages and of what is dragged vertically.
var (
	// pageSpring settles a page after the finger lifts or a key turned it.
	pageSpring = motion.Spring{Response: 420 * time.Millisecond, Damping: 1}
	// returnSpring brings a picture back that was dragged down and not far
	// enough to close: a little lively, like the spring of Apple's photo
	// transition sample (damping ratio 0.62), but less so.
	returnSpring = motion.Spring{Response: 400 * time.Millisecond, Damping: 0.8}
	// cardSpring opens and closes the card below a picture.
	cardSpring = motion.Spring{Response: 380 * time.Millisecond, Damping: 0.9}
)

// The thresholds of the paging and the vertical gestures, from Apple's photo
// transition sample and from Flutter's page physics.
const (
	// flickSpeed is the speed above which a release turns the page in the
	// direction it was moving, however little it had moved.
	flickSpeed = 400
	// dismissSpeed is the speed above which a downward release closes the
	// viewer and an upward one keeps it open, whatever the distance.
	dismissSpeed = 1200
	// dismissProgress is the progress above which a slower release closes.
	dismissProgress = 0.33
	// maxPageVelocity caps the velocity a page spring starts with, so that a
	// violent flick does not overshoot into the page after the next.
	maxPageVelocity = 4000
)

// pager is the horizontal position of the pages: the offset of the current
// page from its resting place, positive towards the next page.
type pager struct {
	off, raw float64
	moving   bool
	t0       time.Duration
	v0       float64
}

func (p *pager) at(now time.Duration) (off float64, moving bool) {
	if !p.moving {
		return p.off, false
	}
	t := now - p.t0
	x, _ := pageSpring.At(t, p.off, 0, p.v0)
	return x, !pageSpring.Settled(t, p.off, 0, p.v0, 0.25)
}

// settle stops the pages where they are now.
func (p *pager) settle(now time.Duration) {
	p.off, _ = p.at(now)
	p.raw, p.moving = p.off, false
}

// show sets the displayed offset from the raw one: free between the
// neighbours, rubber banded past the first and the last page.
func (p *pager) show(hasPrev, hasNext bool, pitch, vw float64) {
	r := p.raw
	switch {
	case r < 0 && !hasPrev, r > 0 && !hasNext:
		p.off = motion.Rubber(r, vw)
	default:
		p.off = clamp(r, -pitch, pitch)
	}
}

// release decides where the pages go when the finger lifts with velocity v
// towards the next page: a flick turns towards where it went, anything slower
// to the nearer page. It returns the direction to turn in, or zero.
func (p *pager) release(v, pitch float64, hasPrev, hasNext bool) int {
	dir := 0
	switch {
	case v > flickSpeed && p.off > -pitch/2:
		dir = 1
	case v < -flickSpeed && p.off < pitch/2:
		dir = -1
	case math.Abs(v) <= flickSpeed && pitch > 0:
		dir = int(math.Round(p.off / pitch))
	}
	if dir > 0 && !hasNext || dir < 0 && !hasPrev {
		dir = 0
	}
	return max(-1, min(1, dir))
}

// spring lets the pages move to their resting place from offset off with
// velocity v.
func (p *pager) spring(now time.Duration, off, v float64) {
	p.off, p.raw = off, off
	p.v0 = clamp(v, -maxPageVelocity, maxPageVelocity)
	p.t0, p.moving = now, true
}

// swipe is a picture dragged down to close the viewer: it follows the finger
// and shrinks, and the background fades with it.
type swipe struct {
	active bool
	t      geom.Point // translation
	anchor geom.Point // where the finger started, in the viewport
	back   bool       // springing back after a release that did not close
	t0     time.Duration
	from   geom.Point
	v0     geom.Point
}

// progress is how far the swipe has gone towards closing, from zero to one:
// half the viewport's height is all the way, as in Apple's sample.
func (s *swipe) progress(vh float64, t geom.Point) float64 {
	if vh <= 0 {
		return 0
	}
	return clamp(float64(t.Y)/(vh/2), 0, 1)
}

// at is the translation at now.
func (s *swipe) at(now time.Duration) (t geom.Point, moving bool) {
	if !s.back {
		return s.t, false
	}
	d := now - s.t0
	x, _ := returnSpring.At(d, float64(s.from.X), 0, float64(s.v0.X))
	y, _ := returnSpring.At(d, float64(s.from.Y), 0, float64(s.v0.Y))
	done := returnSpring.Settled(d, float64(s.from.X), 0, float64(s.v0.X), 0.25) &&
		returnSpring.Settled(d, float64(s.from.Y), 0, float64(s.v0.Y), 0.25)
	return geom.Pt(float32(x), float32(y)), !done
}

// scale is the size of the picture at progress f: it shrinks to two thirds
// while it is dragged all the way.
func swipeScale(f float64) float64 { return 1 - f/3 }

// apply moves the rectangle r of the picture as a swipe at translation t
// shows it: scaled about where the finger started, then moved with it.
func (s *swipe) apply(r geom.Rect, t geom.Point, vh float64) geom.Rect {
	k := float32(swipeScale(s.progress(vh, t)))
	a := s.anchor
	return geom.Rc(
		a.X+(r.Min.X-a.X)*k+t.X, a.Y+(r.Min.Y-a.Y)*k+t.Y,
		a.X+(r.Max.X-a.X)*k+t.X, a.Y+(r.Max.Y-a.Y)*k+t.Y)
}

// card is the panel below a picture that a swipe up reveals: its position
// from closed, zero, to open, one.
type card struct {
	q, raw float64
	moving bool
	t0     time.Duration
	from   float64
	to     float64
	v0     float64
}

func (c *card) at(now time.Duration) (q float64, moving bool) {
	if !c.moving {
		return c.q, false
	}
	d := now - c.t0
	x, _ := cardSpring.At(d, c.from, c.to, c.v0)
	return x, !cardSpring.Settled(d, c.from, c.to, c.v0, 0.002)
}

func (c *card) settle(now time.Duration) {
	c.q, _ = c.at(now)
	c.raw, c.moving = c.q, false
}

// open reports whether the card is open or opening.
func (c *card) open() bool { return c.moving && c.to > 0.5 || !c.moving && c.q > 0.5 }

// drag moves the card by d of its height, rubber banded past open.
func (c *card) drag(d float64) {
	c.raw += d
	switch {
	case c.raw > 1:
		c.q = 1 + motion.Rubber(c.raw-1, 1)
	case c.raw < 0:
		c.q = 0
	default:
		c.q = c.raw
	}
}

// animate lets the card go to target, with velocity v in heights per second.
func (c *card) animate(now time.Duration, target, v float64) {
	c.from, c.to, c.v0 = c.q, target, v
	c.t0, c.moving = now, true
	c.q = c.from
}
