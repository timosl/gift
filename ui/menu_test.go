package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func centreOf(r geom.Rect) geom.Point { return geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2) }

// popoverScene is a text deep in a clipped scroll view, with a popover.
func popoverScene(t *testing.T, top, height float32) (*gifttest.Harness, *int) {
	t.Helper()
	dismissed := new(int)
	root := func(ctx *gift.Context) gift.View {
		open := ctx.State("open", true)
		return ui.VScroll(
			ui.Box().Frame(10, top),
			ui.Popover(
				ui.Text("Anchor").Key("anchor"),
				ui.Text("Inside the popover").Key("content"),
			).Presented(ctx.Read(open)).OnDismiss(func() { *dismissed++; open.Set(false) }),
			ui.Box().Frame(10, 2000),
		).Frame(400, 300).Key("scroll")
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(600, height), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, dismissed
}

func TestAPopoverEscapesItsScrollViewAndPointsAtItsAnchor(t *testing.T) {
	h, dismissed := popoverScene(t, 40, 500)
	anchor := h.Find(gifttest.ByKey("anchor")).LayoutBounds()
	content := h.Find(gifttest.ByKey("content")).LayoutBounds()
	if content.Min.Y < anchor.Max.Y {
		t.Fatalf("the content at %v is not below its anchor at %v", content, anchor)
	}
	// The scroll view is 300 tall; a popover is drawn in the top layer and is
	// therefore not cut by it even where it reaches past it.
	if !content.Overlaps(geom.Rc(0, 0, 600, 500)) {
		t.Fatalf("the popover content at %v is off the window", content)
	}
	// A tap on the content keeps it open; a tap outside closes it, and goes
	// nowhere else.
	h.TapAt(centreOf(content))
	if *dismissed != 0 {
		t.Fatal("a tap inside the popover dismissed it")
	}
	h.TapAt(geom.Pt(550, 450))
	if *dismissed != 1 {
		t.Fatalf("a tap outside dismissed %d times, want once", *dismissed)
	}
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("content"))
}

func TestAPopoverNearTheBottomOpensAbove(t *testing.T) {
	// A window that ends just below the anchor.
	h, dismissed := popoverScene(t, 270, 320)
	anchor := h.Find(gifttest.ByKey("anchor")).LayoutBounds()
	content := h.Find(gifttest.ByKey("content")).LayoutBounds()
	if content.Max.Y > anchor.Min.Y {
		t.Fatalf("the content at %v is not above its anchor at %v", content, anchor)
	}
	h.Key(gift.KeyEscape)
	if *dismissed != 1 {
		t.Error("escape did not dismiss the popover")
	}
}

// menuScene is a menu of four items, one checked, one destructive and one
// disabled, and records what was chosen.
func menuScene(t *testing.T) (*gifttest.Harness, *[]string) {
	t.Helper()
	chosen := new([]string)
	pick := func(s string) func() { return func() { *chosen = append(*chosen, s) } }
	h := gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.Menu(ui.Text("More").Key("more").Padding(10),
			ui.MenuItem("Copy", pick("copy")).Checked(true),
			ui.MenuItem("Archive", pick("archive")).Disabled(true),
			ui.MenuSection("Danger",
				ui.MenuItem("Delete", pick("delete")).Destructive(),
			),
			ui.MenuDivider(),
			ui.MenuItem("Share", pick("share")),
		).Key("menu")).Align(geom.TopLeading).Padding(20),
		Size:  geom.Sz(600, 500),
		Font:  loadTestFont(t),
		Theme: ui.LightTheme(),
	})
	h.Settle()
	return h, chosen
}

func TestATapOpensTheMenuAndATapOnAnItemChoosesIt(t *testing.T) {
	h, chosen := menuScene(t)
	h.AssertNone(gifttest.ByKey("menu-Share"))
	h.Find(gifttest.ByKey("more")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	more := h.Find(gifttest.ByKey("more")).LayoutBounds()
	share := h.Find(gifttest.ByKey("menu-Share")).LayoutBounds()
	if share.Min.Y < more.Max.Y || share.Min.X < more.Min.X-1 {
		t.Errorf("the menu at %v is not below and lined up with its button at %v", share, more)
	}
	if share.Width() < 249 || share.Height() != 44 {
		t.Errorf("an item is %v, want 250 by 44", share.Size())
	}
	// A disabled item refuses the tap and leaves the menu open.
	h.Find(gifttest.ByKey("menu-Archive")).Tap()
	if len(*chosen) != 0 {
		t.Fatalf("a disabled item was chosen: %v", *chosen)
	}
	h.Find(gifttest.ByKey("menu-Delete")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	if len(*chosen) != 1 || (*chosen)[0] != "delete" {
		t.Fatalf("chosen %v, want delete", *chosen)
	}
	h.AssertNone(gifttest.ByKey("menu-Share"))
}

// TestAFingerThatSlidesOntoAnItemChoosesItWithoutASecondTap pins the gesture
// of iOS: press the button, slide, lift.
func TestAFingerThatSlidesOntoAnItemChoosesItWithoutASecondTap(t *testing.T) {
	h, chosen := menuScene(t)
	at := centreOf(h.Find(gifttest.ByKey("more")).LayoutBounds())
	h.FingerDown(1, at)
	h.AdvanceTicks(3, gifttest.FrameInterval)
	share := centreOf(h.Find(gifttest.ByKey("menu-Share")).LayoutBounds())
	h.FingerMove(1, geom.Pt(at.X, at.Y+20))
	h.FingerMove(1, share)
	h.Frame()
	h.FingerUp(1, share)
	h.AdvanceTicks(20, gifttest.FrameInterval)
	if len(*chosen) != 1 || (*chosen)[0] != "share" {
		t.Fatalf("chosen %v, want share", *chosen)
	}
	h.AssertNone(gifttest.ByKey("menu-Share"))
}

func TestTheKeysWalkAMenuAndChoose(t *testing.T) {
	h, chosen := menuScene(t)
	h.Find(gifttest.ByKey("more")).Focus()
	h.Key(gift.KeyEnter)
	h.AdvanceTicks(5, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Copy"))
	// Copy is highlighted; down skips the disabled Archive to Delete.
	h.Key(gift.KeyDown)
	h.Key(gift.KeyEnter)
	h.AdvanceTicks(20, gifttest.FrameInterval)
	if len(*chosen) != 1 || (*chosen)[0] != "delete" {
		t.Fatalf("chosen %v, want delete", *chosen)
	}
	h.Find(gifttest.ByKey("more")).Focus()
	h.Key(gift.KeyEnter)
	h.AdvanceTicks(5, gifttest.FrameInterval)
	h.Key(gift.KeyEscape)
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
}

func TestALongPressOpensAContextMenuAndTakesTheFingerFromTheContent(t *testing.T) {
	taps := 0
	chosen := ""
	h := gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.ContextMenu(
			ui.Button(ui.Text("Photo").Key("photo"), func() { taps++ }).Key("tile"),
			ui.MenuItem("Copy", func() { chosen = "copy" }),
			ui.MenuItem("Delete", func() { chosen = "delete" }).Destructive(),
		).Key("ctx")).Align(geom.Alignment{X: 0.5, Y: 0.3}),
		Size:  geom.Sz(600, 500),
		Font:  loadTestFont(t),
		Theme: ui.LightTheme(),
	})
	h.Settle()
	// A short tap is the content's.
	h.Find(gifttest.ByKey("photo")).Tap()
	if taps != 1 {
		t.Fatalf("the tap reached the content %d times", taps)
	}
	at := centreOf(h.Find(gifttest.ByKey("photo")).LayoutBounds())
	h.FingerDown(1, at)
	h.AdvanceTicks(40, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Delete"))
	h.FingerUp(1, at)
	h.Settle()
	if taps != 1 {
		t.Error("the release of the long press counted as a tap on the content")
	}
	h.Find(gifttest.ByKey("menu-Copy")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	if chosen != "copy" {
		t.Fatalf("chosen %q, want copy", chosen)
	}
	// A secondary click opens it at the pointer: the menu starts there.
	click := geom.Pt(at.X+4, at.Y+3)
	h.App().BeginInput(h.Now())
	h.App().ContextMenuAt(click)
	h.AdvanceTicks(20, gifttest.FrameInterval)
	copyRow := h.Find(gifttest.ByKey("menu-Copy")).LayoutBounds()
	if d := copyRow.Min.Sub(click); d.X < -1 || d.X > 1 || d.Y < 0 || d.Y > 12 {
		t.Errorf("the menu of a secondary click at %v starts at %v", click, copyRow.Min)
	}
}

func TestAClosedMenuCostsNoNode(t *testing.T) {
	h, _ := menuScene(t)
	before := h.Diagnostics().LiveNodes
	h.Find(gifttest.ByKey("more")).Tap()
	h.AdvanceTicks(5, gifttest.FrameInterval)
	if h.Diagnostics().LiveNodes <= before {
		t.Fatal("an open menu added no nodes")
	}
	h.TapAt(geom.Pt(590, 490))
	h.AdvanceTicks(30, gifttest.FrameInterval)
	if got := h.Diagnostics().LiveNodes; got != before {
		t.Errorf("LiveNodes = %d after closing, want %d", got, before)
	}
}
