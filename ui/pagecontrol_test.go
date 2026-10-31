package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func pageScene(t *testing.T, count, start int) (*gifttest.Harness, *[]int) {
	t.Helper()
	got := new([]int)
	root := func(ctx *gift.Context) gift.View {
		p := ctx.State("p", start)
		return ui.VStack(ui.PageControl(count, ctx.Read(p), func(i int) { *got = append(*got, i); p.Set(i) }).Key("pages")).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 200), Font: loadTestFont(t)})
	h.Settle()
	return h, got
}

// dot is the centre of the dot of page i.
func dot(h *gifttest.Harness, count, i int) geom.Point {
	b := h.Find(gifttest.ByKey("pages")).Bounds()
	left := b.Min.X + (b.Width()-(float32(count-1)*16+7))/2 + 3.5
	return geom.Pt(left+float32(i)*16, (b.Min.Y+b.Max.Y)/2)
}

func TestATapBesideTheCurrentDotTurnsOnePage(t *testing.T) {
	h, got := pageScene(t, 5, 2)
	if s := h.Find(gifttest.ByKey("pages")).LayoutBounds().Size(); s.H != 26 || s.W != 5*16+20-9 {
		t.Fatalf("five dots take %v", s)
	}
	h.TapAt(dot(h, 5, 4))
	h.Advance(time.Second)
	h.TapAt(dot(h, 5, 0))
	h.Advance(time.Second)
	h.TapAt(dot(h, 5, 0))
	h.Advance(time.Second)
	h.TapAt(dot(h, 5, 0))
	want := []int{3, 2, 1, 0}
	if len(*got) != len(want) {
		t.Fatalf("taps turned to %v, want %v", *got, want)
	}
	for i := range want {
		if (*got)[i] != want[i] {
			t.Fatalf("taps turned to %v, want %v", *got, want)
		}
	}
	if v := h.Find(gifttest.ByKey("pages")).Value(); v != "page 1 of 5" {
		t.Fatalf("the control reports %q", v)
	}
}

func TestAFingerScrubsAlongTheDotsAndLiftsWithoutAnotherStep(t *testing.T) {
	h, got := pageScene(t, 6, 0)
	from := dot(h, 6, 0)
	h.FingerDown(1, from)
	for i := 1; i <= 5; i++ {
		h.FingerMove(1, dot(h, 6, i))
		h.AdvanceTicks(2, gifttest.FrameInterval)
	}
	h.FingerUp(1, dot(h, 6, 5))
	h.Settle()
	if last := (*got)[len(*got)-1]; last != 5 {
		t.Fatalf("scrubbing to the last dot ended on %d (%v)", last, *got)
	}
	if v := h.Find(gifttest.ByKey("pages")).Value(); v != "page 6 of 6" {
		t.Fatalf("after the scrub the control reports %q", v)
	}
}

func TestTheArrowKeysTurnPagesAndOnePageDrawsNothing(t *testing.T) {
	h, got := pageScene(t, 3, 0)
	h.Find(gifttest.ByKey("pages")).Focus()
	h.Key(gift.KeyRight)
	h.Key(gift.KeyRight)
	h.Key(gift.KeyRight)
	h.Key(gift.KeyLeft)
	if len(*got) != 3 || (*got)[2] != 1 {
		t.Fatalf("right three times and left once turned to %v", *got)
	}
	g, _ := pageScene(t, 1, 0)
	if s := g.Find(gifttest.ByKey("pages")).LayoutBounds().Size(); s.W != 0 {
		t.Fatalf("a single page takes %v", s)
	}
}
