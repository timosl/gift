package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

type toastLog struct{ dismissed, tapped, pressed int }

// toastScene is a button under a toast that the test presents through show.
func toastScene(t *testing.T, edge ui.ToastEdge) (*gifttest.Harness, *toastLog, func()) {
	t.Helper()
	log := &toastLog{}
	var show func()
	root := func(ctx *gift.Context) gift.View {
		on := ctx.State("on", false)
		show = func() { on.Set(true) }
		return ui.Toast(
			ui.VStack(ui.Button(ui.Text("Under"), func() { log.pressed++ }).Key("under")).Padding(20),
			"Link copied",
		).Subtitle("example.com").Edge(edge).
			Presented(ctx.Read(on)).
			OnDismiss(func() { log.dismissed++; on.Set(false) }).
			OnTap(func() { log.tapped++ })
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 300), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, log, func() { h.App().BeginInput(h.Now()); show(); h.Settle() }
}

func card(h *gifttest.Harness) gifttest.Node { return h.Find(gifttest.ByType("ui.toastCard")) }

func TestAToastComesGoesByItselfAndBlocksNothing(t *testing.T) {
	h, log, show := toastScene(t, ui.ToastTop)
	h.AssertNone(gifttest.ByType("ui.toastCard"))
	show()
	h.AdvanceTicks(10, gifttest.FrameInterval)
	b := card(h).LayoutBounds()
	if b.Min.Y != 12 || b.Height() < 50 || (b.Min.X+b.Max.X)/2 != 200 {
		t.Fatalf("the toast is at %v, want centred 12 below the top and 50 tall at least", b)
	}
	// A press beside the capsule reaches the button under the layer.
	h.Find(gifttest.ByKey("under")).Tap()
	if log.pressed != 1 {
		t.Fatal("the toast blocked a tap on the button beside it")
	}
	h.Advance(2 * time.Second)
	h.AdvanceTicks(30, gifttest.FrameInterval)
	if log.dismissed != 1 {
		t.Fatalf("after its time the toast was dismissed %d times", log.dismissed)
	}
	h.AssertNone(gifttest.ByType("ui.toastCard"))
}

func TestASwipeToTheEdgeDismissesAndAShortOneSpringsBack(t *testing.T) {
	h, log, show := toastScene(t, ui.ToastTop)
	show()
	h.AdvanceTicks(40, gifttest.FrameInterval)
	c := centreOf(card(h).Bounds())
	// Down, inwards: a rubber band, and back.
	h.FingerDown(1, c)
	h.FingerMove(1, geom.Pt(c.X, c.Y+30))
	h.FingerUp(1, geom.Pt(c.X, c.Y+30))
	h.AdvanceTicks(10, gifttest.FrameInterval)
	if log.dismissed != 0 {
		t.Fatal("a pull inwards dismissed the toast")
	}
	// Held, it outlasts its time.
	h.FingerDown(1, c)
	h.Advance(3 * time.Second)
	if log.dismissed != 0 {
		t.Fatal("a held toast was dismissed")
	}
	h.FingerMove(1, geom.Pt(c.X, c.Y-12))
	h.FingerMove(1, geom.Pt(c.X, c.Y-30))
	h.FingerUp(1, geom.Pt(c.X, c.Y-30))
	if log.dismissed != 1 || log.tapped != 0 {
		t.Fatalf("a swipe up dismissed %d times and tapped %d", log.dismissed, log.tapped)
	}
}

func TestATapOnAToastCallsOnTapAndABottomToastSitsAtTheBottom(t *testing.T) {
	h, log, show := toastScene(t, ui.ToastBottom)
	show()
	h.AdvanceTicks(40, gifttest.FrameInterval)
	b := card(h).LayoutBounds()
	if b.Max.Y != 300-12 {
		t.Fatalf("a bottom toast ends at %v, want 288", b.Max.Y)
	}
	card(h).Tap()
	if log.tapped != 1 || log.dismissed != 1 {
		t.Fatalf("a tap called OnTap %d times and OnDismiss %d", log.tapped, log.dismissed)
	}
}
