package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// The time field tests. Like the text field tests they drive the widget
// through gifttest — real keys, real runes, the real wheel and the real focus
// — and read the result back from the bound state, from the change callback
// and from the value the field reports to automation, which is the string a
// person sees.

// timeFixture is one bound time field and a button after it, so that tab has
// somewhere to go.
type timeFixture struct {
	h       *gifttest.Harness
	val     *gift.State[ui.TimeOfDay]
	changes []ui.TimeOfDay
	submits []ui.TimeOfDay
}

func newTimeField(t *testing.T, initial ui.TimeOfDay, decorate func(ui.TimeFieldView) ui.TimeFieldView) *timeFixture {
	t.Helper()
	restore := ui.SetEntryClockForTest(func() time.Time {
		return time.Date(2026, time.October, 26, 14, 37, 0, 0, time.UTC)
	})
	t.Cleanup(restore)
	f := &timeFixture{}
	root := func(ctx *gift.Context) gift.View {
		f.val = ctx.State("t", initial)
		ctx.Read(f.val)
		v := ui.TimeField().
			Bind(f.val.Binding()).
			Font(loadTestFont(t)).
			FontSize(16).
			Label("Start").
			OnChange(func(t ui.TimeOfDay) { f.changes = append(f.changes, t) }).
			OnSubmit(func(t ui.TimeOfDay) { f.submits = append(f.submits, t) }).
			Key("time")
		if decorate != nil {
			v = decorate(v)
		}
		return ui.ZStack(ui.Box(), ui.VStack(v, ui.Button(ui.Text("next"), func() {}).Key("next")).Padding(20))
	}
	f.h = gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 200), Font: loadTestFont(t)})
	return f
}

func (f *timeFixture) node() gifttest.Node { return f.h.Find(gifttest.ByKey("time")) }

// focus tabs onto the field, which puts the keyboard into the hour.
func (f *timeFixture) focus() *timeFixture { f.node().Focus(); return f }

func (f *timeFixture) want(t *testing.T, shown string, value ui.TimeOfDay) {
	t.Helper()
	f.node().AssertValue(shown)
	if got := f.val.Get(); got != value {
		t.Fatalf("the bound value is %q, want %q", got, value)
	}
}

// clickPart clicks into the hour (0) or the minute (1). The field is laid out
// as [hour][:][minute] from its left padding, so a quarter and three quarters
// of its width are inside the two parts whatever the font measures.
func (f *timeFixture) clickPart(i int) {
	b := f.node().Bounds()
	x := b.Min.X + b.Width()*0.3
	if i == 1 {
		x = b.Min.X + b.Width()*0.72
	}
	f.h.ClickAt(geom.Pt(x, b.Min.Y+b.Height()/2))
}

// --- typing ----------------------------------------------------------------------

func TestTypingFourDigitsEntersTheTime(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("0930")
	f.want(t, "09:30", ui.TimeOf(9, 30))
	// "0" and "9" fill the hour of a field whose minute is still empty, which
	// is no value; "3" makes 09:03 and "0" makes 09:30.
	if got := f.changes; len(got) != 2 || got[0] != ui.TimeOf(9, 3) || got[1] != ui.TimeOf(9, 30) {
		t.Fatalf("OnChange saw %v, want [09:03 09:30]", got)
	}
}

func TestADigitThatCannotStartAnHourMovesOnAtOnce(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("93")
	f.want(t, "09:03", ui.TimeOf(9, 3))
	// The minute is still waiting for its second digit.
	f.h.TypeText("5")
	f.want(t, "09:35", ui.TimeOf(9, 35))
}

func TestAOneWaitsForItsSecondDigit(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("1")
	f.node().AssertValue("01:--")
	f.h.TypeText("3")
	f.node().AssertValue("13:--")
	f.h.TypeText("4")
	f.want(t, "13:04", ui.TimeOf(13, 4))
}

func TestTheWaitEndsAfterThePause(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("1")
	f.h.Advance(ui.SegmentTypingPause + 100*time.Millisecond)
	f.h.TypeText("2")
	// A fresh "2" in the hour, not 12; it may still become 2x, so the hour
	// keeps the keyboard.
	f.node().AssertValue("02:--")
}

func TestATooLargeHourIsClampedToTheLargestOne(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("2900")
	f.want(t, "23:00", ui.TimeOf(23, 0))
}

func TestColonAndDotMoveOnToTheMinutes(t *testing.T) {
	for _, sep := range []string{":", "."} {
		t.Run(sep, func(t *testing.T) {
			f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
			f.h.TypeText("1" + sep + "5")
			f.node().AssertValue("01:05")
			f.h.TypeText("0")
			f.want(t, "01:50", ui.TimeOf(1, 50))
		})
	}
}

// TestASeparatorAfterAFullPartMovesNowhere types the time the way it is
// written: the 9 has already moved on to the minutes, and the colon after it
// must not move on a second time.
func TestASeparatorAfterAFullPartMovesNowhere(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("9:30")
	f.want(t, "09:30", ui.TimeOf(9, 30))
}

func TestBackspaceClearsThePartAndThenTheOneBefore(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 30), nil).focus()
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyBackspace)
	f.want(t, "09:--", ui.TimeOfDay{})
	if n := len(f.changes); n != 1 || !f.changes[0].IsZero() {
		t.Fatalf("OnChange saw %v, want one change to no time", f.changes)
	}
	f.h.Key(gift.KeyBackspace)
	f.want(t, "", ui.TimeOfDay{})
	// The emptied field still takes a new time from the hour on.
	f.h.TypeText("1745")
	f.want(t, "17:45", ui.TimeOf(17, 45))
}

func TestLettersAreSwallowed(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 30), nil).focus()
	f.h.TypeText("ab")
	f.want(t, "09:30", ui.TimeOf(9, 30))
}

// --- stepping --------------------------------------------------------------------

func TestUpAndDownStepThePartAndWrapAround(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(23, 59), nil).focus()
	f.h.Key(gift.KeyUp)
	f.want(t, "00:59", ui.TimeOf(0, 59))
	f.h.Key(gift.KeyDown)
	f.want(t, "23:59", ui.TimeOf(23, 59))
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp)
	// The minutes wrap on their own; the hour does not carry.
	f.want(t, "23:00", ui.TimeOf(23, 0))
	f.h.Key(gift.KeyDown)
	f.want(t, "23:59", ui.TimeOf(23, 59))
}

func TestTheStepAppliesToTheMinutes(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 7), func(v ui.TimeFieldView) ui.TimeFieldView {
		return v.Step(15 * time.Minute)
	}).focus()
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp)
	f.want(t, "09:15", ui.TimeOf(9, 15))
	f.h.Key(gift.KeyDown)
	f.want(t, "09:00", ui.TimeOf(9, 0))
	f.h.Key(gift.KeyDown)
	f.want(t, "09:45", ui.TimeOf(9, 45))
	// From an off-step minute the way down lands on the step below.
	f.h.TypeText("52")
	f.h.Key(gift.KeyDown)
	f.want(t, "09:45", ui.TimeOf(9, 45))
	// The hour still steps by one.
	f.h.Key(gift.KeyLeft)
	f.h.Key(gift.KeyUp)
	f.want(t, "10:45", ui.TimeOf(10, 45))
}

func TestShiftStepsTheHoursFromTheMinutes(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 30), nil).focus()
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyUp, gift.ModShift)
	f.want(t, "10:30", ui.TimeOf(10, 30))
	f.h.Key(gift.KeyDown, gift.ModShift)
	f.h.Key(gift.KeyDown, gift.ModShift)
	f.want(t, "08:30", ui.TimeOf(8, 30))
	// The keyboard is still in the minutes.
	f.h.Key(gift.KeyUp)
	f.want(t, "08:31", ui.TimeOf(8, 31))
}

func TestAnEmptyPartStartsAtTheCurrentTime(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, func(v ui.TimeFieldView) ui.TimeFieldView {
		return v.Step(5 * time.Minute)
	}).focus()
	f.h.Key(gift.KeyUp)
	f.node().AssertValue("14:--")
	f.h.Key(gift.KeyRight)
	f.h.Key(gift.KeyDown)
	// 14:37 on the step of five is 14:35.
	f.want(t, "14:35", ui.TimeOf(14, 35))
}

func TestTheWheelStepsThePartUnderThePointerWhileFocused(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 30), nil)
	b := f.node().Bounds()
	minute := geom.Pt(b.Min.X+b.Width()*0.72, b.Min.Y+b.Height()/2)
	hour := geom.Pt(b.Min.X+b.Width()*0.3, b.Min.Y+b.Height()/2)

	// Unfocused, the wheel belongs to whatever scrolls the form.
	f.h.Wheel(minute, geom.Pt(0, 1))
	f.want(t, "09:30", ui.TimeOf(9, 30))

	f.focus()
	f.h.Wheel(minute, geom.Pt(0, 1))
	f.want(t, "09:31", ui.TimeOf(9, 31))
	f.h.Wheel(hour, geom.Pt(0, -2))
	f.want(t, "07:31", ui.TimeOf(7, 31))
	// Half a notch is no step, and the other half completes it.
	f.h.Wheel(hour, geom.Pt(0, 0.5))
	f.want(t, "07:31", ui.TimeOf(7, 31))
	f.h.Wheel(hour, geom.Pt(0, 0.5))
	f.want(t, "08:31", ui.TimeOf(8, 31))
}

// --- moving and pointing ---------------------------------------------------------

func TestLeftRightAndClicksChooseThePart(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 30), nil)
	f.clickPart(1)
	f.node().AssertFocused()
	f.h.Key(gift.KeyUp)
	f.want(t, "09:31", ui.TimeOf(9, 31))
	f.h.Key(gift.KeyLeft)
	f.h.Key(gift.KeyUp)
	f.want(t, "10:31", ui.TimeOf(10, 31))
	f.clickPart(1)
	f.h.TypeText("05")
	f.want(t, "10:05", ui.TimeOf(10, 5))
	f.h.Key(gift.KeyHome)
	f.h.TypeText("11")
	f.want(t, "11:05", ui.TimeOf(11, 5))
}

func TestTabLeavesTheFieldAndEnterSubmits(t *testing.T) {
	f := newTimeField(t, ui.TimeOfDay{}, nil).focus()
	f.h.TypeText("1")
	f.h.Key(gift.KeyEnter)
	// Enter ends the lone "1" as one o'clock and submits what that is: the
	// minute is still empty, so no time.
	if len(f.submits) != 1 || !f.submits[0].IsZero() {
		t.Fatalf("OnSubmit saw %v, want one submit of no time", f.submits)
	}
	f.h.Key(gift.KeyRight)
	f.h.TypeText("15")
	f.h.Key(gift.KeyEnter)
	if len(f.submits) != 2 || f.submits[1] != ui.TimeOf(1, 15) {
		t.Fatalf("OnSubmit saw %v, want 01:15 second", f.submits)
	}
	f.h.Tab()
	f.h.AssertFocus(gifttest.ByKey("next"))
}

// --- the clipboard ---------------------------------------------------------------

func TestPasteTakesTheUsualSpellings(t *testing.T) {
	cb := &ui.MemoryClipboard{}
	ui.SetClipboard(cb)
	t.Cleanup(func() { ui.SetClipboard(nil) })
	for _, tc := range []struct {
		in   string
		want ui.TimeOfDay
	}{
		{"9:30", ui.TimeOf(9, 30)},
		{"09:30", ui.TimeOf(9, 30)},
		{"0930", ui.TimeOf(9, 30)},
		{"930", ui.TimeOf(9, 30)},
		{" 17.05 ", ui.TimeOf(17, 5)},
		{"8", ui.TimeOf(8, 0)},
	} {
		t.Run(tc.in, func(t *testing.T) {
			f := newTimeField(t, ui.TimeOf(1, 1), nil).focus()
			cb.SetText(tc.in)
			f.h.Key(gift.KeyV, gift.ShortcutModifier)
			f.want(t, tc.want.String(), tc.want)
		})
	}
	f := newTimeField(t, ui.TimeOf(1, 1), nil).focus()
	cb.SetText("half past nine")
	f.h.Key(gift.KeyV, gift.ShortcutModifier)
	f.want(t, "01:01", ui.TimeOf(1, 1))

	f.h.Key(gift.KeyC, gift.ShortcutModifier)
	if s, _ := cb.Text(); s != "01:01" {
		t.Fatalf("copy put %q on the clipboard, want 01:01", s)
	}
}

func TestParseTimeOfDay(t *testing.T) {
	for in, want := range map[string]string{"": "", "7": "07:00", "7:05": "07:05", "2359": "23:59"} {
		got, err := ui.ParseTimeOfDay(in)
		if err != nil || got.String() != want {
			t.Errorf("ParseTimeOfDay(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"24:00", "9:3", "12345", "9:60", "x"} {
		if _, err := ui.ParseTimeOfDay(in); err == nil {
			t.Errorf("ParseTimeOfDay(%q) did not fail", in)
		}
	}
}

// --- range and validity ----------------------------------------------------------

// dangerStroke reports whether the frame strokes anything in the danger
// colour, which is how an invalid field is drawn.
func dangerStroke(h *gifttest.Harness) bool {
	danger := ui.ResolveColor(ui.ColorDanger)
	for _, op := range h.Ops() {
		if op.Kind == render.OpStrokeRoundRect && op.Color == danger {
			return true
		}
	}
	return false
}

func TestATimeOutsideTheRangeIsKeptAndDrawnAsInvalid(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 0), func(v ui.TimeFieldView) ui.TimeFieldView {
		return v.Range(ui.TimeOf(8, 0), ui.TimeOf(18, 0))
	})
	if dangerStroke(f.h) {
		t.Fatal("a time inside the range is drawn as invalid")
	}
	f.focus()
	f.h.TypeText("19")
	f.want(t, "19:00", ui.TimeOf(19, 0))
	if !dangerStroke(f.h) {
		t.Fatal("19:00 after a range ending at 18:00 is not drawn as invalid")
	}
	f.h.Key(gift.KeyDown, gift.ModShift)
	if dangerStroke(f.h) {
		t.Fatal("18:00 is the end of the range and still drawn as invalid")
	}
}

func TestInvalidIsTheApplicationsToSay(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 0), func(v ui.TimeFieldView) ui.TimeFieldView { return v.Invalid(true) })
	if !dangerStroke(f.h) {
		t.Fatal("Invalid(true) is not drawn")
	}
}

// --- the value from outside -------------------------------------------------------

func TestAValueSetElsewhereReplacesTheDraft(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 0), nil).focus()
	f.h.Key(gift.KeyBackspace)
	f.node().AssertValue("--:00")
	f.val.Set(ui.TimeOf(12, 15))
	f.h.Settle()
	f.want(t, "12:15", ui.TimeOf(12, 15))
}

func TestAFieldWithoutAValueKeepsItsOwn(t *testing.T) {
	var got []ui.TimeOfDay
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.TimeField().Font(loadTestFont(t)).Key("free").
			OnChange(func(t ui.TimeOfDay) { got = append(got, t) })).Padding(20),
		Size: geom.Sz(300, 100),
		Font: loadTestFont(t),
	})
	n := h.Find(gifttest.ByKey("free"))
	n.Focus()
	h.TypeText("0815")
	n.AssertValue("08:15")
	if len(got) == 0 || got[len(got)-1] != ui.TimeOf(8, 15) {
		t.Fatalf("OnChange saw %v, want 08:15 last", got)
	}
}

func TestTheFieldIsNamedAndDisabledLikeAnyControl(t *testing.T) {
	f := newTimeField(t, ui.TimeOf(9, 0), nil)
	f.h.Find(gifttest.ByText("Start")).AssertType("ui.TimeField")
	f = newTimeField(t, ui.TimeOf(9, 0), func(v ui.TimeFieldView) ui.TimeFieldView { return v.Disabled(true) })
	f.node().AssertDisabled()
	f.h.Tab()
	f.h.AssertFocus(gifttest.ByKey("next"))
}
