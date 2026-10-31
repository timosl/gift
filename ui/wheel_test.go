package ui_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// wheelScene is one wheel of 30 rows bound to a state, and records every
// choice it reports.
func wheelScene(t *testing.T, start int, cyclic bool) (*gifttest.Harness, *[]int) {
	t.Helper()
	got := new([]int)
	root := func(ctx *gift.Context) gift.View {
		sel := ctx.State("sel", start)
		ctx.Read(sel)
		return ui.VStack(ui.WheelPicker(
			ui.Wheel(30, func(i int) string { return "Row " + strconv.Itoa(i) }).
				Selected(sel.Get()).Cyclic(cyclic).
				OnChange(func(i int) { *got = append(*got, i); sel.Set(i) }),
		).Width(240).Key("picker")).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 300), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, got
}

func lastChoice(xs []int) int {
	if len(xs) == 0 {
		return -1
	}
	return xs[len(xs)-1]
}

func TestAFlickTurnsTheWheelSeveralRowsAndSettlesOnOne(t *testing.T) {
	h, got := wheelScene(t, 5, false)
	b := h.Find(gifttest.ByKey("picker")).LayoutBounds()
	c := centreOf(b)
	// Upwards at 1200 px/s: the projection runs on about 600 px, some
	// seventeen rows of 36.
	h.Fling(geom.Pt(c.X, c.Y+60), geom.Pt(c.X, c.Y-60), 100*time.Millisecond)
	if v := lastChoice(*got); v < 15 || v > 29 {
		t.Fatalf("a flick upwards chose %v, want far down the list", *got)
	}
	h.AdvanceTicks(60, gifttest.FrameInterval)
	// A slow drag down by two rows and a half turns back by about two.
	before := lastChoice(*got)
	h.Fling(geom.Pt(c.X, c.Y-40), geom.Pt(c.X, c.Y+50), time.Second)
	if v := lastChoice(*got); v != before-2 && v != before-3 {
		t.Errorf("a slow drag of 90 px from %d chose %d, want two or three rows back", before, v)
	}
}

func TestTheFirstRowIsAnEndAndACyclicWheelHasNone(t *testing.T) {
	h, got := wheelScene(t, 1, false)
	c := centreOf(h.Find(gifttest.ByKey("picker")).LayoutBounds())
	h.Fling(geom.Pt(c.X, c.Y-60), geom.Pt(c.X, c.Y+80), 100*time.Millisecond)
	if v := lastChoice(*got); v != 0 {
		t.Errorf("a flick past the first row chose %d, want 0", v)
	}
	h, got = wheelScene(t, 0, true)
	h.Find(gifttest.ByKey("wheel-0")).Focus()
	h.Key(gift.KeyUp)
	if v := lastChoice(*got); v != 29 {
		t.Errorf("up from the first row of a cyclic wheel chose %d, want the last", v)
	}
}

func TestATapAndTheKeysAndTheWheelChooseRows(t *testing.T) {
	h, got := wheelScene(t, 10, false)
	c := centreOf(h.Find(gifttest.ByKey("picker")).LayoutBounds())
	// One row below the middle: 36 px down, a little less on the cylinder.
	h.TapAt(geom.Pt(c.X, c.Y+34))
	if v := lastChoice(*got); v != 11 {
		t.Fatalf("a tap one row below chose %d, want 11", v)
	}
	h.AdvanceTicks(40, gifttest.FrameInterval)
	h.Find(gifttest.ByKey("wheel-0")).Focus()
	h.Key(gift.KeyDown)
	if v := lastChoice(*got); v != 12 {
		t.Errorf("down chose %d, want 12", v)
	}
	h.Key(gift.KeyEnd)
	if v := lastChoice(*got); v != 29 {
		t.Errorf("end chose %d, want 29", v)
	}
	h.Wheel(c, geom.Pt(0, 2))
	if v := lastChoice(*got); v != 27 {
		t.Errorf("two notches away chose %d, want 27", v)
	}
}

func TestADateWheelKeepsTheDayInsideTheMonthAndTheRange(t *testing.T) {
	var chosen ui.Date
	root := func(ctx *gift.Context) gift.View {
		d := ctx.State("d", ui.DateOf(2024, time.January, 31))
		return ui.VStack(ui.DateWheel().Bind(d.Binding()).
			Range(ui.DateOf(2020, time.January, 1), ui.DateOf(2025, time.June, 30)).
			OnChange(func(v ui.Date) { chosen = v })).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(500, 300), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	h.Find(gifttest.ByKey("month")).Focus()
	h.Key(gift.KeyDown) // February 2024 has 29 days.
	h.Settle()
	if chosen != ui.DateOf(2024, time.February, 29) {
		t.Fatalf("31 January plus a month is %v, want 29 February 2024", chosen)
	}
	h.Find(gifttest.ByKey("year")).Focus()
	h.Key(gift.KeyEnd) // 2025, and February 2025 is inside the range.
	h.Settle()
	if chosen != ui.DateOf(2025, time.February, 28) {
		t.Errorf("the last year chose %v, want 28 February 2025", chosen)
	}
	h.Find(gifttest.ByKey("month")).Focus()
	h.Key(gift.KeyEnd) // December 2025 is past the end of the range.
	h.Settle()
	if chosen != ui.DateOf(2025, time.June, 30) {
		t.Errorf("December 2025 became %v, want the end of the range", chosen)
	}
}

func TestATimeWheelStepsByItsInterval(t *testing.T) {
	var chosen ui.TimeOfDay
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.TimeWheel().Value(ui.TimeOf(9, 20)).MinuteInterval(15).
			OnChange(func(v ui.TimeOfDay) { chosen = v })).Padding(20),
		Size: geom.Sz(400, 300), Font: loadTestFont(t), Theme: ui.LightTheme(),
	})
	h.Settle()
	h.Find(gifttest.ByKey("minute")).Focus()
	h.Key(gift.KeyDown)
	if chosen != ui.TimeOf(9, 30) {
		t.Errorf("down from 9:20 at 15 minutes chose %v, want 9:30", chosen)
	}
	h.Find(gifttest.ByKey("hour")).Focus()
	h.Key(gift.KeyUp)
	if chosen.Hour() != 8 {
		t.Errorf("up on the hour chose %v, want 8 o'clock", chosen)
	}
}

// TestTurningTheWheelDrawsItsRowsInFewSizes pins the size ladder of the
// perspective. Every size a row is drawn at is one the glyph atlas rasterises
// and keeps, so a scale that followed the angle continuously was a new size
// for every row on every frame of a turn: a few turns filled the atlas, which
// then turned glyphs away, and digits went missing from the wheel.
func TestTurningTheWheelDrawsItsRowsInFewSizes(t *testing.T) {
	h, _ := wheelScene(t, 15, false)
	at := h.Find(gifttest.ByKey("picker")).Center()
	sizes := map[float32]bool{}
	h.TouchDownAt(at)
	for i := 1; i <= 150; i++ {
		h.TouchMoveTo(geom.Pt(at.X, at.Y+float32(i)*0.7))
		h.Frame()
		l := h.List()
		for _, op := range l.Ops() {
			if op.Kind != render.OpGlyphs {
				continue
			}
			s := l.Xform(op.Xform).A
			for _, g := range l.Glyphs(op.Glyphs, op.GlyphCount) {
				sizes[g.Size*s] = true
			}
		}
	}
	h.TouchUpAt(geom.Pt(at.X, at.Y+105))
	h.Settle()
	// The rows turn through every angle of the visible half cylinder, which
	// the ladder covers in 0.3*64 steps.
	if len(sizes) > 24 {
		t.Errorf("a turn drew the rows in %d sizes, want a ladder of at most 24", len(sizes))
	}
}
