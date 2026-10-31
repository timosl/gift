// Package motion is the arithmetic of things that move under a finger and
// after it: the rubber band past an edge, the deceleration of a fling, and the
// spring that brings something home. It is what a UIScrollView does, written
// down as functions of time.
//
// It has no state of its own and knows nothing about views, events or frames.
// Every value here is a closed form evaluated at a time the caller supplies,
// on the clock gift hands to painters and handlers — [gift.PaintContext.Now]
// — so an animation is exactly reproducible in a test that moves that clock,
// and costs nothing while nobody looks at it.
//
// # The constants, and where they come from
//
//   - The rubber band is UIKit's: a pull of x past an edge of a viewport of
//     size d shows as (1 − 1/(x·c/d + 1))·d with c = 0.55. It approaches d and
//     never reaches it. See [Rubber].
//   - A fling decays like UIScrollView: the velocity is multiplied by a rate
//     per millisecond, 0.998 for [DecelerationNormal] and 0.99 for
//     [DecelerationFast]. Where it comes to rest is [Project], the formula of
//     Apple's "Designing Fluid Interfaces" (WWDC 2018).
//   - Springs are parametrised as SwiftUI's are, by a response and a damping
//     fraction, rather than by mass, stiffness and damping. A response is the
//     period of the undamped oscillation, which is a duration a designer can
//     reason about; a damping of one is critically damped and never
//     overshoots. See [Spring].
package motion

import (
	"math"
	"time"
)

// rubberCoefficient is UIKit's c in the rubber band formula.
const rubberCoefficient = 0.55

// Rubber returns the displacement shown for a pull of x past an edge, in a
// viewport of size dim along the same axis: UIKit's (1 − 1/(x·c/dim + 1))·dim
// with c = 0.55. The sign of x is kept. A non positive dim shows nothing.
//
// The curve starts with a slope of c — just over half of the finger's travel —
// and flattens, so the content follows with growing resistance and never moves
// further than dim however far it is pulled.
func Rubber(x, dim float64) float64 {
	if !(dim > 0) {
		return 0
	}
	a := math.Abs(x)
	r := (1 - 1/(a*rubberCoefficient/dim+1)) * dim
	return math.Copysign(r, x)
}

// RubberInverse is the pull that [Rubber] shows as r: the finger travel that
// produced a displacement. A handler that starts a drag while content is
// already displaced past its edge — caught in the middle of a bounce — uses it
// to continue from the same place instead of jumping.
func RubberInverse(r, dim float64) float64 {
	if !(dim > 0) {
		return 0
	}
	a := math.Min(math.Abs(r), dim*0.999999)
	x := (1/(1-a/dim) - 1) * dim / rubberCoefficient
	return math.Copysign(x, r)
}

// Band maps a position to what is shown for it when the content may be dragged
// between lo and hi and rubber bands beyond: positions inside are unchanged,
// positions outside are pulled back towards the edge by [Rubber] in a
// viewport of dim.
func Band(x, lo, hi, dim float64) float64 {
	switch {
	case x < lo:
		return lo + Rubber(x-lo, dim)
	case x > hi:
		return hi + Rubber(x-hi, dim)
	}
	return x
}

// The deceleration rates of UIScrollView, per millisecond.
const (
	// DecelerationNormal is UIScrollView.DecelerationRate.normal: a fling
	// keeps 99.8 % of its speed per millisecond.
	DecelerationNormal = 0.998
	// DecelerationFast is UIScrollView.DecelerationRate.fast, which paging
	// scroll views and pickers use.
	DecelerationFast = 0.99
)

// Project returns how far a fling with velocity v, in units per second, travels
// before it comes to rest under the deceleration rate per millisecond: Apple's
// v/1000 · rate/(1 − rate). A paging view decides where a release goes from
// the projected position rather than from where the finger lifted, which is
// why a short quick flick turns the page.
func Project(v, rate float64) float64 {
	if !(rate > 0 && rate < 1) {
		return 0
	}
	return v / 1000 * rate / (1 - rate)
}

// Decay is the free deceleration of a fling: a velocity that is multiplied by
// Rate every millisecond.
type Decay struct {
	// Rate is the factor per millisecond, in (0, 1); see
	// [DecelerationNormal].
	Rate float64
}

// k is the decay constant per second, negative.
func (d Decay) k() float64 {
	r := d.Rate
	if !(r > 0 && r < 1) {
		r = DecelerationNormal
	}
	return math.Log(r) * 1000
}

// At returns the position and velocity t after a release at x0 with velocity
// v0, in units and units per second.
func (d Decay) At(t time.Duration, x0, v0 float64) (x, v float64) {
	k := d.k()
	s := t.Seconds()
	e := math.Exp(k * s)
	return x0 + v0*(e-1)/k, v0 * e
}

// Rest is where a release at x0 with velocity v0 comes to rest. It is the
// limit of [Decay.At], which differs from [Project] by the discretisation
// Apple's formula carries: under one percent at the normal rate.
func (d Decay) Rest(x0, v0 float64) float64 { return x0 - v0/d.k() }

// Duration is how long a release with velocity v0 takes until its speed falls
// below minSpeed units per second.
func (d Decay) Duration(v0, minSpeed float64) time.Duration {
	a := math.Abs(v0)
	if a <= minSpeed || minSpeed <= 0 {
		return 0
	}
	return seconds(math.Log(minSpeed/a) / d.k())
}

// crossing returns the time at which a release at x0 with v0 reaches b, and
// whether it does at all before it comes to rest.
func (d Decay) crossing(x0, v0, b float64) (time.Duration, bool) {
	if v0 == 0 {
		return 0, false
	}
	k := d.k()
	arg := 1 + k*(b-x0)/v0
	if !(arg > 0 && arg <= 1) {
		return 0, false
	}
	return seconds(math.Log(arg) / k), true
}

// Spring is a damped spring in SwiftUI's parametrisation.
//
// Response is the period of the spring without damping: the time a full
// swing would take. Damping is the fraction of critical damping: 1 settles as
// fast as possible without overshooting, below 1 overshoots and swings, above
// 1 creeps. The zero Spring is [Smooth].
type Spring struct {
	Response time.Duration
	Damping  float64
}

// The springs gift uses. The names are SwiftUI's, and so are the values.
var (
	// Smooth is SwiftUI's .smooth: half a second, no bounce. It is the zero
	// value of [Spring].
	Smooth = Spring{Response: 500 * time.Millisecond, Damping: 1}
	// Snappy is SwiftUI's .snappy: quicker, with a trace of overshoot.
	Snappy = Spring{Response: 500 * time.Millisecond, Damping: 0.85}
	// Bouncy is SwiftUI's .bouncy.
	Bouncy = Spring{Response: 500 * time.Millisecond, Damping: 0.7}
	// Edge is the spring that brings content back from past an edge, the
	// one gift's scroll containers use: critically damped at 14 rad/s, a
	// response of 2π/14 s, so a pulled list is home in about a third of a
	// second.
	Edge = Spring{Response: 448799 * time.Microsecond, Damping: 1}
)

func (s Spring) params() (w, z float64) {
	if s.Response <= 0 {
		s = Smooth
	}
	z = s.Damping
	if z <= 0 {
		z = 1
	}
	return 2 * math.Pi / s.Response.Seconds(), z
}

// At returns the position and velocity t after the spring was let go at from
// with velocity v0, pulling towards to. Velocities are in units per second.
func (s Spring) At(t time.Duration, from, to, v0 float64) (x, v float64) {
	w, z := s.params()
	ts := t.Seconds()
	x0 := from - to
	var dx, dv float64
	switch {
	case math.Abs(z-1) < 1e-6:
		e := math.Exp(-w * ts)
		c := v0 + w*x0
		dx = (x0 + c*ts) * e
		dv = (v0 - w*c*ts) * e
	case z < 1:
		wd := w * math.Sqrt(1-z*z)
		e := math.Exp(-z * w * ts)
		c := (v0 + z*w*x0) / wd
		cos, sin := math.Cos(wd*ts), math.Sin(wd*ts)
		dx = e * (x0*cos + c*sin)
		dv = e * ((c*wd-z*w*x0)*cos - (x0*wd+z*w*c)*sin)
	default:
		r := w * math.Sqrt(z*z-1)
		r1, r2 := -z*w+r, -z*w-r
		c2 := (v0 - r1*x0) / (r2 - r1)
		c1 := x0 - c2
		e1, e2 := math.Exp(r1*ts), math.Exp(r2*ts)
		dx = c1*e1 + c2*e2
		dv = c1*r1*e1 + c2*r2*e2
	}
	return to + dx, dv
}

// Settled reports whether the spring is within eps of its target and slower
// than ten eps per second, the point at which a frame more or less is
// invisible.
func (s Spring) Settled(t time.Duration, from, to, v0, eps float64) bool {
	x, v := s.At(t, from, to, v0)
	return math.Abs(x-to) < eps && math.Abs(v) < eps*10
}

// Duration is how long the spring takes from a standing start to settle
// within eps of a target dist away: an upper bound for an animation window
// such as [gift.EventContext.Animate].
func (s Spring) Duration(dist, v0, eps float64) time.Duration {
	if eps <= 0 {
		eps = 0.5
	}
	for t := time.Duration(0); t < 10*time.Second; t += 8 * time.Millisecond {
		if s.Settled(t, dist, 0, v0, eps) {
			return t
		}
	}
	return 10 * time.Second
}

// Glide is the motion of content let go between two edges: it decelerates
// freely while it is inside [Lo, Hi], and a spring brings it back to the edge
// it crossed or was already past. It is what a UIScrollView does after the
// finger lifts, as one function of time.
type Glide struct {
	Decay  Decay
	Spring Spring
	// Lo and Hi bound the resting position. Lo > Hi is read as Lo == Hi.
	Lo, Hi float64

	x0, v0 float64
	// edge is the bound the spring pulls to, and springAt when it takes
	// over; a glide that never leaves [Lo, Hi] has springAt < 0.
	edge     float64
	springAt time.Duration
	// sx, sv are the position and velocity when the spring took over.
	sx, sv float64
}

// NewGlide starts a glide at x0 with velocity v0, in units and units per
// second.
func NewGlide(x0, v0, lo, hi float64, d Decay, s Spring) Glide {
	if hi < lo {
		hi = lo
	}
	g := Glide{Decay: d, Spring: s, Lo: lo, Hi: hi, x0: x0, v0: v0, springAt: -1}
	switch {
	case x0 < lo:
		g.edge, g.springAt, g.sx, g.sv = lo, 0, x0, v0
	case x0 > hi:
		g.edge, g.springAt, g.sx, g.sv = hi, 0, x0, v0
	default:
		rest := d.Rest(x0, v0)
		b := lo
		if rest > hi {
			b = hi
		}
		if rest < lo || rest > hi {
			if t, ok := d.crossing(x0, v0, b); ok {
				_, v := d.At(t, x0, v0)
				g.edge, g.springAt, g.sx, g.sv = b, t, b, v
			}
		}
	}
	return g
}

// At returns the position and velocity t after the release.
func (g Glide) At(t time.Duration) (x, v float64) {
	if g.springAt >= 0 && t >= g.springAt {
		return g.Spring.At(t-g.springAt, g.sx, g.edge, g.sv)
	}
	return g.Decay.At(t, g.x0, g.v0)
}

// Rest is where the glide comes to rest.
func (g Glide) Rest() float64 {
	if g.springAt >= 0 {
		return g.edge
	}
	return g.Decay.Rest(g.x0, g.v0)
}

// Done reports whether the glide has come to rest within eps.
func (g Glide) Done(t time.Duration, eps float64) bool {
	x, v := g.At(t)
	return math.Abs(x-g.Rest()) < eps && math.Abs(v) < eps*20
}

// Lerp interpolates from a to b by f.
func Lerp(a, b, f float64) float64 { return a + (b-a)*f }

func seconds(s float64) time.Duration {
	if !(s > 0) {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}
