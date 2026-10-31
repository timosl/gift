package gift

import (
	"math"
	"time"

	"github.com/worldiety/gift/internal/scene"
)

// The rubber band of iOS, for every scroll container unless its
// [ScrollConfig.HardStop] is set.
//
// Dragged past an end, the content follows the finger with growing
// resistance – the curve UIKit uses, where the displacement approaches the
// viewport size but never reaches it. Let go, it springs back. A fling that
// reaches an end with speed left bounces off it and settles.
//
// It is all translation: the offset stays within its bounds, and what goes
// past them is an extra displacement in the container's transform. Nothing is
// laid out or built for it, and it costs a repaint of the container while it
// moves.

// bounceStiffness is the angular frequency of the spring back in 1/s. At 14
// a pulled list is home in about a third of a second, and a bounce off an end
// overshoots by a little over a hundredth of the fling speed.
const bounceStiffness = 14

// rubber is the displacement shown for a pull of p past an end, for a
// viewport of size dim: UIKit's (1 - 1/(p·c/dim + 1))·dim with c = 0.55.
func rubber(p, dim float32) float32 {
	if !(dim > 0) {
		return 0
	}
	a := p
	if a < 0 {
		a = -a
	}
	r := (1 - 1/(a*0.55/dim+1)) * dim
	if p < 0 {
		return -r
	}
	return r
}

// rubberInverse is the pull that shows the displacement r: the inverse of
// rubber, for a finger that takes hold of content which is already
// displaced, by a hold or by a bounce under way.
func rubberInverse(r, dim float32) float32 {
	if !(dim > 0) || r == 0 {
		return 0
	}
	a := min(absf(r), dim*0.999)
	p := (1/(1-a/dim) - 1) * dim / 0.55
	if r < 0 {
		return -p
	}
	return p
}

// rest is the displacement the content comes to rest at: past the leading end
// by the hold of [ScrollSpec.LeadingHold] while it is at that end, and none
// otherwise.
func (s *scrollState) rest() float32 {
	if s.hold > 0 && s.off <= 0 {
		return -s.hold
	}
	return 0
}

// dragBounce moves a bouncing container by d offset units under a finger.
// Within the bounds it scrolls; past them the rest becomes pull.
func (a *App) dragBounce(h scene.Handle, s *scrollState, d float64) {
	s.bouncing = false
	if s.pull == 0 && s.over != 0 {
		// Taken hold of while displaced: carry on from where it shows
		// rather than jumping back to the end.
		s.pull = rubberInverse(s.over, s.viewport)
	}
	if s.pull != 0 {
		p := s.pull + float32(d)
		if (p > 0) == (s.pull > 0) && p != 0 {
			s.pull = p
			s.over = rubber(p, s.viewport)
			a.markNeedsPaint(h)
			return
		}
		// Back across the end: the remainder scrolls.
		d = float64(p)
		s.pull, s.over = 0, 0
		a.markNeedsPaint(h)
	}
	target := s.off + d
	clamped := s.clamp(target)
	a.setScroll(h, s, clamped)
	if p := float32(target - clamped); p != 0 {
		s.pull = p
		s.over = rubber(p, s.viewport)
		a.markNeedsPaint(h)
	}
}

// wheelRelease is how long a precise wheel has to be quiet before the
// stretch it made springs back. GLFW reports no phases of a trackpad gesture,
// so the end of the stroke, and of the momentum macOS adds to it, is the
// moment the deltas stop; they come with every frame until then.
const wheelRelease = 120 * time.Millisecond

// wheelBounce scrolls a container by d offset units for a precise wheel, a
// trackpad, with the rubber band of a finger: past an end the content
// stretches with growing resistance, and it springs back once the wheel has
// been quiet for wheelRelease, as on macOS. It reports whether it took the
// scroll.
//
// A stroke that starts at an end, pointing past it, goes on to an enclosing
// container that can still move that way, as a browser hands a scroll on; only
// where none can does this one stretch. A stroke this container took stays
// with it to the end, momentum included, which is what makes a list scrolled
// into its end bounce rather than drag the page along. Content that fits does
// not stretch, as macOS leaves it.
func (a *App) wheelBounce(h scene.Handle, s *scrollState, d float64) bool {
	if !s.wheeling && s.pull == 0 && !s.canMove(d) {
		if !(s.maxOffset() > 0) || a.outerCanScroll(h, s.axis, d) {
			return false
		}
	}
	s.flinging, s.vel = false, 0
	a.dragBounce(h, s, d)
	s.wheeling, s.wheelAt = true, a.in.now
	a.enrolScroll(h)
	return true
}

// outerCanScroll reports whether a scroll container around h moves along axis
// when scrolled by d.
func (a *App) outerCanScroll(h scene.Handle, axis ScrollAxis, d float64) bool {
	for p := a.store.Get(h).Parent; !p.IsZero() && a.store.Valid(p); p = a.store.Get(p).Parent {
		if s := a.data(p).scroll; s != nil && s.axis == axis && s.canMove(d) {
			return true
		}
	}
	return false
}

// stepWheel ends the stroke of a precise wheel once it has been quiet for
// wheelRelease and springs back what it stretched. It reports whether the
// stroke is still going.
func (a *App) stepWheel(h scene.Handle, s *scrollState, now time.Duration) bool {
	if now-s.wheelAt < wheelRelease {
		// Painted, so that the backend keeps its rate until the release.
		a.markNeedsPaint(h)
		return true
	}
	s.wheeling = false
	if s.pull != 0 {
		a.releaseBounce(h, s)
		// From the moment the wheel went quiet, which a frame that comes
		// late is past already.
		s.overStart = s.wheelAt + wheelRelease
	}
	return false
}

// releaseBounce springs a container that was let go past an end back.
func (a *App) releaseBounce(h scene.Handle, s *scrollState) {
	over := s.over
	s.pull = 0
	a.startBounce(h, s, over, 0)
}

// startBounce starts the spring from displacement x0 with velocity v0, both
// in offset units, towards the rest of [scrollState.rest], and enrols the
// container in the scroll tick.
func (a *App) startBounce(h scene.Handle, s *scrollState, x0, v0 float32) {
	s.bouncing = true
	s.overFrom, s.overVel, s.overStart, s.overTo = x0, v0, a.in.now, s.rest()
	s.over = x0
	a.enrolScroll(h)
}

// enrolScroll adds h to the containers the scroll tick advances, once.
func (a *App) enrolScroll(h scene.Handle) {
	a.markNeedsPaint(h)
	for _, existing := range a.in.flings {
		if existing == h {
			return
		}
	}
	a.in.flings = append(a.in.flings, h)
}

// stepBounce advances the spring: critically damped, so it returns without
// swinging through its rest, x(t) = r + (d0 + (v0 + ω·d0)·t)·e^(−ω·t) with
// d0 = x0 − r. It reports whether the container is still moving.
func (a *App) stepBounce(h scene.Handle, s *scrollState, now time.Duration) bool {
	t := (now - s.overStart).Seconds()
	const w = bounceStiffness
	r := float64(s.overTo)
	d0, v0 := float64(s.overFrom)-r, float64(s.overVel)
	e := math.Exp(-w * t)
	d := (d0 + (v0+w*d0)*t) * e
	v := (v0 - w*(v0+w*d0)*t) * e
	a.markNeedsPaint(h)
	if math.Abs(d) < 0.25 && math.Abs(v) < 20 {
		s.over, s.bouncing = s.overTo, false
		return false
	}
	s.over = float32(r + d)
	return true
}
