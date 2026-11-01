package ui_test

import (
	"strconv"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// vlistNames returns n distinct titles, built once so that the row function
// under test is a slice index and allocates nothing.
func vlistNames(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "item " + strconv.Itoa(i)
	}
	return out
}

func titleRow(names []string) func(int) ui.ListRow {
	return func(i int) ui.ListRow { return ui.ListRow{Title: names[i]} }
}

// vlistHarness mounts a list of the given titles under the key "list" and
// records every tap in *tapped.
func vlistHarness(t testing.TB, s *ui.ListState, names []string, tapped *[]int) *gifttest.Harness {
	t.Helper()
	return gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.VirtualList(s, len(names), titleRow(names)).
			OnTap(func(i int) { *tapped = append(*tapped, i) }).
			Flex(1).Key("list")),
		Size:  geom.Sz(400, 600),
		Theme: ui.LightTheme(),
		Font:  loadTestFont(t),
	})
}

// TestVirtualListScrollPathCost is TestGalleryScrollPathCost for the list: a
// scroll by one viewport builds nothing, runs a bounded number of layouters,
// and costs the same at a hundred items and at a hundred thousand.
func TestVirtualListScrollPathCost(t *testing.T) {
	type cost struct{ builds, layouts uint64 }
	got := map[int]cost{}
	for _, n := range []int{100, 100000} {
		s := ui.NewListState()
		var tapped []int
		h := vlistHarness(t, s, vlistNames(n), &tapped)
		node := h.Find(gifttest.ByKey("list"))
		for range 3 {
			node.ScrollBy(600)
		}
		before := h.Diagnostics()
		node.ScrollBy(600)
		after := h.Diagnostics()
		got[n] = cost{after.Builds - before.Builds, after.Layouts - before.Layouts}
		t.Logf("n=%d: one viewport cost %d builds and %d layouts (pool %d)",
			n, got[n].builds, got[n].layouts, s.PoolSize())

		if got[n].builds != 0 {
			t.Errorf("n=%d: scrolling rebuilt %d scopes; the scroll path must not build", n, got[n].builds)
		}
		if limit := uint64(s.PoolSize() + 8); got[n].layouts > limit {
			t.Errorf("n=%d: scrolling ran %d layouters for a pool of %d, want at most %d",
				n, got[n].layouts, s.PoolSize(), limit)
		}
		// 600 pixels of rows of at least 44 is at most 15 visible rows.
		if s.PoolSize() > 15+1+4 {
			t.Errorf("n=%d: a pool of %d rows for a 600 pixel viewport", n, s.PoolSize())
		}
	}
	if got[100] != got[100000] {
		t.Errorf("one viewport costs %+v at n=100 and %+v at n=100000; the scroll path depends on N",
			got[100], got[100000])
	}
}

// TestVirtualListLabelsFollowTheItemsAfterRecycling is the list's version of
// "no wrong picture after recycling": a row node that now stands for item 520
// answers to that item's title and not to the one it was mounted with, and a
// tap on it reports item 520.
func TestVirtualListLabelsFollowTheItemsAfterRecycling(t *testing.T) {
	s := ui.NewListState()
	var tapped []int
	h := vlistHarness(t, s, vlistNames(1000), &tapped)
	h.Find(gifttest.ByText("item 3")).AssertVisible()

	s.Reveal(520)
	h.Settle()
	h.AssertNone(gifttest.ByText("item 3"))
	row := h.Find(gifttest.ByText("item 520")).AssertVisible()
	row.Tap()
	if len(tapped) != 1 || tapped[0] != 520 {
		t.Fatalf("a tap on the row labelled item 520 reported %v", tapped)
	}
	if s.Cursor() != 520 {
		t.Errorf("the cursor is on %d after tapping item 520", s.Cursor())
	}
}

func TestVirtualListKeysMoveTheCursorAndEnterActivates(t *testing.T) {
	s := ui.NewListState()
	var tapped []int
	h := vlistHarness(t, s, vlistNames(500), &tapped)
	h.Find(gifttest.ByKey("list")).Focus()

	for range 3 {
		h.Key(gift.KeyDown)
	}
	if s.Cursor() != 2 {
		t.Fatalf("three downs from no cursor put it on %d, want 2", s.Cursor())
	}
	h.Key(gift.KeyEnter)
	if len(tapped) != 1 || tapped[0] != 2 {
		t.Fatalf("enter on item 2 reported %v", tapped)
	}

	h.Key(gift.KeyEnd)
	h.Settle()
	if s.Cursor() != 499 {
		t.Fatalf("end put the cursor on %d, want 499", s.Cursor())
	}
	if _, end := s.VisibleRange(); end != 500 {
		t.Errorf("end did not scroll the last item into view: visible range ends at %d", end)
	}
	h.Find(gifttest.ByText("item 499")).AssertVisible()

	h.Key(gift.KeyHome)
	h.Settle()
	h.Find(gifttest.ByText("item 0")).AssertVisible()
}

func TestADisabledVirtualRowRefusesTapAndEnter(t *testing.T) {
	s := ui.NewListState()
	var tapped []int
	names := vlistNames(10)
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.VirtualList(s, len(names), func(i int) ui.ListRow {
			return ui.ListRow{Title: names[i], Disabled: i == 1}
		}).OnTap(func(i int) { tapped = append(tapped, i) }).Flex(1).Key("list")),
		Size: geom.Sz(400, 600), Theme: ui.LightTheme(), Font: loadTestFont(t),
	})
	h.Find(gifttest.ByText("item 1")).Tap()
	h.Find(gifttest.ByKey("list")).Focus()
	s.SetCursor(1)
	h.Key(gift.KeyEnter)
	if len(tapped) != 0 {
		t.Fatalf("a disabled row reported %v", tapped)
	}
	h.Find(gifttest.ByText("item 2")).Tap()
	if len(tapped) != 1 || tapped[0] != 2 {
		t.Fatalf("the enabled row next to it reported %v", tapped)
	}
}

// TestVirtualListFollowsItsData covers the two changes an application makes
// to a list it shows: the count shrinks under the scroll offset, and the data
// changes under unchanged indices. Both arrive through an ordinary state write
// and its rebuild.
func TestVirtualListFollowsItsData(t *testing.T) {
	s := ui.NewListState()
	names := vlistNames(1000)
	renamed := vlistNames(1000)
	for i := range renamed {
		renamed[i] = "renamed " + strconv.Itoa(i)
	}
	var count, version *gift.State[int]
	h := gifttest.New(t, gifttest.Options{
		Root: func(ctx *gift.Context) gift.View {
			count = ctx.State("count", 1000)
			version = ctx.State("version", 0)
			src := names
			if ctx.Read(version) > 0 {
				src = renamed
			}
			return ui.VStack(ui.VirtualList(s, ctx.Read(count), titleRow(src)).Flex(1).Key("list"))
		},
		Size: geom.Sz(400, 600), Theme: ui.LightTheme(), Font: loadTestFont(t),
	})

	s.Reveal(999)
	h.Settle()
	h.Find(gifttest.ByText("item 999")).AssertVisible()

	count.Set(10)
	h.Settle()
	if first, end := s.VisibleRange(); first != 0 || end != 10 {
		t.Fatalf("after shrinking to 10 items the list binds [%d, %d)", first, end)
	}
	h.Find(gifttest.ByText("item 9")).AssertVisible()
	h.AssertNone(gifttest.ByText("item 999"))

	version.Set(1)
	h.Settle()
	h.Find(gifttest.ByText("renamed 4")).AssertVisible()
	h.AssertNone(gifttest.ByText("item 4"))
}

// TestVirtualRowsCutLongLabelsInsteadOfOverflowing puts a title and a value
// that are both far too long into a narrow list.
func TestVirtualRowsCutLongLabelsInsteadOfOverflowing(t *testing.T) {
	s := ui.NewListState()
	long := "a file name that is very much longer than any phone is wide.jpeg"
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.VirtualList(s, 20, func(int) ui.ListRow {
			return ui.ListRow{Title: long, Subtitle: long, Value: long}
		}).Flex(1)),
		Size: geom.Sz(240, 400), Theme: ui.LightTheme(), Font: loadTestFont(t),
	})
	h.AssertNoOverflow()
	for _, g := range h.First(gifttest.ByText(long)).Glyphs() {
		if g.X > 240 {
			t.Fatalf("a glyph of a cut label sits at x=%v, past the 240 pixel row", g.X)
		}
	}
}

func TestEveryVirtualRowIsAtLeastTheTouchTargetTall(t *testing.T) {
	s := ui.NewListState()
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.VirtualList(s, 5, titleRow(vlistNames(5))).RowHeight(10).Flex(1)),
		Size: geom.Sz(400, 600), Theme: ui.LightTheme(), Font: loadTestFont(t),
	})
	if got := h.Find(gifttest.ByText("item 0")).Bounds().Height(); got < ui.RowHeight {
		t.Fatalf("a row is %v tall, want at least %v", got, ui.RowHeight)
	}
}

// --- faces and the hairlines around them --------------------------------------

// faceList mounts a tappable list of n titles with item sel selected.
func faceList(t testing.TB, s *ui.ListState, n, sel int) *gifttest.Harness {
	t.Helper()
	names := vlistNames(n)
	return gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.VirtualList(s, n, func(i int) ui.ListRow {
			return ui.ListRow{Title: names[i], Selected: i == sel}
		}).OnTap(func(int) {}).Flex(1).Key("list")),
		Size: geom.Sz(400, 600), Theme: ui.LightTheme(), Font: loadTestFont(t),
	})
}

// vlistFace returns the face the row of item i drew, if it drew one.
func vlistFace(h *gifttest.Harness, i int) (render.Op, bool) {
	b := h.Find(gifttest.ByText("item " + strconv.Itoa(i))).Bounds()
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Color.A > 0 && op.Bounds.Min == b.Min && op.Bounds.Max.X == b.Max.X {
			return op, true
		}
	}
	return render.Op{}, false
}

// TestNoHairlineRunsAlongAVirtualRowWithAFace is
// TestNoHairlineRunsAlongARowWithAFace for the virtual list: the hairlines on
// either side of the selected row and of the row under the mouse are not
// drawn, and come back when the mouse leaves.
func TestNoHairlineRunsAlongAVirtualRowWithAFace(t *testing.T) {
	h := faceList(t, ui.NewListState(), 4, 1)
	if got := separatorOps(t, h, sepColor()); len(got) != 1 {
		t.Fatalf("four rows, the second selected, drew %d hairlines; want the one between the third and the fourth", len(got))
	}
	h.MoveTo(h.Find(gifttest.ByText("item 3")).Center())
	if got := separatorOps(t, h, sepColor()); len(got) != 0 {
		t.Errorf("with the mouse over the last row %d hairlines are drawn; want none, the rest run along a face", len(got))
	}
	h.MoveTo(geom.Pt(390, 590))
	if got := separatorOps(t, h, sepColor()); len(got) != 1 {
		t.Errorf("with the mouse gone %d hairlines are drawn, want the one again", len(got))
	}
}

// TestRecycledRowsKnowTheFaceOfTheRowBelow: the row below a row is the next
// item, which after scrolling sits in whatever slot of the pool it was bound
// to, the first one included when the run of items wraps around the pool.
// Under the mouse, every row takes out its own hairline and the one above it,
// whichever slots the two are in.
func TestRecycledRowsKnowTheFaceOfTheRowBelow(t *testing.T) {
	s := ui.NewListState()
	h := faceList(t, s, 1000, -1)
	s.Reveal(437)
	h.Settle()
	first, end := s.VisibleRange()
	all := len(separatorOps(t, h, sepColor()))
	for i := first + 1; i < end-1; i++ {
		row := h.Find(gifttest.ByText("item " + strconv.Itoa(i)))
		if b := row.Bounds(); b.Min.Y < 0 || b.Max.Y > 600 {
			continue
		}
		h.MoveTo(row.Center())
		if got := len(separatorOps(t, h, sepColor())); got != all-2 {
			t.Errorf("with the mouse over item %d of %d…%d, %d of %d hairlines are drawn; want two fewer", i, first, end, got, all)
		}
	}
}

// TestTheFacesOfAVirtualRowAreTheFacesOfARow: selected, hovered and pressed,
// a row shows a neutral face rounded by RowCornerRadius, the selected one
// keeps its face under the mouse, and two faces next to each other keep the
// hairline's gap between them.
func TestTheFacesOfAVirtualRowAreTheFacesOfARow(t *testing.T) {
	h := faceList(t, ui.NewListState(), 4, 1)
	sel, ok := vlistFace(h, 1)
	if !ok || sel.CornerRadius != ui.RowCornerRadius {
		t.Fatalf("the selected row has the face %+v (found %v); want one rounded by %v", sel, ok, ui.RowCornerRadius)
	}
	if want := ui.LightTheme().Color(ui.Fade(ui.ColorLabel, 0.10)); sel.Color != want {
		t.Errorf("the selected face is %+v; want %+v, the selection of a ui.Row", sel.Color, want)
	}
	if _, ok := vlistFace(h, 2); ok {
		t.Fatal("a row that is neither selected nor under the mouse has a face")
	}

	h.MoveTo(h.Find(gifttest.ByText("item 2")).Center())
	hover, ok := vlistFace(h, 2)
	if !ok || hover.CornerRadius != ui.RowCornerRadius || !(hover.Color.A < sel.Color.A) {
		t.Errorf("the row under the mouse has the face %+v (found %v); want the selection's shape, fainter than %+v", hover, ok, sel.Color)
	}
	if sel, _ := vlistFace(h, 1); !(sel.Bounds.Max.Y < hover.Bounds.Min.Y) {
		t.Errorf("the selected face ends at %v and the hovered one below it starts at %v; want a gap between them",
			sel.Bounds.Max.Y, hover.Bounds.Min.Y)
	}
	h.MoveTo(h.Find(gifttest.ByText("item 1")).Center())
	if got, _ := vlistFace(h, 1); got.Color != sel.Color {
		t.Errorf("under the mouse the selected row turned %+v; want it to keep its face %+v", got.Color, sel.Color)
	}
	h.PressAt(h.Find(gifttest.ByText("item 1")).Center())
	if got, _ := vlistFace(h, 1); !(got.Color.A > sel.Color.A) || got.CornerRadius != ui.RowCornerRadius {
		t.Errorf("pressed, the selected row has the face %+v; want the same shape, stronger than its selection", got)
	}
}

// vlistApp mounts a list directly on a [gift.App], for the reason
// galleryApp gives: the harness copies the display list.
func vlistApp(t testing.TB, n int) (*gift.App, gift.NodeRef, *ui.ListState) {
	t.Helper()
	ui.SetDefaultFont(loadTestFont(t))
	s := ui.NewListState()
	names := vlistNames(n)
	a := gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		return ui.VStack(ui.VirtualList(s, len(names), titleRow(names)).Flex(1))
	}})
	for range 4 {
		if err := a.Update(geom.Sz(400, 600)); err != nil {
			t.Fatal(err)
		}
		a.Paint()
	}
	node, ok := galleryNodeOf(a, a.Root())
	if !ok {
		t.Fatal("no scroll container in the tree")
	}
	return a, node, s
}

// TestVirtualListScrollFrameIsAllocationFree applies the allocation contract
// of the project plan, section 11, to a scroll that rebinds rows. The titles
// were shaped once on the way down, so the frames measured here find them in
// the shaping cache; a title shaped for the first time allocates its glyphs,
// which is the cache's business and the same for any label.
func TestVirtualListScrollFrameIsAllocationFree(t *testing.T) {
	a, node, s := vlistApp(t, 2000)
	// Twelve steps down and twelve back, over and over, so that every title
	// on the way has been shaped before the measurement starts.
	steps := 0
	step := func() {
		d := 137.0
		if (steps/12)%2 == 1 {
			d = -d
		}
		steps++
		a.ScrollBy(node, d)
		if err := a.Update(geom.Sz(400, 600)); err != nil {
			t.Fatal(err)
		}
		a.Paint()
	}
	for range 48 {
		step()
	}
	first, _ := s.VisibleRange()
	step()
	if f, _ := s.VisibleRange(); f == first {
		t.Fatal("the step did not move the visible range; this is not the recycling case")
	}
	if got := testing.AllocsPerRun(200, step); got != 0 {
		t.Fatalf("a recycling scroll frame allocated %v times per run, want 0", got)
	}
}

// BenchmarkVirtualListScroll is one frame of a scrolling list: a wheel notch,
// an update and a paint of the visible rows, at two item counts that must cost
// the same.
//
//	go test ./ui -run x -bench VirtualListScroll -benchmem
func BenchmarkVirtualListScroll(b *testing.B) {
	for _, n := range []int{1000, 100000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			a, node, s := vlistApp(b, n)
			dir := 1.0
			step := func() {
				if info, _ := a.ScrollInfo(node); info.Offset >= info.MaxOffset-1 || info.Offset >= 20000 {
					dir = -1
				} else if info.Offset <= 1 {
					dir = 1
				}
				a.ScrollBy(node, 48*dir)
				if err := a.Update(geom.Sz(400, 600)); err != nil {
					b.Fatal(err)
				}
				a.Paint()
			}
			// Shape every title on the way once, so the loop measures the
			// list and not the shaper's first encounter with a string.
			for range 1000 {
				step()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				step()
			}
			b.StopTimer()
			b.ReportMetric(float64(s.PoolSize()), "rows")
		})
	}
}
