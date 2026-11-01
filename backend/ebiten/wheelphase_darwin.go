//go:build darwin && !ios

package ebiten

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// The phases of a trackpad gesture, which Ebitengine drops.
//
// macOS tells a scroll event of fingers on the trackpad from one of the
// momentum it adds after they lifted: NSEvent.phase runs began, changed …
// ended while they touch, and NSEvent.momentumPhase began, changed … ended
// after. Ebitengine's Cocoa port, like GLFW, whose port it is, reads
// scrollingDeltaX and scrollingDeltaY, multiplies precise ones by a tenth and
// reports the sum, so the momentum reaches gift as a finger that goes on
// scrolling for a second after it lifted, and the lift itself, an event with
// no delta, not at all. See [gift.App.PointerWheelPhase] for what that did to
// a list flicked into its end.
//
// So the backend reads the scroll events itself, with a local event monitor
// of AppKit, which sees every event the application dispatches before the
// window does: [NSEvent addLocalMonitorForEventsMatchingMask:handler:]. The
// handler runs on the main thread, in Ebitengine's event loop, and returns
// the event unchanged, so Ebitengine still gets it; the backend then takes
// its deltas from here and not from [eb.Wheel], so that the delta and its
// phase come from one event and not from two readings a tick apart.

// NSEventMaskScrollWheel is 1 << NSEventTypeScrollWheel, and the phases are
// the bits of NSEventPhase. Numbers in an ABI, from NSEvent.h.
const (
	nsEventMaskScrollWheel = 1 << 22

	nsPhaseBegan     = 1 << 0
	nsPhaseChanged   = 1 << 2
	nsPhaseEnded     = 1 << 3
	nsPhaseCancelled = 1 << 4
)

// macWheel is the monitor and the events it recorded since the last drain.
type macWheel struct {
	mu  sync.Mutex
	evs []wheelSample

	phase, momentumPhase, deltaX, deltaY, precise objc.SEL
}

// openWheelPhases installs the monitor and returns the function that drains
// it, or nil where it could not be installed; the bridge then reads
// [eb.Wheel] as before.
//
// It is called from [Run], on the main thread, which is the one AppKit wants
// to be called on and the one Ebitengine runs Run on: it locks the main
// goroutine to it.
func openWheelPhases() func([]wheelSample) []wheelSample {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		return nil
	}
	cls := objc.GetClass("NSEvent")
	if cls == 0 {
		return nil
	}
	m := &macWheel{
		evs:           make([]wheelSample, 0, 8),
		phase:         objc.RegisterName("phase"),
		momentumPhase: objc.RegisterName("momentumPhase"),
		deltaX:        objc.RegisterName("scrollingDeltaX"),
		deltaY:        objc.RegisterName("scrollingDeltaY"),
		precise:       objc.RegisterName("hasPreciseScrollingDeltas"),
	}
	// Never released: the monitor lives as long as the process, and AppKit
	// keeps the block it was given.
	block := objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
		m.record(ev)
		return ev
	})
	mon := objc.ID(cls).Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"),
		uint64(nsEventMaskScrollWheel), block)
	if mon == 0 {
		return nil
	}
	mon.Send(objc.RegisterName("retain"))
	return m.drain
}

// record files one scroll event: its delta, in the units of [eb.Wheel], under
// the phase it belongs to, merged with the event before it where that has the
// same phase.
func (m *macWheel) record(ev objc.ID) {
	phase := objc.Send[uint64](ev, m.phase)
	momentum := objc.Send[uint64](ev, m.momentumPhase)
	d := geom.Pt(float32(objc.Send[float64](ev, m.deltaX)), float32(objc.Send[float64](ev, m.deltaY)))
	if objc.Send[bool](ev, m.precise) {
		// What GLFW makes of a precise delta, so that a unit is still ten
		// points; see [gift.Event.Precise].
		d = geom.Pt(d.X*0.1, d.Y*0.1)
	}

	var p gift.WheelPhase
	switch {
	case momentum&(nsPhaseBegan|nsPhaseChanged) != 0:
		p = gift.WheelPhaseMomentum
	case momentum != 0:
		// The end of the momentum, with no delta: the stroke ends when the
		// deltas stop, as it always has.
		return
	case phase&(nsPhaseBegan|nsPhaseChanged) != 0:
		p = gift.WheelPhaseContact
	case phase&(nsPhaseEnded|nsPhaseCancelled) != 0:
		p, d = gift.WheelPhaseLift, geom.Point{}
	case phase != 0:
		// May begin and stationary: fingers resting, nothing to report.
		return
	default:
		// No phase at all: a mouse wheel.
		p = gift.WheelPhaseUnknown
	}
	if d == (geom.Point{}) && p != gift.WheelPhaseLift && p != gift.WheelPhaseContact {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if n := len(m.evs); n > 0 && m.evs[n-1].phase == p {
		m.evs[n-1].delta = m.evs[n-1].delta.Add(d)
		return
	}
	m.evs = append(m.evs, wheelSample{phase: p, delta: d})
}

// drain appends the events recorded since the last call to dst, in the order
// they came, and forgets them.
func (m *macWheel) drain(dst []wheelSample) []wheelSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	dst = append(dst, m.evs...)
	m.evs = m.evs[:0]
	return dst
}
