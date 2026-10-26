package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The date field tests. The date field is the time field's machinery with
// three parts, so these tests are about what is particular to a date: the day
// kept within its month, the year of four digits and of two, and the
// spellings a date is pasted in. Typing, the focus and the wheel are the time
// field's and are tested there.

type dateFixture struct {
	h       *gifttest.Harness
	val     *gift.State[ui.Date]
	changes []ui.Date
}

func newDateField(t *testing.T, initial ui.Date, decorate func(ui.DateFieldView) ui.DateFieldView) *dateFixture {
	t.Helper()
	restore := ui.SetEntryClockForTest(func() time.Time {
		return time.Date(2026, time.October, 26, 14, 37, 0, 0, time.UTC)
	})
	t.Cleanup(restore)
	f := &dateFixture{}
	root := func(ctx *gift.Context) gift.View {
		f.val = ctx.State("d", initial)
		ctx.Read(f.val)
		v := ui.DateField().
			Bind(f.val.Binding()).
			Font(loadTestFont(t)).
			FontSize(16).
			OnChange(func(d ui.Date) { f.changes = append(f.changes, d) }).
			Key("date")
		if decorate != nil {
			v = decorate(v)
		}
		return ui.VStack(v, ui.Button(ui.Text("next"), func() {}).Key("next")).Padding(20)
	}
	f.h = gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 200), Font: loadTestFont(t)})
	return f
}

func (f *dateFixture) node() gifttest.Node { return f.h.Find(gifttest.ByKey("date")) }

func (f *dateFixture) want(t *testing.T, shown string, value ui.Date) {
	t.Helper()
	f.node().AssertValue(shown)
	if got := f.val.Get(); got != value {
		t.Fatalf("the bound value is %q, want %q", got, value)
	}
}

func TestTypingADateFillsDayMonthAndYear(t *testing.T) {
	f := newDateField(t, ui.Date{}, nil)
	f.node().Focus()
	f.h.TypeText("26102026")
	f.want(t, "26.10.2026", ui.DateOf(2026, time.October, 26))
	// The year on its way, 2, 20 and 202, is no date; the first value is the
	// finished one.
	if len(f.changes) != 1 {
		t.Fatalf("OnChange saw %v, want one change", f.changes)
	}
}

func TestShortPartsAndSeparators(t *testing.T) {
	f := newDateField(t, ui.Date{}, nil)
	f.node().Focus()
	// "5" cannot start a two digit day and moves on by itself, so the dot
	// after it moves nowhere; "3" does the same for the month, and "26" is
	// the start of a year.
	f.h.TypeText("5.3.26")
	f.node().AssertValue("05.03.26")
	if !f.val.Get().IsZero() {
		t.Fatalf("a year still being typed is already the value %v", f.val.Get())
	}
	// Leaving the year puts a two digit one into this century.
	f.h.Key(gift.KeyLeft)
	f.want(t, "05.03.2026", ui.DateOf(2026, time.March, 5))
}

func TestTheDayIsKeptWithinTheMonth(t *testing.T) {
	f := newDateField(t, ui.DateOf(2026, time.January, 31), nil)
	f.node().Focus()
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp)
	f.want(t, "28.02.2026", ui.DateOf(2026, time.February, 28))

	f = newDateField(t, ui.Date{}, nil)
	f.node().Focus()
	// Before the year is known the 29th of February is allowed, and the year
	// then decides.
	f.h.TypeText("2902")
	f.node().AssertValue("29.02.----")
	f.h.TypeText("2025")
	f.want(t, "28.02.2025", ui.DateOf(2025, time.February, 28))
	f.h.Key(gift.KeyUp)
	f.h.Key(gift.KeyLeft)
	f.h.Key(gift.KeyLeft)
	// Typed, the day cannot go past the month either.
	f.h.TypeText("29")
	f.want(t, "28.02.2026", ui.DateOf(2026, time.February, 28))
}

func TestDayAndMonthWrapAndTheYearStops(t *testing.T) {
	f := newDateField(t, ui.DateOf(2026, time.December, 31), nil)
	f.node().Focus()
	f.h.Key(gift.KeyUp)
	f.want(t, "01.12.2026", ui.DateOf(2026, time.December, 1))
	f.h.Key(gift.KeyDown)
	f.want(t, "31.12.2026", ui.DateOf(2026, time.December, 31))
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp)
	f.want(t, "31.01.2026", ui.DateOf(2026, time.January, 31))
	f.h.Key(gift.KeyEnd)
	f.h.TypeText("9999")
	f.h.Key(gift.KeyUp)
	f.want(t, "31.01.9999", ui.DateOf(9999, time.January, 31))
}

func TestAnEmptyDateStartsAtToday(t *testing.T) {
	f := newDateField(t, ui.Date{}, nil)
	f.node().Focus()
	f.h.Key(gift.KeyUp)
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyDown)
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp)
	f.want(t, "26.10.2026", ui.DateOf(2026, time.October, 26))
}

func TestPasteTakesTheUsualDateSpellings(t *testing.T) {
	cb := &ui.MemoryClipboard{}
	ui.SetClipboard(cb)
	t.Cleanup(func() { ui.SetClipboard(nil) })
	want := ui.DateOf(2026, time.October, 26)
	for _, in := range []string{"26.10.2026", "26.10.26", "2026-10-26", "26/10/2026", "26102026", "261026"} {
		t.Run(in, func(t *testing.T) {
			f := newDateField(t, ui.DateOf(2000, time.January, 1), nil)
			f.node().Focus()
			cb.SetText(in)
			f.h.Key(gift.KeyV, gift.ShortcutModifier)
			f.want(t, "26.10.2026", want)
		})
	}
	f := newDateField(t, ui.DateOf(2000, time.January, 1), nil)
	f.node().Focus()
	cb.SetText("31.02.2026")
	f.h.Key(gift.KeyV, gift.ShortcutModifier)
	f.want(t, "01.01.2000", ui.DateOf(2000, time.January, 1))
	f.h.Key(gift.KeyC, gift.ShortcutModifier)
	if s, _ := cb.Text(); s != "01.01.2000" {
		t.Fatalf("copy put %q on the clipboard", s)
	}
}

func TestADateOutsideTheRangeIsDrawnAsInvalid(t *testing.T) {
	f := newDateField(t, ui.DateOf(2026, time.October, 26), func(v ui.DateFieldView) ui.DateFieldView {
		return v.Range(ui.DateOf(2026, time.January, 1), ui.DateOf(2026, time.December, 31))
	})
	if dangerStroke(f.h) {
		t.Fatal("a date inside the range is drawn as invalid")
	}
	f.node().Focus()
	f.h.Key(gift.KeyEnd)
	f.h.Key(gift.KeyUp)
	f.want(t, "26.10.2027", ui.DateOf(2027, time.October, 26))
	if !dangerStroke(f.h) {
		t.Fatal("a date after the range is not drawn as invalid")
	}
}

func TestParseDateAndTheDateValue(t *testing.T) {
	d, err := ui.ParseDate("29.02.2024")
	if err != nil || d != ui.DateOf(2024, time.February, 29) {
		t.Fatalf("ParseDate = %v, %v", d, err)
	}
	if got := d.Time(time.UTC); !got.Equal(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("Time = %v", got)
	}
	if ui.DateFrom(time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC)) != ui.DateOf(2026, time.October, 26) {
		t.Fatal("DateFrom lost the day")
	}
	for _, in := range []string{"29.02.2023", "1.1", "32.01.2026", "2026-13-01", "x"} {
		if _, err := ui.ParseDate(in); err == nil {
			t.Errorf("ParseDate(%q) did not fail", in)
		}
	}
	if (ui.Date{}).String() != "" || !(ui.Date{}).IsZero() {
		t.Fatal("the zero Date is not empty")
	}
}
