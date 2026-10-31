package gesture

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui/motion"
)

// The thresholds of the recognisers. They are iOS's, as far as Apple states
// them, and SwiftUI's defaults where it does not.
var (
	// Slop is how far a finger may travel and still be a tap or a long press,
	// and how far a drag has to travel before it begins: the "hysteresis"
	// that WWDC 2018's "Designing Fluid Interfaces" gives as usually ten
	// points, and SwiftUI's default minimumDistance.
	Slop float32 = 10
	// MultiTapInterval is the longest pause between the taps of a double or
	// triple tap.
	MultiTapInterval = 300 * time.Millisecond
	// MultiTapDistance is how far apart the taps of a multiple tap may land.
	MultiTapDistance float32 = 40
	// LongPressDuration is SwiftUI's default minimumDuration.
	LongPressDuration = 500 * time.Millisecond
)

// maxFingers is how many fingers one set follows. A sixth finger on one view is
// counted by nobody and changes nothing.
const maxFingers = 5

// maxGestures bounds a set; see [Handle].
const maxGestures = 12

// Phase is where a continuous gesture is in its life.
type Phase uint8

const (
	// Began is the first change of a gesture: the moment it was recognised.
	Began Phase = iota + 1
	// Changed is every change after the first.
	Changed
	// Ended is the release.
	Ended
)

// Common is what every value carries.
type Common struct {
	// Time is the event time, on the clock of [gift.App.BeginInput].
	Time time.Duration
	// Bounds is the device space rectangle of the node that recognised the
	// gesture, the space every position of a value is in; see
	// [gift.EventContext.DeviceBounds].
	Bounds geom.Rect
	// Device says whether a mouse or a finger made the gesture.
	Device gift.PointerKind
	// Mods are the modifier keys that were held.
	Mods gift.Mods
}

// Local converts a device space position into the recognising node's own
// coordinates, with the origin at its top left corner.
func (c Common) Local(p geom.Point) geom.Point {
	return geom.Pt(p.X-c.Bounds.Min.X, p.Y-c.Bounds.Min.Y)
}

// Gesture is one recogniser declaration. It is a plain value, configured with
// its methods and rebuilt with the view it belongs to; the recognition state
// lives in the node. Obtain one from [Tap], [LongPress], [Drag], [Magnify],
// [Rotate], [Hover] or [Wheel].
type Gesture interface {
	kind() kind
}

type kind uint8

const (
	kindTap kind = iota + 1
	kindLongPress
	kindDrag
	kindMagnify
	kindRotate
	kindHover
	kindWheel
)

// --- tap ---------------------------------------------------------------------

// TapValue is a recognised tap.
type TapValue struct {
	Common
	// Location is where the last tap landed, in device space.
	Location geom.Point
	// Count is the number of taps, the gesture's own count.
	Count int
}

// TapGesture recognises one or more taps of one finger or of the mouse.
type TapGesture struct {
	count int
	fn    func(TapValue)
}

// Tap returns a single tap recogniser.
//
// A tap is a press that comes up within [Slop] of where it went down. Several
// taps of different counts on one view do not all fire: a single tap waits
// until a double tap in the same set has failed — the second tap did not come
// within [MultiTapInterval] — and only then fires. That is UIKit's
// require-to-fail and the reason a single tap on a view that also zooms on a
// double tap answers a moment late.
func Tap() TapGesture { return TapGesture{count: 1} }

// Count sets how many taps the gesture needs.
func (g TapGesture) Count(n int) TapGesture { g.count = max(1, n); return g }

// OnEnded sets the function called when the taps are complete.
func (g TapGesture) OnEnded(fn func(TapValue)) TapGesture { g.fn = fn; return g }

func (TapGesture) kind() kind { return kindTap }

// --- long press --------------------------------------------------------------

// LongPressValue is a recognised long press.
type LongPressValue struct {
	Common
	Location geom.Point
}

// LongPressGesture recognises a finger or a mouse button held still.
type LongPressGesture struct {
	d  time.Duration
	fn func(LongPressValue)
}

// LongPress returns a long press recogniser with [LongPressDuration]. When it
// fires, no tap of the same set fires for that press.
func LongPress() LongPressGesture { return LongPressGesture{d: LongPressDuration} }

// MinimumDuration sets how long the press must last.
func (g LongPressGesture) MinimumDuration(d time.Duration) LongPressGesture { g.d = d; return g }

// OnEnded sets the function called when the press has lasted long enough. It
// is called while the finger is still down.
func (g LongPressGesture) OnEnded(fn func(LongPressValue)) LongPressGesture { g.fn = fn; return g }

func (LongPressGesture) kind() kind { return kindLongPress }

// --- drag --------------------------------------------------------------------

// Axis restricts the direction a [DragGesture] may begin in.
type Axis uint8

const (
	// Free begins in any direction.
	Free Axis = iota
	// Horizontal begins only when the movement at the [Slop] is mostly
	// horizontal, and fails otherwise, so that the event goes on to an
	// ancestor — a vertical scroll container, say.
	Horizontal
	// Vertical is the counterpart of Horizontal.
	Vertical
)

// DragValue is the state of a drag. Positions are in device space and
// velocities in device units per second.
type DragValue struct {
	Common
	Phase Phase
	// Location is the centroid of the fingers on the view now, and
	// StartLocation where the drag started.
	Location, StartLocation geom.Point
	// Translation is the distance the fingers travelled since the start.
	// It is continuous when a finger is added or lifted: the centroid then
	// jumps, the translation does not.
	Translation geom.Point
	// Velocity is the current velocity of the translation.
	Velocity geom.Point
	// PredictedEndTranslation is where a release now would come to rest
	// under [motion.DecelerationNormal]; see [motion.Project].
	PredictedEndTranslation geom.Point
	// Fingers is how many fingers are down; the mouse counts as one.
	Fingers int
}

// DragGesture recognises a pointer that moves.
type DragGesture struct {
	min       float32
	axis      Axis
	mouse     bool
	onChanged func(DragValue)
	onEnded   func(DragValue)
	onCancel  func()
	should    func(DragValue) bool
}

// Drag returns a drag recogniser that begins after [Slop] in any direction,
// for fingers and the mouse.
//
// It follows the centroid of every finger on the view, as UIKit's pan
// recogniser does, so a second finger joins a drag rather than ending it. The
// drag ends when the last finger lifts.
func Drag() DragGesture { return DragGesture{min: Slop, mouse: true} }

// MinimumDistance sets how far the pointer travels before the drag begins.
// Zero begins it on the press.
func (g DragGesture) MinimumDistance(d float32) DragGesture { g.min = max(0, d); return g }

// Axis restricts the direction the drag may begin in; see [Horizontal].
func (g DragGesture) Axis(a Axis) DragGesture { g.axis = a; return g }

// Mouse sets whether the mouse may drag. It is true by default; a view that
// leaves dragging to fingers and scrolls with the wheel sets it false.
func (g DragGesture) Mouse(v bool) DragGesture { g.mouse = v; return g }

// ShouldBegin sets a predicate that is asked when the drag is about to begin,
// with the value it would begin with. False makes the drag fail for this
// press, and the events go on to the ancestors: UIKit's
// gestureRecognizerShouldBegin. A picture that pans only while it is zoomed,
// inside a list that scrolls, declines here while it is not.
func (g DragGesture) ShouldBegin(fn func(DragValue) bool) DragGesture { g.should = fn; return g }

// OnChanged sets the function called when the drag begins and on every move.
func (g DragGesture) OnChanged(fn func(DragValue)) DragGesture { g.onChanged = fn; return g }

// OnEnded sets the function called when the last finger lifts.
func (g DragGesture) OnEnded(fn func(DragValue)) DragGesture { g.onEnded = fn; return g }

// OnCancelled sets the function called when a drag that began is taken away:
// by an ancestor that stole the pointer, or by the platform.
func (g DragGesture) OnCancelled(fn func()) DragGesture { g.onCancel = fn; return g }

func (DragGesture) kind() kind { return kindDrag }

// --- magnify and rotate ------------------------------------------------------

// MagnifyValue is the state of a pinch.
type MagnifyValue struct {
	Common
	Phase Phase
	// Magnification is the factor by which the distance between the fingers
	// changed since the pinch began. It is continuous when the fingers that
	// make up the pinch change.
	Magnification float64
	// Velocity is the rate of change of Magnification per second.
	Velocity float64
	// StartAnchor is the centroid of the fingers when the pinch began, and
	// Location the centroid now.
	StartAnchor, Location geom.Point
}

// MagnifyGesture recognises two fingers moving apart or together.
type MagnifyGesture struct {
	onChanged func(MagnifyValue)
	onEnded   func(MagnifyValue)
	onCancel  func()
}

// Magnify returns a pinch recogniser. It begins when a second finger is down
// and either finger moves, and ends when fewer than two fingers remain. A
// view that carries it accepts further fingers; see [gift.Element.MultiTouch].
func Magnify() MagnifyGesture { return MagnifyGesture{} }

// OnChanged sets the function called when the pinch begins and on every move.
func (g MagnifyGesture) OnChanged(fn func(MagnifyValue)) MagnifyGesture { g.onChanged = fn; return g }

// OnEnded sets the function called when the pinch ends.
func (g MagnifyGesture) OnEnded(fn func(MagnifyValue)) MagnifyGesture { g.onEnded = fn; return g }

// OnCancelled sets the function called when a pinch that began is taken away.
func (g MagnifyGesture) OnCancelled(fn func()) MagnifyGesture { g.onCancel = fn; return g }

func (MagnifyGesture) kind() kind { return kindMagnify }

// RotateValue is the state of a two finger rotation.
type RotateValue struct {
	Common
	Phase Phase
	// Rotation is the angle in radians the line between the fingers turned
	// since the gesture began, clockwise on the screen positive.
	Rotation float64
	// Velocity is its rate per second.
	Velocity float64
	// StartAnchor and Location are as in [MagnifyValue].
	StartAnchor, Location geom.Point
}

// RotateGesture recognises two fingers turning.
type RotateGesture struct {
	onChanged func(RotateValue)
	onEnded   func(RotateValue)
	onCancel  func()
}

// Rotate returns a rotation recogniser. It has the life of [Magnify] and runs
// alongside it.
func Rotate() RotateGesture { return RotateGesture{} }

// OnChanged sets the function called when the rotation begins and on every
// move.
func (g RotateGesture) OnChanged(fn func(RotateValue)) RotateGesture { g.onChanged = fn; return g }

// OnEnded sets the function called when the rotation ends.
func (g RotateGesture) OnEnded(fn func(RotateValue)) RotateGesture { g.onEnded = fn; return g }

// OnCancelled sets the function called when a rotation that began is taken
// away.
func (g RotateGesture) OnCancelled(fn func()) RotateGesture { g.onCancel = fn; return g }

func (RotateGesture) kind() kind { return kindRotate }

// --- hover and wheel ---------------------------------------------------------

// HoverValue is the position of a mouse over the view.
type HoverValue struct {
	Common
	Location geom.Point
	// Inside reports whether the mouse is over the view; the last value of
	// a hover has it false.
	Inside bool
}

// HoverGesture follows a mouse that moves over the view without a button
// held. A finger never hovers.
type HoverGesture struct{ fn func(HoverValue) }

// Hover returns a hover recogniser.
func Hover() HoverGesture { return HoverGesture{} }

// OnChanged sets the function called on every movement, and once with Inside
// false when the mouse leaves.
func (g HoverGesture) OnChanged(fn func(HoverValue)) HoverGesture { g.fn = fn; return g }

func (HoverGesture) kind() kind { return kindHover }

// WheelValue is one scroll of a wheel or of two fingers on a trackpad.
type WheelValue struct {
	Common
	Location geom.Point
	// Delta is the scroll amount as [gift.Event.Delta] defines it: positive
	// Y is the wheel pushed away from the user.
	Delta geom.Point
	// Precise reports a trackpad rather than a notched wheel; see
	// [gift.Event.Precise]. A map pans under it and zooms under a notch.
	Precise bool
}

// WheelGesture receives the wheel.
type WheelGesture struct{ fn func(WheelValue) bool }

// Wheel returns a wheel recogniser.
func Wheel() WheelGesture { return WheelGesture{} }

// OnChanged sets the function called for every wheel event. It reports
// whether it used the event; one it did not use goes on to the ancestors,
// which is how a view that zooms only with a modifier lets a scroll container
// around it scroll.
func (g WheelGesture) OnChanged(fn func(WheelValue) bool) WheelGesture { g.fn = fn; return g }

func (WheelGesture) kind() kind { return kindWheel }

// NeedsMultiTouch reports whether a set contains a recogniser that needs more
// than one finger. A node that feeds the set to [Handle] sets
// [gift.Element.MultiTouch] to it.
func NeedsMultiTouch(gs []Gesture) bool {
	for _, g := range gs {
		if k := g.kind(); k == kindMagnify || k == kindRotate {
			return true
		}
	}
	return false
}

// --- the arena ---------------------------------------------------------------

type finger struct {
	id    gift.PointerID
	pos   geom.Point
	mouse bool
}

// state is the recognition state of one gesture of a set.
type state struct {
	k kind
	// failed says this gesture has given up on the current press sequence;
	// began says a continuous gesture is running.
	failed, began bool

	// tap and long press
	taps     int
	downAt   time.Duration
	downPos  geom.Point
	lastUp   time.Duration
	lastPos  geom.Point
	waiting  bool // a recognised tap that waits for a longer one to fail
	deadline time.Duration

	// drag
	from geom.Point // translation when it became possible

	// magnify and rotate
	pair        [2]gift.PointerID
	d0, a0      float64 // distance and angle of the pair when it formed
	base, cur   float64 // value before the pair formed, and now
	anchor      geom.Point
	samples     tracker
	hoverInside bool
}

// arena is everything one node remembers about its gestures. It lives in the
// node's retained state; see [gift.EventContext.Retained].
type arena struct {
	shape   [maxGestures]kind
	nshape  int
	st      [maxGestures]state
	fingers [maxFingers]finger
	n       int

	// centroid is the centroid of the fingers, origin the centroid when the
	// sequence began, and travel the continuous translation since then:
	// it moves with the centroid while the set of fingers stays the same,
	// and not when a finger arrives or lifts.
	centroid, origin, travel geom.Point
	track                    tracker
	// claimed says a long press fired in this sequence, so no tap may.
	claimed bool
	device  gift.PointerKind
}

// Handle feeds one event to a set of gestures on the node receiving it and
// reports whether the event was used.
//
// It is the engine under [On], for a node that has an interactor of its own
// and wants gestures in it:
//
//	func (n *myNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
//		return gesture.Handle(ctx, e, n.gestures) || n.keys(ctx, e)
//	}
//
// The slice is the node's, built once per build; passing the same gestures in
// the same order on every event is what lets the state of each be found
// again. A set whose kinds change starts over.
//
// # How the recognisers of a set get along
//
// They run side by side and see every event, as SwiftUI's simultaneous
// gestures do. Three rules keep them from contradicting each other: a tap
// waits for a tap of a higher count to fail, a long press that fired takes
// the press away from every tap, and a tap fails once its finger has moved
// [Slop], which is exactly the distance at which a drag begins. Directional
// drags decide at the slop and fail in the other direction, which makes a
// horizontal and a vertical drag on one view exclusive without further
// arrangement.
//
// A continuous gesture that begins takes the pointer from whatever node
// captured it — a button under the finger stops being pressed — with
// [gift.EventContext.StealPointer]. Events that no recogniser wants are
// reported as unused and bubble on to the ancestors.
func Handle(ctx *gift.EventContext, e gift.Event, gs []Gesture) bool {
	a := ctx.Retained[arena]()
	a.conform(gs)
	c := Common{Time: e.Time, Bounds: ctx.DeviceBounds(), Device: e.Device, Mods: e.Mods}
	switch e.Kind {
	case gift.EventPointerDown:
		return a.down(ctx, e, gs, c)
	case gift.EventPointerMove:
		if a.find(e.Pointer) < 0 {
			return a.hover(e, gs, c, true)
		}
		return a.move(ctx, e, gs, c)
	case gift.EventPointerUp:
		return a.up(ctx, e, gs, c)
	case gift.EventPointerCancel:
		return a.cancel(e, gs)
	case gift.EventPointerEnter:
		return a.hover(e, gs, c, true)
	case gift.EventPointerLeave:
		return a.hover(e, gs, c, c.Bounds.Contains(e.Pos))
	case gift.EventWheel:
		for _, g := range gs {
			if w, ok := g.(WheelGesture); ok && w.fn != nil {
				if w.fn(WheelValue{Common: c, Location: e.Pos, Delta: e.Delta, Precise: e.Precise}) {
					return true
				}
			}
		}
		return false
	case gift.EventTimer:
		a.timer(ctx, e.Time, gs, c)
		return true
	}
	return false
}

// conform resets the per gesture state when the set changed its shape.
func (a *arena) conform(gs []Gesture) {
	n := min(len(gs), maxGestures)
	same := n == a.nshape
	for i := range n {
		if same && a.shape[i] != gs[i].kind() {
			same = false
		}
	}
	if same {
		return
	}
	a.nshape = n
	for i := range n {
		a.shape[i] = gs[i].kind()
		a.st[i] = state{k: a.shape[i]}
	}
}

func (a *arena) find(id gift.PointerID) int {
	for i := range a.n {
		if a.fingers[i].id == id {
			return i
		}
	}
	return -1
}

// rebase recomputes the centroid after the set of fingers changed, without
// moving the travel.
func (a *arena) rebase() {
	if a.n == 0 {
		return
	}
	var s geom.Point
	for i := range a.n {
		s = s.Add(a.fingers[i].pos)
	}
	a.centroid = geom.Pt(s.X/float32(a.n), s.Y/float32(a.n))
}

func (a *arena) down(ctx *gift.EventContext, e gift.Event, gs []Gesture, c Common) bool {
	// Forget fingers that went away while nothing was delivered here.
	for i := 0; i < a.n; {
		if !ctx.PointerIsDown(a.fingers[i].id) {
			a.remove(i)
			continue
		}
		i++
	}
	if i := a.find(e.Pointer); i >= 0 {
		a.remove(i)
	}
	if a.n >= maxFingers {
		return false
	}
	fresh := a.n == 0
	a.fingers[a.n] = finger{id: e.Pointer, pos: e.Pos, mouse: e.Device == gift.PointerMouse}
	a.n++
	a.rebase()
	if fresh {
		a.origin, a.travel, a.claimed, a.device = a.centroid, geom.Point{}, false, e.Device
		a.track.reset()
		a.track.add(e.Time, a.travel)
	}
	used := false
	for i, g := range gs[:a.nshape] {
		s := &a.st[i]
		if fresh {
			s.failed, s.began, s.d0 = false, false, 0
		}
		switch g := g.(type) {
		case TapGesture:
			if a.n > 1 {
				s.failed = true
				s.waiting = false
				continue
			}
			if s.waiting {
				// The next tap of a longer gesture is on its way; the
				// shorter one that waited for it to fail is not going to
				// fire.
				s.waiting = false
				s.taps = 0
			}
			if s.taps > 0 && (e.Time-s.lastUp > MultiTapInterval || dist(e.Pos, s.lastPos) > MultiTapDistance) {
				s.taps = 0
			}
			s.downAt, s.downPos = e.Time, e.Pos
			used = true
		case LongPressGesture:
			if a.n > 1 {
				s.failed = true
				continue
			}
			s.downAt, s.downPos = e.Time, e.Pos
			s.deadline = e.Time + g.d
			used = true
		case DragGesture:
			if !g.mouse && e.Device == gift.PointerMouse {
				s.failed = true
				continue
			}
			if fresh {
				s.from = a.travel
				if g.min == 0 && !a.beginDrag(ctx, s, g, c, e) {
					continue
				}
			}
			used = used || !s.failed
		case MagnifyGesture, RotateGesture:
			if a.n >= 2 {
				a.formPair(s)
			}
			used = true
		}
	}
	a.arm(ctx, e.Time)
	return used
}

func (a *arena) move(ctx *gift.EventContext, e gift.Event, gs []Gesture, c Common) bool {
	i := a.find(e.Pointer)
	a.fingers[i].pos = e.Pos
	prev := a.centroid
	a.rebase()
	a.travel = a.travel.Add(a.centroid.Sub(prev))
	a.track.add(e.Time, a.travel)
	used := false
	for k, g := range gs[:a.nshape] {
		s := &a.st[k]
		if s.failed {
			continue
		}
		switch g := g.(type) {
		case TapGesture:
			if dist(e.Pos, s.downPos) > Slop {
				s.failed, s.taps = true, 0
				continue
			}
			used = true
		case LongPressGesture:
			if dist(e.Pos, s.downPos) > Slop {
				s.failed = true
				continue
			}
			used = true
		case DragGesture:
			if !s.began {
				d := a.travel.Sub(s.from)
				if length(d) < g.min {
					used = true
					continue
				}
				ax, ay := abs32(d.X), abs32(d.Y)
				if (g.axis == Horizontal && ax < ay) || (g.axis == Vertical && ay < ax) {
					s.failed = true
					continue
				}
				if !a.beginDrag(ctx, s, g, c, e) {
					continue
				}
			} else {
				ctx.StealPointer()
				if g.onChanged != nil {
					v := a.dragValue(s, c, e.Time)
					v.Phase = Changed
					g.onChanged(v)
				}
			}
			used = true
		case MagnifyGesture:
			if a.n < 2 {
				continue
			}
			a.updatePair(s, false)
			if !s.began {
				if math.Abs(a.pairDist()-s.d0) < float64(Slop)/2 && length(a.centroid.Sub(s.anchor)) < Slop/2 {
					used = true
					continue
				}
				s.began = true
				ctx.StealPointer()
				if g.onChanged != nil {
					v := a.magnifyValue(s, c, e.Time)
					v.Phase = Began
					g.onChanged(v)
				}
			} else {
				ctx.StealPointer()
				if g.onChanged != nil {
					v := a.magnifyValue(s, c, e.Time)
					v.Phase = Changed
					g.onChanged(v)
				}
			}
			used = true
		case RotateGesture:
			if a.n < 2 {
				continue
			}
			a.updatePair(s, true)
			if !s.began {
				if math.Abs(s.cur-s.base) < 2*math.Pi/180 {
					used = true
					continue
				}
				s.began = true
				ctx.StealPointer()
				if g.onChanged != nil {
					v := a.rotateValue(s, c, e.Time)
					v.Phase = Began
					g.onChanged(v)
				}
			} else if g.onChanged != nil {
				v := a.rotateValue(s, c, e.Time)
				v.Phase = Changed
				g.onChanged(v)
			}
			used = true
		}
	}
	return used
}

func (a *arena) up(ctx *gift.EventContext, e gift.Event, gs []Gesture, c Common) bool {
	i := a.find(e.Pointer)
	if i < 0 {
		return false
	}
	a.fingers[i].pos = e.Pos
	prev := a.centroid
	a.rebase()
	a.travel = a.travel.Add(a.centroid.Sub(prev))
	a.track.add(e.Time, a.travel)
	last := a.n == 1
	used := false
	for k, g := range gs[:a.nshape] {
		s := &a.st[k]
		switch g := g.(type) {
		case TapGesture:
			if s.failed || a.claimed || !e.Inside || e.Dragged && dist(e.Pos, s.downPos) > Slop {
				s.taps = 0
				continue
			}
			s.taps++
			s.lastUp, s.lastPos = e.Time, e.Pos
			used = true
			switch {
			case s.taps < g.count:
				s.deadline = e.Time + MultiTapInterval
			case a.longerTapPending(gs, k, s.taps):
				s.waiting = true
				s.deadline = e.Time + MultiTapInterval
			default:
				s.taps = 0
				a.silenceShorterTaps(gs, g.count)
				if g.fn != nil {
					g.fn(TapValue{Common: c, Location: e.Pos, Count: g.count})
				}
			}
		case LongPressGesture:
			s.failed = true
		case DragGesture:
			if !s.began || !last {
				continue
			}
			s.began = false
			used = true
			if g.onEnded != nil {
				v := a.dragValue(s, c, e.Time)
				v.Phase = Ended
				v.Fingers = 0
				g.onEnded(v)
			}
		case MagnifyGesture:
			if !s.began || (s.pair[0] != e.Pointer && s.pair[1] != e.Pointer) {
				continue
			}
			if a.n-1 >= 2 {
				// Another pair forms from what remains; the value stays
				// continuous.
				continue
			}
			s.began, s.d0 = false, 0
			used = true
			if g.onEnded != nil {
				v := a.magnifyValue(s, c, e.Time)
				v.Phase = Ended
				g.onEnded(v)
			}
		case RotateGesture:
			if !s.began || (s.pair[0] != e.Pointer && s.pair[1] != e.Pointer) || a.n-1 >= 2 {
				continue
			}
			s.began, s.d0 = false, 0
			used = true
			if g.onEnded != nil {
				v := a.rotateValue(s, c, e.Time)
				v.Phase = Ended
				g.onEnded(v)
			}
		}
	}
	a.remove(i)
	a.rebase()
	for k, g := range gs[:a.nshape] {
		switch g.(type) {
		case MagnifyGesture, RotateGesture:
			s := &a.st[k]
			if a.n >= 2 {
				a.formPair(s)
			}
		}
	}
	a.arm(ctx, e.Time)
	return used
}

func (a *arena) cancel(e gift.Event, gs []Gesture) bool {
	i := a.find(e.Pointer)
	if i < 0 {
		return false
	}
	for k, g := range gs[:a.nshape] {
		s := &a.st[k]
		was := s.began
		s.began, s.failed, s.waiting, s.taps = false, true, false, 0
		if !was {
			continue
		}
		switch g := g.(type) {
		case DragGesture:
			if g.onCancel != nil {
				g.onCancel()
			}
		case MagnifyGesture:
			if g.onCancel != nil {
				g.onCancel()
			}
		case RotateGesture:
			if g.onCancel != nil {
				g.onCancel()
			}
		}
	}
	a.remove(i)
	a.rebase()
	return true
}

func (a *arena) hover(e gift.Event, gs []Gesture, c Common, inside bool) bool {
	if e.Device != gift.PointerMouse {
		return false
	}
	for k, g := range gs[:a.nshape] {
		h, ok := g.(HoverGesture)
		if !ok || h.fn == nil {
			continue
		}
		s := &a.st[k]
		if !inside && !s.hoverInside {
			continue
		}
		if inside && s.hoverInside && e.Pos == s.anchor {
			// A backend reports the cursor on every tick, moved or not;
			// a mouse that rests is not hovering anew.
			continue
		}
		s.hoverInside, s.anchor = inside, e.Pos
		h.fn(HoverValue{Common: c, Location: e.Pos, Inside: inside})
	}
	return false
}

func (a *arena) timer(ctx *gift.EventContext, now time.Duration, gs []Gesture, c Common) {
	for k, g := range gs[:a.nshape] {
		s := &a.st[k]
		if s.deadline == 0 || now < s.deadline {
			continue
		}
		s.deadline = 0
		switch g := g.(type) {
		case TapGesture:
			if s.waiting {
				s.waiting, s.taps = false, 0
				if g.fn != nil {
					g.fn(TapValue{Common: c, Location: s.lastPos, Count: g.count})
				}
				continue
			}
			s.taps = 0
		case LongPressGesture:
			if s.failed || a.n != 1 {
				continue
			}
			s.failed = true
			a.claimed = true
			c.Device = a.device
			// The press is the long press's now: whatever captured the
			// finger — a tile, a button — is told it was cancelled and does
			// not see the release.
			ctx.Capture(a.fingers[0].id)
			if g.fn != nil {
				g.fn(LongPressValue{Common: c, Location: s.downPos})
			}
		}
	}
	a.arm(ctx, now)
}

// arm asks for a timer at the earliest deadline of the set.
func (a *arena) arm(ctx *gift.EventContext, now time.Duration) {
	var next time.Duration
	for k := range a.nshape {
		d := a.st[k].deadline
		if d != 0 && (next == 0 || d < next) {
			next = d
		}
	}
	if next == 0 {
		return
	}
	ctx.After(max(next-now, time.Millisecond))
}

// silenceShorterTaps stops every tap of a lower count than one that just
// fired: their taps were part of it, whichever order the set lists them in.
func (a *arena) silenceShorterTaps(gs []Gesture, count int) {
	for j, g := range gs[:a.nshape] {
		if t, ok := g.(TapGesture); ok && t.count < count {
			s := &a.st[j]
			s.taps, s.waiting, s.failed = 0, false, true
		}
	}
}

// longerTapPending reports whether a tap with a higher count than the one at
// index k is still on its way to completion after taps taps.
func (a *arena) longerTapPending(gs []Gesture, k, taps int) bool {
	for j, g := range gs[:a.nshape] {
		t, ok := g.(TapGesture)
		if !ok || j == k || t.count <= taps {
			continue
		}
		if s := &a.st[j]; !s.failed && s.taps == taps {
			return true
		}
	}
	return false
}

// beginDrag begins a drag unless its ShouldBegin declines, and reports which.
func (a *arena) beginDrag(ctx *gift.EventContext, s *state, g DragGesture, c Common, e gift.Event) bool {
	v := a.dragValue(s, c, e.Time)
	v.Phase = Began
	if g.should != nil && !g.should(v) {
		s.failed = true
		return false
	}
	s.began = true
	ctx.StealPointer()
	if g.onChanged != nil {
		g.onChanged(v)
	}
	return true
}

func (a *arena) dragValue(s *state, c Common, now time.Duration) DragValue {
	c.Device = a.device
	t := a.travel.Sub(s.from)
	v := a.track.velocity(now)
	start := a.origin.Add(s.from)
	return DragValue{
		Common:        c,
		Location:      start.Add(t),
		StartLocation: start,
		Translation:   t,
		Velocity:      v,
		PredictedEndTranslation: geom.Pt(
			t.X+float32(motion.Project(float64(v.X), motion.DecelerationNormal)),
			t.Y+float32(motion.Project(float64(v.Y), motion.DecelerationNormal))),
		Fingers: a.n,
	}
}

// formPair makes the first two fingers the pair of a magnify or rotate state,
// carrying the value over so it stays continuous.
func (a *arena) formPair(s *state) {
	if a.n < 2 {
		return
	}
	p := [2]gift.PointerID{a.fingers[0].id, a.fingers[1].id}
	if s.pair == p && s.d0 > 0 {
		return
	}
	fresh := s.d0 == 0 || !s.began
	switch s.k {
	case kindMagnify:
		if fresh {
			s.base, s.cur = 1, 1
			s.samples.reset()
		} else {
			s.base = s.cur
		}
	case kindRotate:
		if fresh {
			s.base, s.cur = 0, 0
			s.samples.reset()
		} else {
			s.base = s.cur
		}
	}
	s.pair = p
	s.d0 = a.pairDist()
	s.a0 = a.pairAngle()
	if fresh {
		s.anchor = a.centroid
	}
}

func (a *arena) pairDist() float64 {
	d := a.fingers[0].pos.Sub(a.fingers[1].pos)
	return math.Hypot(float64(d.X), float64(d.Y))
}

func (a *arena) pairAngle() float64 {
	d := a.fingers[1].pos.Sub(a.fingers[0].pos)
	return math.Atan2(float64(d.Y), float64(d.X))
}

func (a *arena) updatePair(s *state, rotate bool) {
	if s.d0 == 0 {
		a.formPair(s)
	}
	if rotate {
		d := a.pairAngle() - s.a0
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		s.cur = s.base + d
		return
	}
	if s.d0 > 0 {
		s.cur = s.base * a.pairDist() / s.d0
	}
}

func (a *arena) magnifyValue(s *state, c Common, now time.Duration) MagnifyValue {
	s.samples.add(now, geom.Pt(float32(s.cur), 0))
	return MagnifyValue{
		Common:        c,
		Magnification: s.cur,
		Velocity:      float64(s.samples.velocity(now).X),
		StartAnchor:   s.anchor,
		Location:      a.centroid,
	}
}

func (a *arena) rotateValue(s *state, c Common, now time.Duration) RotateValue {
	s.samples.add(now, geom.Pt(float32(s.cur), 0))
	return RotateValue{
		Common:      c,
		Rotation:    s.cur,
		Velocity:    float64(s.samples.velocity(now).X),
		StartAnchor: s.anchor,
		Location:    a.centroid,
	}
}

func (a *arena) remove(i int) {
	copy(a.fingers[i:a.n], a.fingers[i+1:a.n])
	a.n--
	a.fingers[a.n] = finger{}
}

func dist(p, q geom.Point) float32 { return length(p.Sub(q)) }

func length(p geom.Point) float32 {
	return float32(math.Hypot(float64(p.X), float64(p.Y)))
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
