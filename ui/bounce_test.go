package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

var bounceMark = ui.RGB(200, 30, 30)

// firstRowY is where the first row of the list is drawn.
func firstRowY(h *gifttest.Harness) float32 {
	list := h.List()
	for i, op := range h.Ops() {
		if op.Kind == render.OpFillRect && op.Color == render.Color(ui.ResolveColor(bounceMark)) {
			return list.DeviceXform(i).TransformRect(op.Bounds).Min.Y
		}
	}
	return -1
}

// TestABouncingListIsPulledPastItsTopAndSpringsBack is the rubber band of
// iOS: pulled down at the top, the list follows with resistance; let go, it
// springs home.
func TestABouncingListIsPulledPastItsTopAndSpringsBack(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(300, 300),
		Root: func(*gift.Context) gift.View {
			rows := []gift.View{ui.Box().Frame(300, 60).Background(bounceMark)}
			for range 20 {
				rows = append(rows, ui.Box().Frame(300, 60))
			}
			return ui.VScroll(ui.VStack(rows...)).Bounce(true).Flex(1)
		},
	})
	h.Settle()
	rest := firstRowY(h)

	h.PressAt(geom.Pt(150, 100))
	for y := float32(110); y <= 200; y += 10 {
		h.MoveTo(geom.Pt(150, y))
	}
	h.Advance(16 * time.Millisecond)
	pulled := firstRowY(h)
	if !(pulled > rest+20 && pulled < rest+100) {
		t.Fatalf("pulled 100 past the top the first row is at %v (rest %v); want it to follow with resistance", pulled, rest)
	}

	h.Release()
	h.Advance(100 * time.Millisecond)
	if y := firstRowY(h); !(y < pulled && y > rest) {
		t.Errorf("100 ms after release the first row is at %v; want it on its way home from %v", y, pulled)
	}
	h.Advance(time.Second)
	if y := firstRowY(h); y != rest {
		t.Errorf("settled the first row is at %v, want %v", y, rest)
	}
	if d := h.App().Diagnostics(); d.Animating {
		t.Error("the list keeps animating after it came home")
	}
}

// bounceList is the list of TestABouncingListIsPulledPastItsTopAndSpringsBack.
func bounceList(t *testing.T) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(300, 300),
		Root: func(*gift.Context) gift.View {
			rows := []gift.View{ui.Box().Frame(300, 60).Background(bounceMark)}
			for range 20 {
				rows = append(rows, ui.Box().Frame(300, 60))
			}
			return ui.VScroll(ui.VStack(rows...)).Flex(1)
		},
	})
	h.Settle()
	return h
}

// TestATrackpadFollowsTheFingersAndStretchesPastTheTop is the rubber band of
// macOS: a trackpad scrolls ten points per unit of its delta, the distance its
// fingers travelled, and past an end the content stretches and springs back
// once the deltas stop.
func TestATrackpadFollowsTheFingersAndStretchesPastTheTop(t *testing.T) {
	h := bounceList(t)
	rest := firstRowY(h)
	at := geom.Pt(150, 150)

	h.Wheel(at, geom.Pt(0, -0.5))
	if y := firstRowY(h); y != rest-5 {
		t.Fatalf("half a unit of trackpad moved the first row to %v, want %v: ten points per unit", y, rest-5)
	}
	h.Wheel(at, geom.Pt(0, 0.5))
	// A whole number in the middle of a stroke is still the trackpad.
	h.Wheel(at, geom.Pt(0, 3))
	stretched := firstRowY(h)
	if !(stretched > rest+10 && stretched < rest+30) {
		t.Fatalf("30 points past the top the first row is at %v (rest %v); want it to follow with resistance", stretched, rest)
	}
	h.Advance(50 * time.Millisecond)
	if y := firstRowY(h); y != stretched {
		t.Errorf("50 ms after the last delta the row moved from %v to %v; the stroke may still be going", stretched, y)
	}
	h.Advance(time.Second)
	if y := firstRowY(h); y != rest {
		t.Errorf("a second after the stroke the first row is at %v, want it home at %v", y, rest)
	}
	if h.App().Diagnostics().Animating {
		t.Error("the list keeps animating after it came home")
	}
}

// TestANotchScrollsAWholeStepAndDoesNotStretch: a mouse wheel counts notches,
// each a step of DefaultScrollWheelStep, and stops at the ends, as on macOS.
// After the trackpad, it is a notch again once the trackpad has been quiet.
func TestANotchScrollsAWholeStepAndDoesNotStretch(t *testing.T) {
	h := bounceList(t)
	rest := firstRowY(h)
	at := geom.Pt(150, 150)

	h.Wheel(at, geom.Pt(0, -1))
	if y := firstRowY(h); y != rest-gift.DefaultScrollWheelStep {
		t.Fatalf("one notch moved the first row to %v, want %v", y, rest-gift.DefaultScrollWheelStep)
	}
	h.Wheel(at, geom.Pt(0, 3))
	if y := firstRowY(h); y != rest {
		t.Fatalf("three notches up from one notch down left the first row at %v, want it at the top %v and no further", y, rest)
	}

	h.Wheel(at, geom.Pt(0, -0.5))
	h.Advance(time.Second)
	h.Wheel(at, geom.Pt(0, -1))
	if y := firstRowY(h); y != rest-5-gift.DefaultScrollWheelStep {
		t.Errorf("a notch a second after the trackpad moved the first row to %v, want a whole step to %v", y, rest-5-gift.DefaultScrollWheelStep)
	}
}

// TestATrackpadAtTheEndOfAnInnerListScrollsTheOuterOne: a stroke that starts
// with the inner list at its end goes on to the page around it, as in a
// browser, instead of stretching the inner list.
func TestATrackpadAtTheEndOfAnInnerListScrollsTheOuterOne(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: geom.Sz(300, 300),
		Root: func(*gift.Context) gift.View {
			var inner []gift.View
			for range 10 {
				inner = append(inner, ui.Box().Frame(300, 60))
			}
			return ui.VScroll(
				ui.Box().Frame(300, 200),
				ui.VScroll(ui.VStack(inner...)).Frame(300, 150).Key("inner"),
				ui.Box().Frame(300, 400),
			).Flex(1).Key("outer")
		},
	})
	h.Settle()
	outer := h.Find(gifttest.ByKey("outer"))
	inner := h.Find(gifttest.ByKey("inner"))
	outer.ScrollTo(100)

	h.Wheel(inner.Center(), geom.Pt(0, 0.5))
	if got := outer.ScrollOffset(); got != 95 {
		t.Errorf("the page is at %v after half a unit up over the inner list at its top, want 95", got)
	}
	if info := inner.ScrollInfo(); info.Offset != 0 || info.Overscroll != 0 {
		t.Errorf("the inner list moved to %v and stretched by %v; the stroke was the page's", info.Offset, info.Overscroll)
	}
}

// momentum delivers the momentum of a flick as macOS does after the fingers
// lifted: a delta a tick, from first, decaying by a twentieth a tick, for n
// ticks. It returns the largest displacement of the first row from rest seen
// on the way, and its sign.
func momentum(h *gifttest.Harness, at geom.Point, first float32, n int, rest float32) float32 {
	var peak float32
	d := first
	for range n {
		h.WheelPhase(at, geom.Pt(0, d), gift.WheelPhaseMomentum)
		h.Advance(16 * time.Millisecond)
		if y := firstRowY(h) - rest; y*y > peak*peak {
			peak = y
		}
		d *= 0.95
	}
	return peak
}

// TestMomentumIntoTheTopBouncesOffItAsOnAMac: macOS reports the momentum
// after a flick as a phase of its own. Carried into the top at 3000 points a
// second, the list overshoots by about a hundredth of that, as AppKit's
// rubber band lets it, and is home well within a second; the rest of the
// momentum is dropped. Treated as a finger, the same momentum stretched the
// list past a hundred points and held it there until it ran out.
func TestMomentumIntoTheTopBouncesOffItAsOnAMac(t *testing.T) {
	h := bounceList(t)
	rest := firstRowY(h)
	at := geom.Pt(150, 150)

	h.WheelPhase(at, geom.Pt(0, -5), gift.WheelPhaseContact)
	h.Advance(16 * time.Millisecond)
	h.WheelPhase(at, geom.Pt(0, 0), gift.WheelPhaseLift)
	if y := firstRowY(h); y != rest-50 {
		t.Fatalf("five units down left the first row at %v, want %v", y, rest-50)
	}

	// 4.8 units a tick of 16 ms is 3000 points a second, towards the top.
	peak := momentum(h, at, 4.8, 15, rest)
	if !(peak > 15 && peak < 40) {
		t.Errorf("the momentum carried the first row %v past the top; want the 27 or so AppKit lets 3000 points a second overshoot by", peak)
	}
	momentum(h, at, 4.8*0.46, 45, rest)
	if y := firstRowY(h); y != rest {
		t.Errorf("after the momentum the first row is at %v, want it home at %v", y, rest)
	}
	h.Advance(time.Second)
	if h.App().Diagnostics().Animating {
		t.Error("the list keeps animating after it came home")
	}
}

// TestFingersHoldAStretchUntilTheyLift: fingers resting on the trackpad hold
// the stretch they made, as on a Mac, and the lift springs it back at once
// and drops the momentum after it, which goes neither into the stretch nor
// into a scroll.
func TestFingersHoldAStretchUntilTheyLift(t *testing.T) {
	h := bounceList(t)
	rest := firstRowY(h)
	at := geom.Pt(150, 150)

	h.WheelPhase(at, geom.Pt(0, 3), gift.WheelPhaseContact)
	stretched := firstRowY(h)
	if !(stretched > rest+10 && stretched < rest+30) {
		t.Fatalf("30 points past the top the first row is at %v (rest %v); want it to follow with resistance", stretched, rest)
	}
	h.Advance(500 * time.Millisecond)
	if y := firstRowY(h); y != stretched {
		t.Fatalf("half a second of resting fingers moved the first row from %v to %v; want the stretch held", stretched, y)
	}

	h.WheelPhase(at, geom.Pt(0, 0), gift.WheelPhaseLift)
	h.Advance(50 * time.Millisecond)
	if y := firstRowY(h); !(y < stretched && y > rest) {
		t.Errorf("50 ms after the lift the first row is at %v; want it on its way home from %v", y, stretched)
	}
	if peak := momentum(h, at, 2, 40, rest); peak > stretched-rest {
		t.Errorf("the momentum after the lift carried the first row %v past the top, further than the fingers did", peak)
	}
	if y := firstRowY(h); y != rest {
		t.Errorf("after the momentum the first row is at %v, want it home at %v", y, rest)
	}
}

// TestMomentumWithinTheBoundsScrollsAsFarAsItGoes: away from the ends the
// momentum is a scroll like the fingers before it.
func TestMomentumWithinTheBoundsScrollsAsFarAsItGoes(t *testing.T) {
	h := bounceList(t)
	rest := firstRowY(h)
	at := geom.Pt(150, 150)

	h.WheelPhase(at, geom.Pt(0, -1), gift.WheelPhaseContact)
	h.WheelPhase(at, geom.Pt(0, 0), gift.WheelPhaseLift)
	h.WheelPhase(at, geom.Pt(0, -2), gift.WheelPhaseMomentum)
	h.Advance(16 * time.Millisecond)
	h.WheelPhase(at, geom.Pt(0, -1.5), gift.WheelPhaseMomentum)
	if y := firstRowY(h); y != rest-45 {
		t.Errorf("one unit of fingers and three and a half of momentum left the first row at %v, want %v", y, rest-45)
	}
}
