package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// Metrics of pull-to-refresh: those of UIRefreshControl. The spinner is
// [paintSpinner].
const (
	// refreshTrigger is how far the content has to be displaced past its top
	// for the refresh to start.
	refreshTrigger = float32(70)
	// refreshHold is how far below its top the content rests while the
	// refresh runs.
	refreshHold = float32(56)
)

// refreshSpec is what a refreshable scroll view knows about its refresh.
type refreshSpec struct {
	refreshing bool
	onRefresh  func()
	spokes     Color
}

// refreshState is kept across builds: whether this pull has started a
// refresh already.
type refreshState struct{ fired bool }

// handleEvent starts the refresh once per pull, as soon as the content is
// pulled far enough, after the scroll gesture has moved it.
func (r *refreshSpec) handleEvent(ctx *gift.EventContext, e gift.Event) {
	st := ctx.Retained[refreshState]()
	switch e.Kind {
	case gift.EventPointerDown:
		st.fired = false
	case gift.EventPointerMove:
		info, ok := ctx.ScrollInfo()
		if !ok || !info.Dragging || st.fired || r.refreshing || r.onRefresh == nil {
			return
		}
		if info.Overscroll <= -refreshTrigger {
			st.fired = true
			r.onRefresh()
		}
	}
}

// paint draws the spinner in the room the content left above it: its spokes
// appear one by one as the pull nears the trigger, and turn while the
// refresh runs.
func (r *refreshSpec) paint(ctx *gift.PaintContext) {
	info, ok := ctx.ScrollInfo()
	if !ok {
		return
	}
	over := -info.Overscroll
	if over <= 0 {
		return
	}
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, b.Min.Y+min(over, refreshHold)/2)
	shown := int(float32(spinnerSpokes) * min(over/refreshTrigger, 1))
	paintSpinner(ctx, c, 1, r.spokes, shown, r.refreshing)
}
