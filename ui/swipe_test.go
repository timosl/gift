package ui_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

type swipeLog struct {
	tapped  [3]int
	deleted []int
	flagged []int
	read    []int
}

// swipeScene is a scrolling list of three rows, each a button, with Delete
// and Flag behind it on the right and Read on the left.
func swipeScene(t *testing.T) (*gifttest.Harness, *swipeLog) {
	t.Helper()
	log := &swipeLog{}
	root := func(ctx *gift.Context) gift.View {
		rows := make([]gift.View, 0, 13)
		for i := range 3 {
			rows = append(rows, ui.Swipe(
				ui.Button(ui.Text("Row "+strconv.Itoa(i)), func() { log.tapped[i]++ }).Frame(geom.Unbounded(), 60).Key("row-"+strconv.Itoa(i)),
			).Trailing(
				ui.SwipeAction("Delete", func() { log.deleted = append(log.deleted, i) }).Destructive(),
				ui.SwipeAction("Flag", func() { log.flagged = append(log.flagged, i) }),
			).Leading(
				ui.SwipeAction("Read", func() { log.read = append(log.read, i) }),
			).Key("swipe-"+strconv.Itoa(i)))
		}
		for range 10 {
			rows = append(rows, ui.Box().Frame(10, 60))
		}
		return ui.VScroll(rows...).Frame(360, 400).Key("list")
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(360, 400), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, log
}

// drag moves a finger from a by d in steps of ten, a frame apart.
func drag(h *gifttest.Harness, a geom.Point, d geom.Point, lift bool) {
	h.FingerDown(1, a)
	steps := int(max(absf(d.X), absf(d.Y)) / 10)
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		h.FingerMove(1, geom.Pt(a.X+d.X*f, a.Y+d.Y*f))
		h.AdvanceTicks(1, gifttest.FrameInterval)
	}
	if lift {
		h.FingerUp(1, geom.Pt(a.X+d.X, a.Y+d.Y))
	}
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func rowCentre(h *gifttest.Harness, i int) geom.Point {
	return centreOf(h.Find(gifttest.ByKey("row-" + strconv.Itoa(i))).Bounds())
}

func TestASwipePastHalfOpensTheRowAndAnActionRunsAndCloses(t *testing.T) {
	h, log := swipeScene(t)
	c := rowCentre(h, 0)
	drag(h, c, geom.Pt(-140, 0), true)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if log.tapped[0] != 0 {
		t.Fatal("the swipe tapped the button it started on")
	}
	in := h.Find(gifttest.ByKey("swipe-0"))
	del := in.Find(gifttest.ByKey("swipe-Delete")).LayoutBounds()
	row := h.Find(gifttest.ByKey("swipe-0")).LayoutBounds()
	if del.Max.X != row.Max.X || del.Width() < 74 {
		t.Fatalf("Delete is at %v in the row at %v, want at its right edge", del, row)
	}
	// The content moved left by the width of both actions.
	flag := in.Find(gifttest.ByKey("swipe-Flag")).LayoutBounds()
	if content := h.Find(gifttest.ByKey("row-0")).LayoutBounds(); content.Max.X > flag.Min.X+0.5 {
		t.Fatalf("the open row's content ends at %v, over its actions from %v", content.Max.X, flag.Min.X)
	}
	in.Find(gifttest.ByKey("swipe-Flag")).Tap()
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if len(log.flagged) != 1 || log.flagged[0] != 0 {
		t.Fatalf("a tap on Flag flagged %v", log.flagged)
	}
	if content := h.Find(gifttest.ByKey("row-0")).LayoutBounds(); content.Min.X != row.Min.X {
		t.Fatalf("after the action the content starts at %v, want closed at %v", content.Min.X, row.Min.X)
	}
}

func TestAShortSwipeSpringsBackAndATapElsewhereOnlyCloses(t *testing.T) {
	h, log := swipeScene(t)
	row := h.Find(gifttest.ByKey("swipe-1")).LayoutBounds()
	// Slowly, and held still before it lifts: no throw.
	drag(h, rowCentre(h, 1), geom.Pt(-40, 0), false)
	h.AdvanceTicks(6, gifttest.FrameInterval)
	h.FingerUp(1, geom.Pt(rowCentre(h, 1).X-40, rowCentre(h, 1).Y))
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if content := h.Find(gifttest.ByKey("row-1")).LayoutBounds(); content.Min.X != row.Min.X {
		t.Fatalf("a short swipe left the content at %v", content.Min.X)
	}
	h.Advance(time.Second)
	drag(h, rowCentre(h, 1), geom.Pt(-140, 0), true)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	// A tap on another row's button closes the open row and does not tap.
	h.Advance(time.Second)
	h.TapAt(rowCentre(h, 2))
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if log.tapped[2] != 0 {
		t.Fatal("the tap that closed the open row also tapped another")
	}
	if content := h.Find(gifttest.ByKey("row-1")).LayoutBounds(); content.Min.X != row.Min.X {
		t.Fatalf("a tap elsewhere left the row open at %v", content.Min.X)
	}
	h.Advance(time.Second)
	h.TapAt(rowCentre(h, 2))
	if log.tapped[2] != 1 {
		t.Fatal("with no row open a tap does not reach the button")
	}
}

func TestAFullSwipeRunsTheFirstActionAndALeadingSwipeOpensTheOtherSide(t *testing.T) {
	h, log := swipeScene(t)
	drag(h, rowCentre(h, 0), geom.Pt(-300, 0), true)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	if len(log.deleted) != 1 || log.deleted[0] != 0 {
		t.Fatalf("a full swipe deleted %v", log.deleted)
	}
	h.Advance(time.Second)
	drag(h, rowCentre(h, 1), geom.Pt(120, 0), true)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	h.Find(gifttest.ByKey("swipe-1")).Find(gifttest.ByKey("swipe-Read")).Tap()
	if len(log.read) != 1 || log.read[0] != 1 {
		t.Fatalf("Read on the left ran for %v", log.read)
	}
}

func TestAVerticalStrokeOverARowScrollsTheList(t *testing.T) {
	h, _ := swipeScene(t)
	drag(h, rowCentre(h, 1), geom.Pt(4, -120), true)
	if off := h.Find(gifttest.ByKey("list")).ScrollInfo().Offset; off < 50 {
		t.Fatalf("a stroke up over a row scrolled the list by %v", off)
	}
	row := h.Find(gifttest.ByKey("swipe-1")).LayoutBounds()
	if content := h.Find(gifttest.ByKey("row-1")).LayoutBounds(); content.Min.X != row.Min.X {
		t.Fatal("a vertical stroke moved the row sideways")
	}
}
