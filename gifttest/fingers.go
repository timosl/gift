package gifttest

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// FrameInterval is the clock step of the gestures below that take time: one
// frame at 60 Hz.
const FrameInterval = time.Second / 60

// FingerDown puts the finger with the given identifier down at the device
// space point p and leaves it there. With several fingers this is how a test
// writes a multitouch gesture one event at a time; a further finger reaches
// only a view that declared [gift.Element.MultiTouch].
//
// The identifiers are the test's own. They must differ from each other while
// the fingers are down and are independent of the one [Harness.TapAt] uses.
func (h *Harness) FingerDown(id int, p geom.Point) {
	h.t.Helper()
	h.beginInput()
	h.app.PointerDown(fingerID(id), gift.PointerTouch, p)
	h.Settle()
}

// FingerMove moves the finger id to p without advancing the clock.
func (h *Harness) FingerMove(id int, p geom.Point) {
	h.t.Helper()
	h.beginInput()
	h.app.PointerMove(fingerID(id), gift.PointerTouch, p)
	h.Settle()
}

// FingerUp lifts the finger id at p.
func (h *Harness) FingerUp(id int, p geom.Point) {
	h.t.Helper()
	h.beginInput()
	h.app.PointerUp(fingerID(id), gift.PointerTouch, p)
	h.Settle()
}

// Fling is a finger that goes down at from, travels to to in d at an even
// speed with one move per [FrameInterval], and lifts there. The clock
// advances with every move, so a recogniser measures the velocity a real hand
// would have: (to − from)/d.
func (h *Harness) Fling(from, to geom.Point, d time.Duration) {
	h.t.Helper()
	steps := max(2, int(d/FrameInterval))
	h.FingerDown(fingerBase, from)
	var p geom.Point
	for i := 1; i <= steps; i++ {
		p = lerp(from, to, float32(i)/float32(steps))
		h.now += d / time.Duration(steps)
		h.beginInput()
		h.app.PointerMove(fingerID(fingerBase), gift.PointerTouch, p)
		h.Settle()
	}
	h.FingerUp(fingerBase, p)
}

// Pinch puts two fingers down on a horizontal line through centre, spread
// fromSpread apart, moves them to toSpread apart in d with one move per
// [FrameInterval], and lifts them. A spread that grows is a zoom in.
func (h *Harness) Pinch(centre geom.Point, fromSpread, toSpread float32, d time.Duration) {
	h.t.Helper()
	at := func(spread float32) (geom.Point, geom.Point) {
		return geom.Pt(centre.X-spread/2, centre.Y), geom.Pt(centre.X+spread/2, centre.Y)
	}
	a, b := at(fromSpread)
	h.FingerDown(fingerBase, a)
	h.FingerDown(fingerBase+1, b)
	steps := max(2, int(d/FrameInterval))
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		a, b = at(fromSpread + (toSpread-fromSpread)*f)
		h.now += d / time.Duration(steps)
		h.beginInput()
		h.app.PointerMove(fingerID(fingerBase), gift.PointerTouch, a)
		h.app.PointerMove(fingerID(fingerBase+1), gift.PointerTouch, b)
		h.Settle()
	}
	h.FingerUp(fingerBase+1, b)
	h.FingerUp(fingerBase, a)
}

// fingerBase keeps the identifiers of these helpers away from touchID, which
// the single finger verbs use, so a test may hold one of each.
const fingerBase = 100

func fingerID(id int) gift.PointerID {
	return gift.PointerID(int64(id)) + gift.PointerID(math.MaxInt32)
}
