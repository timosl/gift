package gift_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var counterType = gift.RegisterType("test.RetainedCounter")

type taps struct{ n int }

// counter counts presses in its retained state, and its label in a build
// argument, so that a rebuild can change the view without touching the node.
type counter struct {
	label string
	seen  *int
}

func (counter) ViewType() gift.TypeID { return counterType }

func (c counter) Build(*gift.BuildContext) gift.Element {
	n := &counterNode{c: c}
	return gift.Element{Key: "counter", Layouter: n, Interactor: n, Label: c.label}
}

type counterNode struct{ c counter }

func (n *counterNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size { return geom.Sz(50, 50) }

func (n *counterNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventPointerDown {
		return false
	}
	st := ctx.Retained[taps]()
	st.n++
	*n.c.seen = st.n
	return true
}

func TestRetainedStateSurvivesARebuildAndDiesWithTheNode(t *testing.T) {
	seen := 0
	show := true
	label := "a"
	root := func(*gift.Context) gift.View {
		if !show {
			return frameView{w: 100, h: 100}
		}
		return frameView{w: 100, h: 100, offsets: []geom.Point{at(0, 0)},
			children: []gift.View{counter{label: label, seen: &seen}}}
	}
	a := newInputApp(t, root)
	press := func() {
		a.BeginInput(0)
		a.PointerDown(gift.MousePointer, gift.PointerMouse, at(10, 10))
		a.PointerUp(gift.MousePointer, gift.PointerMouse, at(10, 10))
		mustUpdate(t, a)
	}
	press()
	label = "b"
	a.InvalidateAll()
	mustUpdate(t, a)
	press()
	if seen != 2 {
		t.Fatalf("the second press counted %d, want 2: the state did not survive the rebuild", seen)
	}
	show = false
	a.InvalidateAll()
	mustUpdate(t, a)
	show = true
	a.InvalidateAll()
	mustUpdate(t, a)
	press()
	if seen != 1 {
		t.Fatalf("a remounted node counted %d, want 1: it inherited its predecessor's state", seen)
	}
}
