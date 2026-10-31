package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// calendarScene is a calendar on the 15th of October 2026 that can choose
// from the 10th of January 2026 to the 20th of March 2027, and records the
// choices.
func calendarScene(t *testing.T, first time.Weekday) (*gifttest.Harness, *[]ui.Date) {
	t.Helper()
	chosen := new([]ui.Date)
	root := func(ctx *gift.Context) gift.View {
		d := ctx.State("d", ui.DateOf(2026, time.October, 15))
		return ui.VStack(ui.Calendar().Bind(d.Binding()).FirstWeekday(first).
			Range(ui.DateOf(2026, time.January, 10), ui.DateOf(2027, time.March, 20)).
			OnChange(func(v ui.Date) { *chosen = append(*chosen, v) }).Key("cal")).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(500, 500), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, chosen
}

func dayOf(h *gifttest.Harness, year int, month time.Month, day int) gifttest.Node {
	return h.Find(gifttest.ByKey("month-" + calItoa(year) + "-" + calItoa(int(month)))).Find(gifttest.ByKey("day-" + calItoa(day)))
}

func calItoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return calItoa(i/10) + string(rune('0'+i%10))
}

// rest lets the months come to rest: Settle does not move the clock.
func rest(h *gifttest.Harness) { h.AdvanceTicks(40, gifttest.FrameInterval) }

func title(h *gifttest.Harness) string { return h.Find(gifttest.ByKey("calendar-title")).Text() }

func lastDate(xs []ui.Date) ui.Date {
	if len(xs) == 0 {
		return ui.Date{}
	}
	return xs[len(xs)-1]
}

func TestTheDaysOfAMonthStandUnderTheirWeekdays(t *testing.T) {
	for _, tc := range []struct {
		first time.Weekday
		col   int // of the 1st of October 2026, a Thursday
	}{{time.Monday, 3}, {time.Sunday, 4}} {
		h, _ := calendarScene(t, tc.first)
		if got := title(h); got != "October 2026" {
			t.Fatalf("title %q", got)
		}
		one := dayOf(h, 2026, time.October, 1).LayoutBounds()
		eight := dayOf(h, 2026, time.October, 8).LayoutBounds()
		last := dayOf(h, 2026, time.October, 31).LayoutBounds()
		cell := eight.Min.Y - one.Min.Y
		if cell < 40 || cell > 48 {
			t.Fatalf("a week is %v high", cell)
		}
		// The 1st sits col cells right of the first column, which the 5th
		// (Monday) or the 4th (Sunday) opens.
		opener := 5
		if tc.first == time.Sunday {
			opener = 4
		}
		o := dayOf(h, 2026, time.October, opener).LayoutBounds()
		w := (centreOf(one).X - centreOf(o).X) / float32(tc.col)
		if d := centreOf(one).Y - centreOf(o).Y; d > -cell+1 {
			t.Errorf("first weekday %v: the %d. is not a row below the 1st", tc.first, opener)
		}
		if w < 40 || w > 80 {
			t.Errorf("first weekday %v: a column is %v wide", tc.first, w)
		}
		if last.Min.Y <= eight.Min.Y {
			t.Error("the 31st is not below the 8th")
		}
	}
}

func TestATapOnADayChoosesItAndADayOutsideTheRangeRefusesIt(t *testing.T) {
	h, chosen := calendarScene(t, time.Monday)
	h.TapAt(centreOf(dayOf(h, 2026, time.October, 20).LayoutBounds()))
	if got := lastDate(*chosen); got != ui.DateOf(2026, time.October, 20) {
		t.Fatalf("chose %v, want the 20th of October", got)
	}
	// Back to January, whose days before the 10th are out of range.
	for range 9 {
		h.Find(gifttest.ByKey("calendar-previous")).Tap()
	}
	rest(h)
	if got := title(h); got != "January 2026" {
		t.Fatalf("title %q after nine months back", got)
	}
	if !h.Find(gifttest.ByKey("calendar-previous")).Interaction().Disabled {
		t.Error("the previous arrow is enabled at the first month of the range")
	}
	n := len(*chosen)
	h.TapAt(centreOf(dayOf(h, 2026, time.January, 5).LayoutBounds()))
	if len(*chosen) != n {
		t.Fatalf("a day before the range was chosen: %v", lastDate(*chosen))
	}
	h.TapAt(centreOf(dayOf(h, 2026, time.January, 12).LayoutBounds()))
	if got := lastDate(*chosen); got != ui.DateOf(2026, time.January, 12) {
		t.Fatalf("chose %v, want the 12th of January", got)
	}
}

func TestASwipeTurnsTheMonthAndStopsAtTheRange(t *testing.T) {
	h, chosen := calendarScene(t, time.Monday)
	grid := dayOf(h, 2026, time.October, 15).LayoutBounds()
	c := centreOf(grid)
	// A flick to the left shows the next month; the date stays.
	h.Fling(c, geom.Pt(c.X-80, c.Y), 100*time.Millisecond)
	rest(h)
	if got := title(h); got != "November 2026" {
		t.Fatalf("title %q after a flick to the left", got)
	}
	// A slow drag short of half way springs back.
	h.FingerDown(1, c)
	for i := 1; i <= 10; i++ {
		h.FingerMove(1, geom.Pt(c.X+float32(i)*8, c.Y))
		h.AdvanceTicks(3, gifttest.FrameInterval)
	}
	h.FingerUp(1, geom.Pt(c.X+80, c.Y))
	rest(h)
	if got := title(h); got != "November 2026" {
		t.Fatalf("title %q after a short slow drag", got)
	}
	h.Fling(c, geom.Pt(c.X+80, c.Y), 100*time.Millisecond)
	rest(h)
	if got := title(h); got != "October 2026" {
		t.Fatalf("title %q after a flick to the right", got)
	}
	if len(*chosen) != 0 {
		t.Fatalf("turning the month chose %v", *chosen)
	}
	// March 2027 is the last month: a flick past it bounces.
	for range 5 {
		h.Find(gifttest.ByKey("calendar-next")).Tap()
	}
	rest(h)
	if got := title(h); got != "March 2027" {
		t.Fatalf("title %q after five months on", got)
	}
	h.Fling(c, geom.Pt(c.X-120, c.Y), 100*time.Millisecond)
	rest(h)
	if got := title(h); got != "March 2027" {
		t.Fatalf("a flick past the range showed %q", got)
	}
	if !h.Find(gifttest.ByKey("calendar-next")).Interaction().Disabled {
		t.Error("the next arrow is enabled at the last month of the range")
	}
}

func TestTheKeysMoveTheDayAndTheWheelTurnsTheMonth(t *testing.T) {
	h, chosen := calendarScene(t, time.Monday)
	h.Find(gifttest.ByKey("ui.calendarCore")).Focus()
	h.Key(gift.KeyRight)
	h.Key(gift.KeyDown)
	if got := lastDate(*chosen); got != ui.DateOf(2026, time.October, 23) {
		t.Fatalf("right and down chose %v, want the 23rd", got)
	}
	h.Key(gift.KeyDown)
	h.Key(gift.KeyDown)
	rest(h)
	if got := lastDate(*chosen); got != ui.DateOf(2026, time.November, 6) {
		t.Fatalf("two weeks on chose %v, want the 6th of November", got)
	}
	if got := title(h); got != "November 2026" {
		t.Fatalf("the month did not follow the day: %q", got)
	}
	n := len(*chosen)
	h.Key(gift.KeyPageDown)
	rest(h)
	if got := title(h); got != "December 2026" || len(*chosen) != n {
		t.Fatalf("page down showed %q and chose %d dates", got, len(*chosen)-n)
	}
	c := centreOf(h.Find(gifttest.ByKey("ui.calendarCore")).LayoutBounds())
	h.Wheel(c, geom.Pt(0, 1))
	rest(h)
	if got := title(h); got != "November 2026" {
		t.Fatalf("the wheel pushed away showed %q", got)
	}
	h.Wheel(c, geom.Pt(0, -1))
	rest(h)
	if got := title(h); got != "December 2026" {
		t.Fatalf("the wheel pulled back showed %q", got)
	}
}

func TestATapOnTheTitleChoosesTheMonthOnWheels(t *testing.T) {
	h, chosen := calendarScene(t, time.Monday)
	h.TapAt(centreOf(h.Find(gifttest.ByKey("calendar-title")).LayoutBounds()))
	rest(h)
	h.AssertExists(gifttest.ByKey("calendar-year"))
	h.AssertNone(gifttest.ByKey("month-2026-10"))
	h.Find(gifttest.ByKey("calendar-month")).Focus()
	h.Key(gift.KeyDown)
	h.Key(gift.KeyDown)
	rest(h)
	if got := title(h); got != "December 2026" {
		t.Fatalf("two months down on the wheel showed %q", got)
	}
	h.TapAt(centreOf(h.Find(gifttest.ByKey("calendar-title")).LayoutBounds()))
	rest(h)
	h.AssertNone(gifttest.ByKey("calendar-year"))
	h.AssertExists(gifttest.ByKey("month-2026-12"))
	if len(*chosen) != 0 {
		t.Fatalf("the wheels chose %v", *chosen)
	}
}
