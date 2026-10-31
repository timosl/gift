package ui_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// searchScene is a search field bound to state, inside a component that
// counts the escapes that got past it.
func searchScene(t *testing.T) (*gifttest.Harness, *string, *int, *[]string) {
	t.Helper()
	term, escapes, submits := new(string), new(int), new([]string)
	root := func(ctx *gift.Context) gift.View {
		q := ctx.State("q", "")
		*term = ctx.Read(q)
		return escapeCounter{n: escapes, child: ui.VStack(
			ui.SearchField(ui.Editor(ctx, "q-editor", "")).Bind(q.Binding()).
				OnSubmit(func(s string) { *submits = append(*submits, s) }).Key("search"),
		).Padding(20)}
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(400, 200), Font: loadTestFont(t)})
	h.Settle()
	return h, term, escapes, submits
}

var escapeCounterType = gift.RegisterType("test.escapeCounter")

type escapeCounter struct {
	n     *int
	child gift.View
}

func (escapeCounter) ViewType() gift.TypeID { return escapeCounterType }

func (e escapeCounter) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: e, Interactor: e, Children: []gift.View{e.child}}
}

func (escapeCounter) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

func (e escapeCounter) HandleEvent(_ *gift.EventContext, ev gift.Event) bool {
	if ev.Kind == gift.EventKeyDown && ev.Key == gift.KeyEscape {
		*e.n++
		return true
	}
	return false
}

func TestASearchFieldClearsWithItsButtonAndKeepsTheFocus(t *testing.T) {
	h, term, _, submits := searchScene(t)
	b := h.Find(gifttest.ByKey("search")).LayoutBounds()
	if b.Height() != 36 {
		t.Fatalf("a search field is %v tall, want 36", b.Height())
	}
	h.AssertNone(gifttest.ByKey("search-clear"))
	h.Find(gifttest.ByKey("search-field")).Focus()
	h.TypeText("cats")
	h.Settle()
	if *term != "cats" {
		t.Fatalf("the bound term is %q", *term)
	}
	h.Key(gift.KeyEnter)
	if len(*submits) != 1 || (*submits)[0] != "cats" {
		t.Fatalf("enter submitted %v", *submits)
	}
	h.Find(gifttest.ByKey("search-clear")).Tap()
	h.Settle()
	if *term != "" {
		t.Fatalf("the clear button left %q", *term)
	}
	h.AssertNone(gifttest.ByKey("search-clear"))
	if f, ok := h.Focused(); !ok || f.Key() != "search-field" {
		t.Fatal("the clear button took the focus from the field")
	}
	h.TypeText("dogs")
	h.Settle()
	if *term != "dogs" {
		t.Fatalf("typing after clearing gave %q", *term)
	}
}

func TestEscapeClearsASearchFieldAndThenGoesOn(t *testing.T) {
	h, term, escapes, _ := searchScene(t)
	h.Find(gifttest.ByKey("search-field")).Focus()
	h.TypeText("x")
	h.Settle()
	h.Key(gift.KeyEscape)
	h.Settle()
	if *term != "" || *escapes != 0 {
		t.Fatalf("the first escape left %q and reached the outside %d times", *term, *escapes)
	}
	h.Key(gift.KeyEscape)
	if *escapes != 1 {
		t.Fatalf("escape in an empty field reached the outside %d times, want once", *escapes)
	}
}
