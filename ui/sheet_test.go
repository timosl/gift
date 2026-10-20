package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// sheetApp mounts a sheet over a tappable background, with the sheet's
// presentation in a state slot the test can write. The sheet's content is
// marker 1 behind a button under the key "inside".
func sheetApp(t testing.TB, size geom.Size, dismissed, behind *int) (*gifttest.Harness, *gift.State[bool]) {
	t.Helper()
	var open *gift.State[bool]
	h := gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(), Font: loadTestFont(t), Size: size,
		Root: func(ctx *gift.Context) gift.View {
			open = ctx.State("open", false)
			content := ui.Button(ui.Text("behind"), func() { *behind++ }).Flex(1).Key("behind")
			sheet := ui.ZStack(
				ui.Box().Background(markerColor(1)),
				ui.Button(ui.Text("inside"), nil).Key("inside"),
			)
			return ui.Sheet(ui.VStack(content), sheet).
				Presented(ctx.Read(open)).
				OnDismiss(func() { *dismissed++; open.Set(false) })
		},
	})
	return h, open
}

// TestASheetSlidesUpFromBelowTheWindowAndLeavesTheSameWay: it starts wholly
// below the bottom edge, rises, comes to rest with its bottom on that edge,
// and on dismissal sinks again and is gone once it has.
func TestASheetSlidesUpFromBelowTheWindowAndLeavesTheSameWay(t *testing.T) {
	var dismissed, behind int
	size := geom.Sz(400, 500)
	h, open := sheetApp(t, size, &dismissed, &behind)
	if _, ok := markerRect(t, h, 1); ok {
		t.Fatal("a sheet that is not presented is drawn")
	}

	open.Set(true)
	h.Settle()
	first := mustMarkerRect(t, h, 1)
	if first.Min.Y < size.H-1 {
		t.Errorf("the first frame of an opening sheet has its top at y=%v; it has to start "+
			"below the window edge at %v", first.Min.Y, size.H)
	}
	h.Advance(90 * time.Millisecond)
	mid := mustMarkerRect(t, h, 1)
	if !(mid.Min.Y < first.Min.Y) {
		t.Errorf("halfway through the opening the sheet is at y=%v, having started at %v",
			mid.Min.Y, first.Min.Y)
	}
	h.Advance(ui.ControlAnimation)
	up := mustMarkerRect(t, h, 1)
	if want := size.H * 0.1; up.Min.Y != want || up.Max.Y != size.H {
		t.Errorf("the opened sheet spans y=%v..%v, want %v..%v", up.Min.Y, up.Max.Y, want, size.H)
	}
	if up.Min.X != 0 || up.Max.X != size.W {
		t.Errorf("a sheet in a window narrower than its maximum spans x=%v..%v, want the "+
			"whole width", up.Min.X, up.Max.X)
	}

	open.Set(false)
	h.Settle()
	h.Advance(60 * time.Millisecond)
	leaving, ok := markerRect(t, h, 1)
	if !ok {
		t.Fatal("the dismissed sheet was gone 60 ms into its exit; the dismissal is a cut")
	}
	if !(leaving.Min.Y > up.Min.Y) {
		t.Errorf("the leaving sheet is at y=%v, having been at %v; it is not sinking",
			leaving.Min.Y, up.Min.Y)
	}
	h.Advance(ui.ControlAnimation)
	if _, ok := markerRect(t, h, 1); ok {
		t.Error("the dismissed sheet is still painted a transition after it left")
	}
	if h.Diagnostics().Animating {
		t.Error("the application never went idle after the sheet was dismissed")
	}
}

func TestASheetIsCentredAndNoWiderThanItsMaximum(t *testing.T) {
	var dismissed, behind int
	h, open := sheetApp(t, geom.Sz(1000, 600), &dismissed, &behind)
	open.Set(true)
	h.Settle()
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	r := mustMarkerRect(t, h, 1)
	if r.Width() != 720 || r.Min.X != 140 {
		t.Fatalf("the sheet spans x=%v..%v in a 1000 wide window, want 140..860", r.Min.X, r.Max.X)
	}
}

// TestTheScrimAndEscapeDismissASheetAndATapOnItDoesNot covers the three ways
// a pointer or a key can meet an open sheet.
func TestTheScrimAndEscapeDismissASheetAndATapOnItDoesNot(t *testing.T) {
	var dismissed, behind int
	h, open := sheetApp(t, geom.Sz(400, 500), &dismissed, &behind)
	open.Set(true)
	h.Settle()
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)

	// On the panel, away from the button: nothing at all.
	h.TapAt(geom.Pt(20, 480))
	h.Settle()
	if dismissed != 0 || behind != 0 {
		t.Fatalf("a tap on the panel dismissed %d times and reached the content %d times",
			dismissed, behind)
	}

	// On the scrim above the panel: a dismissal, and nothing underneath.
	h.TapAt(geom.Pt(200, 20))
	h.Settle()
	if dismissed != 1 || behind != 0 {
		t.Fatalf("a tap on the scrim dismissed %d times and reached the content %d times",
			dismissed, behind)
	}

	// Escape with nothing focused, which is the touchscreen case.
	open.Set(true)
	h.Settle()
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	h.AssertNoFocus()
	h.Key(gift.KeyEscape)
	h.Settle()
	if dismissed != 2 {
		t.Fatalf("escape with nothing focused dismissed %d times in all, want 2", dismissed)
	}

	// Escape from a control inside the sheet.
	open.Set(true)
	h.Settle()
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	h.Find(gifttest.ByKey("inside")).Focus()
	h.Key(gift.KeyEscape)
	h.Settle()
	if dismissed != 3 {
		t.Fatalf("escape from a button in the sheet dismissed %d times in all, want 3", dismissed)
	}
}

func TestTabDoesNotLeaveAnOpenSheet(t *testing.T) {
	var dismissed, behind int
	h, open := sheetApp(t, geom.Sz(400, 500), &dismissed, &behind)
	open.Set(true)
	h.Settle()
	inside := h.Find(gifttest.ByKey("inside")).Ref()
	for range 6 {
		h.App().MoveFocus(true)
		h.Settle()
		if f, ok := h.App().Focus(); ok && f != inside {
			t.Fatalf("tab left the sheet and landed on %q", h.At(center(h.App().NodeDeviceBounds(f))).Text())
		}
	}
}

// TestAFieldAtTheBottomOfASheetStaysAboveTheKeyboard is the composition
// [ui.SheetView] documents: inside ui.Window, the sheet sits on the keyboard.
func TestAFieldAtTheBottomOfASheetStaysAboveTheKeyboard(t *testing.T) {
	font := loadTestFont(t)
	ed := ui.NewTextEditor("")
	sheet := ui.VStack(
		ui.Text("Name").Font(font),
		ui.Spacer(),
		ui.TextField(ed).Font(font).FontSize(16).Key("field"),
	).Padding(12).Key("sheet")

	h := newAvoiding(t, ui.Window(
		ui.Sheet(ui.Text("behind").Font(font), sheet),
		ui.OnScreenKeyboard().Key("kb"),
	).Align(geom.Bottom))
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)

	h.Find(gifttest.ByKey("field")).Focus()
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	kb := h.Find(gifttest.ByKey("kb")).Bounds()
	fb := h.Find(gifttest.ByKey("field")).Bounds()
	if !(kb.Height() > 0) {
		t.Fatal("no keyboard, so there is nothing this test can show")
	}
	if fb.Max.Y > kb.Min.Y || fb.Min.Y < 0 {
		t.Fatalf("the field is %v, the keyboard starts at y=%v", fb, kb.Min.Y)
	}
}

func TestAClosedSheetCostsNothing(t *testing.T) {
	h := navHarness(t, ui.Sheet(ui.Text("content"), nil))
	h.AssertNone(gifttest.ByType("ui.scrim"))
	h.AssertNone(gifttest.ByType("ui.layer"))
	h.Find(gifttest.ByText("content")).AssertVisible()
}
