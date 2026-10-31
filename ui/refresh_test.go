package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// refreshScene is a refreshable list whose refreshes the test ends through
// done.
func refreshScene(t *testing.T) (h *gifttest.Harness, calls *int, done func()) {
	t.Helper()
	calls = new(int)
	var set func(bool)
	root := func(ctx *gift.Context) gift.View {
		busy := ctx.State("busy", false)
		set = busy.Set
		rows := make([]gift.View, 30)
		for i := range rows {
			rows[i] = ui.Box().Frame(200, 40)
		}
		return ui.VScroll(rows...).Gap(4).Refreshable(ctx.Read(busy), func() { *calls++; busy.Set(true) }).
			Frame(300, 300).Key("scroll")
	}
	h = gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(300, 300), Font: loadTestFont(t)})
	h.Settle()
	return h, calls, func() { h.App().BeginInput(h.Now()); set(false); h.Settle() }
}

// pull drags a finger down by d in steps of five, a frame apart.
func pull(h *gifttest.Harness, d float32, lift bool) {
	from := geom.Pt(150, 60)
	h.FingerDown(1, from)
	for y := float32(5); y <= d; y += 5 {
		h.FingerMove(1, geom.Pt(from.X, from.Y+y))
		h.AdvanceTicks(1, gifttest.FrameInterval)
	}
	if lift {
		h.FingerUp(1, geom.Pt(from.X, from.Y+d))
	}
}

func overscroll(h *gifttest.Harness) float32 {
	return h.Find(gifttest.ByKey("scroll")).ScrollInfo().Overscroll
}

func TestAPullFarEnoughRefreshesOnceAndHoldsTheContent(t *testing.T) {
	h, calls, done := refreshScene(t)
	pull(h, 200, false)
	if *calls != 1 {
		t.Fatalf("a long pull refreshed %d times before the finger lifted, want once", *calls)
	}
	h.FingerUp(1, geom.Pt(150, 260))
	h.AdvanceTicks(60, gifttest.FrameInterval)
	if got := overscroll(h); got < -57 || got > -55 {
		t.Fatalf("while refreshing the content rests %v past its top, want 56", -got)
	}
	// The spinner turns: eight spokes in the room above the content.
	if n := countSpokes(h); n != 8 {
		t.Fatalf("the spinner has %d spokes", n)
	}
	done()
	h.AdvanceTicks(60, gifttest.FrameInterval)
	if got := overscroll(h); got != 0 {
		t.Fatalf("after the refresh the content rests %v past its top", -got)
	}
	if *calls != 1 {
		t.Fatalf("one pull refreshed %d times", *calls)
	}
}

func TestAShortPullDoesNotRefresh(t *testing.T) {
	h, calls, _ := refreshScene(t)
	pull(h, 60, true)
	h.AdvanceTicks(60, gifttest.FrameInterval)
	if *calls != 0 {
		t.Fatalf("a short pull refreshed %d times", *calls)
	}
	if got := overscroll(h); got != 0 {
		t.Fatalf("a short pull left the content %v past its top", -got)
	}
}

// countSpokes counts the rounded bars the spinner draws: rotated, so they
// are the round rects drawn under a transform other than the identity.
func countSpokes(h *gifttest.Harness) int {
	n := 0
	l := h.List()
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Xform != 0 {
			if x := l.Xform(op.Xform); x != (geom.Affine2D{}) {
				n++
			}
		}
	}
	return n
}
