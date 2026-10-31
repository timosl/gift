package gift

import (
	"time"

	"github.com/worldiety/gift/internal/scene"
)

// After asks for one [EventTimer] to be delivered to the receiving node d
// from now, on the clock [App.BeginInput] is given.
//
// # What it is for
//
// A recogniser has decisions that only time can make, and nothing else
// happens while it waits: a single tap is a single tap only once the second
// tap of a double tap has failed to arrive, and a press is a long press once
// it has rested long enough. gift fires its own long press from
// [App.BeginInput] for exactly this reason; this is the same mechanism made
// available to a node, so a recogniser with its own durations does not have to
// poll from a painter.
//
// # Rules
//
// A node has at most one pending timer. Asking again replaces it, and a d of
// zero or less cancels it. The timer is dropped when the node is unmounted or
// hidden, like an [EventContext.Animate] enrolment, and it keeps the node
// marked for repaint until it fires, so a backend that idles between frames
// still ticks in time. The event goes to the node and does not bubble.
func (c *EventContext) After(d time.Duration) { c.app.after(c.cur, d) }

// After is [EventContext.After] for the node being laid out. A layouter that
// starts something with an end — a transition it was asked for by a model the
// application changed between frames — arms the timer that finishes it.
func (l *LayoutContext) After(d time.Duration) { l.app.after(l.node, d) }

func (a *App) after(h scene.Handle, d time.Duration) {
	if !a.store.Valid(h) {
		return
	}
	if d <= 0 {
		a.stopTimer(h)
		return
	}
	if a.hiddenAbove(h) {
		return
	}
	// The repaint enrolment is what keeps the backend ticking until the
	// deadline: an idle application would otherwise drop to its idle rate and
	// fire a 300 ms decision a second late.
	a.animate(h, d)
	at := a.in.now + d
	for i := range a.in.timers {
		if a.in.timers[i].node == h {
			a.in.timers[i].until = at
			return
		}
	}
	a.in.timers = append(a.in.timers, animation{node: h, until: at})
}

func (a *App) stopTimer(h scene.Handle) {
	for i, tm := range a.in.timers {
		if tm.node == h {
			a.in.timers = append(a.in.timers[:i], a.in.timers[i+1:]...)
			return
		}
	}
}

// tickTimers delivers every timer that is due and drops it. A handler that
// re-arms from its own timer event is honoured: the entry is removed before
// the delivery.
func (a *App) tickTimers(now time.Duration) {
	for i := 0; i < len(a.in.timers); {
		tm := a.in.timers[i]
		if !a.store.Valid(tm.node) {
			a.in.timers = append(a.in.timers[:i], a.in.timers[i+1:]...)
			continue
		}
		if now < tm.until {
			i++
			continue
		}
		a.in.timers = append(a.in.timers[:i], a.in.timers[i+1:]...)
		a.deliver(tm.node, Event{Kind: EventTimer, Mods: a.in.mods, Time: now}, true)
	}
}
