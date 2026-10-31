package gift_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	presenterType = gift.RegisterType("test.Presenter")
	sheetType     = gift.RegisterType("test.OverlaySheet")
)

// presenter is a 40 by 20 node that presents an overlaySheet while open.
type presenter struct {
	open  bool
	exit  time.Duration
	sheet overlaySheet
}

func (presenter) ViewType() gift.TypeID { return presenterType }

func (p presenter) Build(*gift.BuildContext) gift.Element {
	e := gift.Element{Key: "presenter", Layouter: fixed{40, 20}, OverlayExit: p.exit}
	if p.open {
		e.Overlay = p.sheet
	}
	return e
}

type fixed struct{ w, h float32 }

func (f fixed) Layout(*gift.LayoutContext, geom.Constraints) geom.Size { return geom.Sz(f.w, f.h) }

// overlaySheet fills the window, records where its presenter is, paints a
// marker and counts the presses it gets.
type overlaySheet struct {
	seen    *geom.Rect
	presses *int
}

func (overlaySheet) ViewType() gift.TypeID { return sheetType }

func (s overlaySheet) Build(*gift.BuildContext) gift.Element {
	n := &sheetNode{s: s}
	return gift.Element{Key: "sheet", Layouter: n, Painter: n, Interactor: n}
}

type sheetNode struct{ s overlaySheet }

func (n *sheetNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if r, ok := ctx.Presenter(); ok {
		*n.s.seen = r
	}
	return c.Constrain(c.Max)
}

func (n *sheetNode) Paint(ctx *gift.PaintContext) {
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(0, 0, 10, 10), Color: render.RGB(1, 2, 3)})
}

func (n *sheetNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind == gift.EventPointerDown {
		*n.s.presses++
	}
	return true
}

// marker reports whether the overlay's marker is in the list, and whether it
// is the last operation, painted over everything.
func marker(a *gift.App) (present, last bool) {
	l := a.Paint()
	ops := l.Ops()
	for i, op := range ops {
		if op.Color == render.RGB(1, 2, 3) {
			return true, i == len(ops)-1
		}
	}
	return false, false
}

func TestAnOverlayEscapesItsClippingAncestorAndIsOnTop(t *testing.T) {
	var seen geom.Rect
	presses, behind := 0, 0
	open := true
	show := true
	root := func(*gift.Context) gift.View {
		kids := []gift.View{target{name: "behind", w: 300, h: 300, activations: &behind}}
		if show {
			// A presenter at (100, 100) in a clipped 50 by 50 box.
			kids = append(kids, frameView{key: "clip", w: 50, h: 50, clip: true,
				children: []gift.View{presenter{open: open, sheet: overlaySheet{seen: &seen, presses: &presses}}}})
		}
		return frameView{w: 400, h: 400, offsets: []geom.Point{at(0, 0), at(100, 100)}, children: kids}
	}
	a := newInputApp(t, root)
	live := a.Diagnostics().LiveNodes

	if want := geom.Rc(100, 100, 140, 120); seen != want {
		t.Errorf("Presenter() = %v, want %v", seen, want)
	}
	if present, last := marker(a); !present || !last {
		t.Fatalf("the overlay is painted %v, last %v; want painted over everything", present, last)
	}
	// Far outside the clip, over the button behind: the overlay answers.
	click(a, at(250, 250))
	if presses != 1 || behind != 0 {
		t.Fatalf("presses %d, behind %d: the overlay must take the tap", presses, behind)
	}

	// Withdrawn: gone at once, and so is the top layer.
	open = false
	a.InvalidateAll()
	mustUpdate(t, a)
	if present, _ := marker(a); present {
		t.Error("a withdrawn overlay without an exit time is still painted")
	}
	click(a, at(250, 250))
	if behind != 1 {
		t.Error("the tap did not reach the button once the overlay was gone")
	}
	open = true
	a.InvalidateAll()
	mustUpdate(t, a)
	// The presenter unmounted: its overlay goes with it.
	show = false
	a.InvalidateAll()
	mustUpdate(t, a)
	if present, _ := marker(a); present {
		t.Error("the overlay of an unmounted presenter is still painted")
	}
	if got := a.Diagnostics().LiveNodes; got >= live {
		t.Errorf("LiveNodes = %d after closing, want fewer than the %d with the overlay open", got, live)
	}
}

func TestAnOverlayLeavesOverItsExitTimeWithoutInput(t *testing.T) {
	var seen geom.Rect
	presses := 0
	open := true
	root := func(*gift.Context) gift.View {
		return frameView{w: 400, h: 400, offsets: []geom.Point{at(10, 10)},
			children: []gift.View{presenter{open: open, exit: 200 * time.Millisecond, sheet: overlaySheet{seen: &seen, presses: &presses}}}}
	}
	a := newInputApp(t, root)
	open = false
	a.InvalidateAll()
	a.BeginInput(time.Second)
	mustUpdate(t, a)
	if present, _ := marker(a); !present {
		t.Fatal("a leaving overlay is not painted any more")
	}
	a.BeginInput(time.Second + 16*time.Millisecond)
	if !a.NeedsPaint() {
		t.Error("nothing keeps a leaving overlay animating")
	}
	click(a, at(300, 300))
	if presses != 0 {
		t.Error("a leaving overlay took a tap")
	}
	a.BeginInput(time.Second + 250*time.Millisecond)
	mustUpdate(t, a)
	if present, _ := marker(a); present {
		t.Error("the overlay is still painted after its exit time")
	}
}

func TestASecondaryClickAsksForAContextMenuAndBubbles(t *testing.T) {
	var events []gift.Event
	root := func(*gift.Context) gift.View {
		return frameView{w: 300, h: 300, offsets: []geom.Point{at(0, 0)},
			children: []gift.View{target{name: "t", w: 100, h: 100, events: &events}}}
	}
	a := newInputApp(t, root)
	a.BeginInput(0)
	a.ContextMenuAt(at(20, 30))
	found := false
	for _, e := range events {
		if e.Kind == gift.EventContextMenu {
			found = e.Pos == at(20, 30) && e.Device == gift.PointerMouse
		}
	}
	if !found {
		t.Fatalf("no context menu event at (20, 30): %+v", events)
	}
}
