package main

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/internal/showcase"
	"github.com/worldiety/gift/ui"
)

func scene(t *testing.T, w, h float32) *gifttest.Harness {
	t.Helper()
	if err := showcase.Samples(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	initialDemo = "button"
	hr := gifttest.New(t, gifttest.Options{
		Root: Screen, Size: geom.Sz(w, h),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme(),
	})
	App = hr.App()
	t.Cleanup(func() { App = nil; ui.SetOnScreenKeyboard(nil, false) })
	hr.AdvanceTicks(4, gifttest.FrameInterval)
	return hr
}

func TestEveryComponentIsListedAndShowsItsDemo(t *testing.T) {
	h := scene(t, 1180, 800)
	for _, d := range showcase.All() {
		h.AssertExists(gifttest.ByKey("row-" + d.Name))
	}
	h.AssertExists(gifttest.ByKey("stage-button"))
	h.Find(gifttest.ByKey("list")).ScrollTo(0)
	h.Find(gifttest.ByKey("row-color-picker")).Tap()
	h.AdvanceTicks(4, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("stage-button"))
	h.AssertExists(gifttest.ByKey("color-grid"))
	if b := h.Find(gifttest.ByKey("row-color-picker")).LayoutBounds(); b.Max.X > sidebarWidth {
		t.Fatalf("the list reaches %v, past the sidebar", b.Max.X)
	}
}

func TestTheSearchFiltersTheList(t *testing.T) {
	h := scene(t, 1180, 800)
	h.Find(gifttest.ByKey("search-field")).Focus()
	h.TypeText("slider")
	h.AdvanceTicks(4, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("row-slider"))
	h.AssertExists(gifttest.ByKey("row-circular-slider"))
	h.AssertNone(gifttest.ByKey("row-button"))
	h.AssertNone(gifttest.ByKey("row-calendar"))
}

func TestANarrowWindowPushesTheComponentAndComesBack(t *testing.T) {
	h := scene(t, 600, 800)
	h.AdvanceTicks(4, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("stage-button"))
	h.Find(gifttest.ByKey("row-calendar")).Tap()
	h.AdvanceTicks(40, gifttest.FrameInterval)
	stage := h.Find(gifttest.ByKey("stage-calendar"))
	if !stage.IsVisible() {
		t.Fatal("the calendar was not pushed")
	}
	h.Key(gift.KeyEscape)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("stage-calendar"))
	if !h.Find(gifttest.ByKey("row-calendar")).IsVisible() {
		t.Fatal("back did not show the list")
	}
}

func TestTheSwitchesTurnTheThemeAndTheKeyboard(t *testing.T) {
	h := scene(t, 1180, 800)
	h.Find(gifttest.ByKey("theme-switch")).Tap()
	h.AdvanceTicks(2, gifttest.FrameInterval)
	if !ui.CurrentTheme().IsDark() {
		t.Fatal("the theme is not dark after the switch")
	}
	h.Find(gifttest.ByKey("theme-switch")).Tap()
	h.Find(gifttest.ByKey("keyboard-switch")).Tap()
	if !ui.OnScreenKeyboardEnabled() {
		t.Fatal("the keyboard switch did not turn the on-screen keyboard on")
	}
}
