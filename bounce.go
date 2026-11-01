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
// stretch it made springs back. Where the platform reports no phases of a
// trackpad gesture, as GLFW does not, the end of the stroke, and of any
// momentum the platform adds to it, is the moment the deltas stop; they come
// with every frame until then.
const wheelRelease = 120 * time.Millisecond

// wheelHold is how long a stretch is held for fingers that rest on the
// trackpad without moving, where the platform reports them; see
// [App.stepWheel]. AppKit holds it until they lift; this is the bound for a
// lift that never arrives, a gesture whose end went to another window.
const wheelHold = 2 * time.Second

// The rubber band of AppKit, for a trackpad whose platform reports the stages
// of its gesture; see [App.PointerWheelPhase]. WebKit's
// ScrollingEffectsController reproduces it for its own scrolling on the Mac
// with these constants and the curve of [App.stepBounce]:
//
//	x(t) = (x0 + a·v0·t)·e^(−k·t),  k = stiffness/period = 20/1.6,  a = 0.31
//
// Content the momentum carries into an end at v0 overshoots by a·v0/(k·e),
// about a hundredth of its speed — 36 points at 4000 points a second — and is
// home in about half a second; a stretch let go of, v0 = 0, decays in the
// same time without swinging. That is a third of what the spring of a touch
// fling, [bounceStiffness], lets the same speed overshoot by, which is the
// difference between a scroll view on the Mac and on an iPhone.
const (
	macBounceRate      = 20.0 / 1.6
	macBounceAmplitude = 0.31
)

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
//
// # Momentum, where the platform reports it
//
// A delta of [WheelPhaseMomentum] is not a finger and does not stretch. It
// scrolls within the bounds, and where it runs into an end the content
// bounces off it with the speed the momentum had, on AppKit's curve, and the
// rest of the momentum is dropped, as AppKit drops it; so is the momentum
// after fingers that lifted from a stretch. Dropped means taken and ignored,
// so that it does not scroll the page around either.
//
// Before the phases were read, the momentum stretched the content like a
// finger that went on pulling for as long as it lasted: a list flicked into
// its end was dragged a good third of its height past it and held there for
// a second, until the momentum ran out.
func (a *App) wheelBounce(h scene.Handle, s *scrollState, d float64, phase WheelPhase) bool {
	if phase == WheelPhaseMomentum && s.wheeling && s.dropMomentum {
		s.wheelAt = a.in.now
		return true
	}
	if !s.wheeling && s.pull == 0 && !s.canMove(d) {
		if !(s.maxOffset() > 0) || a.outerCanScroll(h, s.axis, d) {
			return false
		}
	}
	s.flinging, s.vel = false, 0
	s.phased = phase != WheelPhaseUnknown
	switch phase {
	case WheelPhaseMomentum:
		a.momentumBounce(h, s, d)
	default:
		s.dropMomentum = false
		a.dragBounce(h, s, d)
	}
	s.wheeling, s.wheelAt = true, a.in.now
	a.enrolScroll(h)
	return true
}

// momentumBounce scrolls a container by d offset units of the momentum after
// a trackpad gesture; see [App.wheelBounce].
func (a *App) momentumBounce(h scene.Handle, s *scrollState, d float64) {
	if s.pull != 0 || s.bouncing {
		// Stretched still, or springing back: fingers lifted from a
		// stretch, and the momentum that follows them is not theirs to
		// carry on with.
		if s.pull != 0 {
			a.releaseBounce(h, s, true)
		}
		s.dropMomentum = true
		return
	}
	// The speed of the momentum, over the time since the delta before it.
	// Two deltas in one update share its time, and the second keeps the
	// speed the first measured.
	if dt := (a.in.now - s.wheelAt).Seconds(); s.wheeling && dt > 0 && dt < maxFlingStep.Seconds() {
		s.wheelVel = float32(d / dt)
	} else if !s.wheeling || s.wheelVel == 0 {
		s.wheelVel = float32(d * 60)
	}
	s.wheelVel = max(-s.cfg.MaxVelocity, min(s.cfg.MaxVelocity, s.wheelVel))

	target := s.off + d
	clamped := s.clamp(target)
	a.setScroll(h, s, clamped)
	if target != clamped {
		a.startBounce(h, s, 0, s.wheelVel)
		s.macBounce = true
		s.dropMomentum = true
	}
}

// liftWheel springs back what fingers on a trackpad stretched, the moment
// they lift, and has the momentum that follows them dropped; see
// [App.PointerWheelPhase].
func (a *App) liftWheel() {
	for _, h := range a.in.flings {
		if !a.store.Valid(h) {
			continue
		}
		if s := a.data(h).scroll; s != nil && s.wheeling && s.pull != 0 {
			a.releaseBounce(h, s, true)
			s.dropMomentum = true
		}
	}
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
//
// Fingers that rest on the trackpad, where the platform reports them, hold
// the stretch they made until they lift, as on the Mac, and not only until
// they have been still for wheelRelease; [App.liftWheel] then lets it go.
func (a *App) stepWheel(h scene.Handle, s *scrollState, now time.Duration) bool {
	quiet := wheelRelease
	if s.phased && s.pull != 0 && a.in.wheelContact {
		quiet = wheelHold
	}
	if now-s.wheelAt < quiet {
		// Painted, so that the backend keeps its rate until the release.
		a.markNeedsPaint(h)
		return true
	}
	s.wheeling, s.dropMomentum = false, false
	if s.pull != 0 {
		a.releaseBounce(h, s, s.phased)
		// From the moment the wheel went quiet, which a frame that comes
		// late is past already.
		s.overStart = s.wheelAt + quiet
	}
	return false
}

// releaseBounce springs a container that was let go past an end back, on
// AppKit's curve for a trackpad whose phases are known and on the spring of a
// finger otherwise.
func (a *App) releaseBounce(h scene.Handle, s *scrollState, mac bool) {
	over := s.over
	s.pull = 0
	a.startBounce(h, s, over, 0)
	s.macBounce = mac
}

// startBounce starts the spring from displacement x0 with velocity v0, both
// in offset units, towards the rest of [scrollState.rest], and enrols the
// container in the scroll tick.
//
// It is the spring of a finger, [bounceStiffness]; a caller that wants
// AppKit's curve sets [scrollState.macBounce] after it.
func (a *App) startBounce(h scene.Handle, s *scrollState, x0, v0 float32) {
	s.bouncing, s.macBounce = true, false
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
// d0 = x0 − r. AppKit's curve, [macBounceRate], is the same without the ω·d0
// term and with the speed scaled by [macBounceAmplitude]. It reports whether
// the container is still moving.
func (a *App) stepBounce(h scene.Handle, s *scrollState, now time.Duration) bool {
	t := (now - s.overStart).Seconds()
	r := float64(s.overTo)
	d0, v0 := float64(s.overFrom)-r, float64(s.overVel)
	var d, v float64
	if s.macBounce {
		const k, amp = macBounceRate, macBounceAmplitude
		e := math.Exp(-k * t)
		d = (d0 + amp*v0*t) * e
		v = (amp*v0 - k*(d0+amp*v0*t)) * e
	} else {
		const w = bounceStiffness
		e := math.Exp(-w * t)
		d = (d0 + (v0+w*d0)*t) * e
		v = (v0 - w*(v0+w*d0)*t) * e
	}
	a.markNeedsPaint(h)
	if math.Abs(d) < 0.25 && math.Abs(v) < 20 {
		s.over, s.bouncing = s.overTo, false
		return false
	}
	s.over = float32(r + d)
	return true
}
