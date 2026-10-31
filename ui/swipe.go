package ui

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/motion"
)

var (
	swipeType        = gift.RegisterType("ui.Swipe")
	swipeRowType     = gift.RegisterType("ui.swipeRow")
	swipeButtonType  = gift.RegisterType("ui.swipeButton")
	swipeCatcherType = gift.RegisterType("ui.swipeCatcher")
)

// Metrics of swipe actions: those of UITableView's.
const (
	swipeMinAction = float32(74)
	swipePad       = float32(16)
	swipeTitle     = float32(15)
	swipeIcon      = float32(22)
	swipeFlick     = float32(300)
)

var swipeSpring = motion.Spring{Response: 350 * time.Millisecond, Damping: 1}

// SwipeActionView is one action behind a [Swipe] row. It is created by
// [SwipeAction].
type SwipeActionView struct {
	title       string
	action      func()
	icon        Symbol
	hasIcon     bool
	tint        Color
	hasTint     bool
	destructive bool
}

// SwipeAction returns an action with a title that runs action when it is
// tapped or, as the first of its edge, swiped through.
func SwipeAction(title string, action func()) SwipeActionView {
	return SwipeActionView{title: title, action: action}
}

// Icon puts a symbol above the title.
func (a SwipeActionView) Icon(s Symbol) SwipeActionView { a.icon, a.hasIcon = s, true; return a }

// Tint sets the colour of the action. The default is the grey of iOS, and
// [ColorDanger] for a destructive action.
func (a SwipeActionView) Tint(c Color) SwipeActionView { a.tint, a.hasTint = c, true; return a }

// Destructive marks the action as one that deletes, in [ColorDanger].
func (a SwipeActionView) Destructive() SwipeActionView { a.destructive = true; return a }

func (a SwipeActionView) colour() Color {
	switch {
	case a.hasTint:
		return a.tint
	case a.destructive:
		return ColorDanger
	}
	return RGB(142, 142, 147)
}

// SwipeView is a row with actions behind it. It is created by [Swipe].
type SwipeView struct {
	content           gift.View
	leading, trailing []SwipeActionView
	noFull            bool
	background        Color
	hasBg             bool
	key               string
	flex              float32
}

// Swipe returns content as a row that a finger or a mouse drags sideways to
// uncover actions behind it, the swipe actions of UITableView and of
// SwiftUI's swipeActions.
//
//	ui.Swipe(ui.Text(mail.Subject)).
//		Trailing(ui.SwipeAction("Delete", remove).Destructive(), ui.SwipeAction("Flag", flag)).
//		Leading(ui.SwipeAction("Read", markRead).Tint(ui.ColorAccent))
//
// Let go past half of the actions, or flicked, the row stays open on them; a
// tap on an action runs it and closes the row, and a tap anywhere else only
// closes it, as on iOS. Dragged far through, the first action of that edge
// grows over the whole row and runs on release; [SwipeView.FullSwipe] turns
// that off. The trailing actions are the ones a drag to the left uncovers,
// the first of them at the edge.
//
// The row hides the actions under a background of its own, [ColorBackground]
// unless [SwipeView.Background] says otherwise: set it to the colour the row
// sits on.
func Swipe(content gift.View) SwipeView { return SwipeView{content: content} }

// Trailing sets the actions a drag to the left uncovers, the first at the
// right edge.
func (v SwipeView) Trailing(actions ...SwipeActionView) SwipeView { v.trailing = actions; return v }

// Leading sets the actions a drag to the right uncovers, the first at the
// left edge.
func (v SwipeView) Leading(actions ...SwipeActionView) SwipeView { v.leading = actions; return v }

// FullSwipe with false stops a drag far through from running the first
// action; the row then only opens.
func (v SwipeView) FullSwipe(b bool) SwipeView { v.noFull = !b; return v }

// Background sets the colour behind the content, which covers the actions.
func (v SwipeView) Background(c Color) SwipeView { v.background, v.hasBg = c, true; return v }

// Key sets the reconciliation key.
func (v SwipeView) Key(s string) SwipeView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v SwipeView) Flex(f float32) SwipeView { v.flex = f; return v }

// ViewType implements gift.View.
func (v SwipeView) ViewType() gift.TypeID { return swipeType }

// Build implements gift.View.
func (v SwipeView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{}, Children: []gift.View{gift.Component("ui.swipe", v.body)}}
}

// body keeps which edge is open, -1 for the trailing actions, 1 for the
// leading ones, as component state, so that opening and closing rebuild the
// row with or without the catcher that closes it.
func (v SwipeView) body(ctx *gift.Context) gift.View {
	open := ctx.State("open", 0)
	ctx.Read(open)
	return swipeRow{v: v, open: open.Get(), setOpen: open.Set}
}

// --- the row -------------------------------------------------------------------

type swipeRow struct {
	v       SwipeView
	open    int
	setOpen func(int)
}

func (swipeRow) ViewType() gift.TypeID { return swipeRowType }

func (r swipeRow) Build(*gift.BuildContext) gift.Element {
	n := &swipeNode{r: r, bg: ResolveColor(ColorBackground)}
	if r.v.hasBg {
		n.bg = ResolveColor(r.v.background)
	}
	var kids []gift.View
	for i, a := range r.v.trailing {
		kids = append(kids, swipeButton{a: a, run: n.runner(-1, i)})
	}
	for i, a := range r.v.leading {
		kids = append(kids, swipeButton{a: a, run: n.runner(1, i)})
	}
	kids = append(kids, r.v.content)
	n.colours = make([]Color, 0, len(r.v.trailing)+len(r.v.leading))
	for _, a := range r.v.trailing {
		n.colours = append(n.colours, ResolveColor(a.colour()))
	}
	for _, a := range r.v.leading {
		n.colours = append(n.colours, ResolveColor(a.colour()))
	}
	e := gift.Element{Key: "ui.swipeRow", Layouter: n, Painter: n, Interactor: n, Children: kids}
	if r.open != 0 {
		e.Overlay = swipeCatcher{side: r.open, width: n.sideWidthOf(r.open), close: func() { r.setOpen(0) }}
	}
	return e
}

// swipeState is the position of the content, kept across builds: where it is
// under a finger, or the spring that brings it to rest.
type swipeState struct {
	off       float32 // the content's displacement, negative to the left
	dragging  bool
	acc       geom.Point // movement seen before the slop, to tell the direction
	lastMove  time.Duration
	grab      float32 // pointer x at which off would be grabOff
	grabOff   float32
	vel       float32
	prevX     float32
	prevT     time.Duration
	moving    bool
	t0        time.Duration
	from, to  float32
	v0        float32
	settledTo float32
	inited    bool
}

func (s *swipeState) at(now time.Duration) (float32, bool) {
	if !s.moving {
		return s.off, false
	}
	t := now - s.t0
	x, _ := swipeSpring.At(t, float64(s.from), float64(s.to), float64(s.v0))
	if swipeSpring.Settled(t, float64(s.from), float64(s.to), float64(s.v0), 0.25) {
		s.off, s.moving = s.to, false
		return s.to, false
	}
	return float32(x), true
}

func (s *swipeState) springTo(now time.Duration, to, v float32) {
	s.off, _ = s.at(now)
	s.from, s.to, s.v0, s.t0, s.moving = s.off, to, v, now, true
}

type swipeNode struct {
	r       swipeRow
	bg      Color
	colours []Color

	// widths are the natural widths of the trailing and then the leading
	// actions, and w the width of the row, from the last layout.
	widths []float32
	w      float32
}

// runner returns what a tap on action i of side runs: the action, then the
// row closes.
func (n *swipeNode) runner(side, i int) func() {
	return func() {
		a := n.action(side, i)
		n.r.setOpen(0)
		if a.action != nil {
			a.action()
		}
	}
}

func (n *swipeNode) actions(side int) []SwipeActionView {
	if side < 0 {
		return n.r.v.trailing
	}
	return n.r.v.leading
}

func (n *swipeNode) action(side, i int) SwipeActionView { return n.actions(side)[i] }

// index is the child index of action i of side.
func (n *swipeNode) index(side, i int) int {
	if side < 0 {
		return i
	}
	return len(n.r.v.trailing) + i
}

// sideWidthOf is the width the actions of side need, by their titles; it is
// an estimate before the first layout and exact after it.
func (n *swipeNode) sideWidthOf(side int) float32 {
	if len(n.widths) == len(n.r.v.trailing)+len(n.r.v.leading) {
		return n.sideWidth(side)
	}
	return float32(len(n.actions(side))) * swipeMinAction
}

func (n *swipeNode) sideWidth(side int) float32 {
	var w float32
	for i := range n.actions(side) {
		w += n.widths[n.index(side, i)]
	}
	return w
}

// rest is where the content rests for the open state.
func (n *swipeNode) rest() float32 {
	switch {
	case n.r.open < 0:
		return -n.sideWidth(-1)
	case n.r.open > 0:
		return n.sideWidth(1)
	}
	return 0
}

func (n *swipeNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := ctx.Retained[swipeState]()
	k := len(n.r.v.trailing) + len(n.r.v.leading)
	// A row fills the width it is given, as a list row does, and is as tall
	// as its content.
	cc := c
	if c.HasBoundedWidth() {
		cc.Min.W = c.Max.W
	}
	content := ctx.Measure(k, cc)
	size := c.Constrain(content)
	n.w = size.W
	n.widths = n.widths[:0]
	for side := -1; side <= 1; side += 2 {
		for i := range n.actions(side) {
			// The natural width, measured from the title.
			s := ctx.Measure(n.index(side, i), geom.Constraints{Max: geom.Sz(size.W, size.H)})
			n.widths = append(n.widths, max(swipeMinAction, s.W))
		}
	}
	// Each action sits where it rests while its side is open.
	x := size.W
	for i := range n.r.v.trailing {
		w := n.widths[n.index(-1, i)]
		x -= w
		ctx.Measure(n.index(-1, i), geom.Tight(geom.Sz(w, size.H)))
		ctx.Place(n.index(-1, i), geom.Pt(x, 0))
	}
	x = 0
	for i := range n.r.v.leading {
		w := n.widths[n.index(1, i)]
		ctx.Measure(n.index(1, i), geom.Tight(geom.Sz(w, size.H)))
		ctx.Place(n.index(1, i), geom.Pt(x, 0))
		x += w
	}
	rest := n.rest()
	switch {
	case !st.inited:
		st.inited, st.off, st.settledTo = true, rest, rest
	case rest != st.settledTo && !st.dragging:
		st.settledTo = rest
		st.springTo(ctx.Now(), rest, 0)
		ctx.Animate(time.Second)
	}
	ctx.Place(k, geom.Pt(rest, 0))
	ctx.ReportOverflow(geom.Size{})
	return size
}

// full reports whether a displacement of off has dragged far enough through
// for the first action of its side to run on release.
func (n *swipeNode) full(off float32) bool {
	side := -1
	if off > 0 {
		side = 1
	}
	if n.r.v.noFull || len(n.actions(side)) == 0 {
		return false
	}
	return absf(off) > max(n.sideWidth(side)+60, n.w*0.55)
}

func (n *swipeNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[swipeState]()
	b := ctx.Bounds()
	off, moving := st.at(ctx.Now())
	ctx.PushClip(deviceRect(ctx, b))
	if off < 0 {
		n.paintSide(ctx, b, -1, -off)
	} else if off > 0 {
		n.paintSide(ctx, b, 1, off)
	}
	// The content, over a background of its own, where it shows now: it is
	// laid out where it rests.
	k := len(n.r.v.trailing) + len(n.r.v.leading)
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b.Translate(geom.Pt(off, 0)), Color: n.bg})
	ctx.PushTransform(geom.Translate(geom.Pt(off-n.rest(), 0)))
	ctx.PaintChild(k)
	ctx.PopTransform()
	ctx.PopClip()
	if moving {
		ctx.Animate(50 * time.Millisecond)
	}
}

// paintSide draws the actions of side in the revealed width rv: each in its
// share of it while they come into view, each at its own width once they all
// show, and the first over all of it when a full swipe is armed.
func (n *swipeNode) paintSide(ctx *gift.PaintContext, b geom.Rect, side int, rv float32) {
	acts := n.actions(side)
	if len(acts) == 0 {
		return
	}
	total := n.sideWidth(side)
	full := n.full(float32(side) * rv)
	scale := min(rv/total, 1)
	extra := max(rv-total, 0)
	// edge is the outer edge of the row on this side, and dir the direction
	// from it into the row.
	edge, dir := b.Max.X, float32(-1)
	if side > 0 {
		edge, dir = b.Min.X, 1
	}
	pos := float32(0) // distance of the next action's outer edge from the edge
	for i := range acts {
		idx := n.index(side, i)
		w := n.widths[idx] * scale
		if i == 0 {
			w += extra
		}
		if full {
			if i > 0 {
				break
			}
			w = rv
		}
		x0, x1 := edge+dir*pos, edge+dir*(pos+w)
		r := geom.Rc(min(x0, x1), b.Min.Y, max(x0, x1), b.Max.Y)
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: r, Color: n.colours[idx]})
		// The label, moved from where it rests to the middle of its share,
		// or, for a growing first action, kept at the inner edge.
		laid := n.restRect(b, side, i)
		cx := (r.Min.X + r.Max.X) / 2
		if i == 0 && (extra > 0 || full) {
			cx = r.Min.X + n.widths[idx]/2
			if side > 0 {
				cx = r.Max.X - n.widths[idx]/2
			}
		}
		ctx.PushClip(deviceRect(ctx, r))
		ctx.PushTransform(geom.Translate(geom.Pt(cx-(laid.Min.X+laid.Max.X)/2, 0)))
		ctx.PaintChild(idx)
		ctx.PopTransform()
		ctx.PopClip()
		pos += w
	}
}

// restRect is where action i of side is laid out, in paint space.
func (n *swipeNode) restRect(b geom.Rect, side, i int) geom.Rect {
	var pos float32
	for j := 0; j < i; j++ {
		pos += n.widths[n.index(side, j)]
	}
	w := n.widths[n.index(side, i)]
	if side < 0 {
		return geom.Rc(b.Max.X-pos-w, b.Min.Y, b.Max.X-pos, b.Max.Y)
	}
	return geom.Rc(b.Min.X+pos, b.Min.Y, b.Min.X+pos+w, b.Max.Y)
}

// HandleEvent follows a sideways drag. The moves inside the slop bubble here
// from whatever the press landed on, a button in the row say, and tell the
// direction; a stroke that goes more up or down is left to the list.
func (n *swipeNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.Retained[swipeState]()
	now := e.Time
	switch e.Kind {
	case gift.EventPointerDown:
		st.acc = geom.Point{}
		st.lastMove = now
		return false
	case gift.EventPointerMove:
		if st.dragging {
			x := e.Pos.X
			off := st.grabOff + (x - st.grab)
			n.limit(&off)
			st.off, st.moving = off, false
			if dt := (now - st.prevT).Seconds(); dt > 0 {
				st.vel = 0.6*st.vel + 0.4*float32(float64(x-st.prevX)/dt)
			}
			st.prevX, st.prevT = x, now
			ctx.Repaint()
			return true
		}
		if now-st.lastMove > 150*time.Millisecond {
			st.acc = geom.Point{}
		}
		st.lastMove = now
		st.acc = st.acc.Add(e.Delta)
		if !e.Dragged {
			return false
		}
		if absf(st.acc.Y) >= absf(st.acc.X) {
			return false
		}
		if !ctx.StealPointer() {
			return false
		}
		cur, _ := st.at(now)
		st.dragging, st.moving = true, false
		st.grab, st.grabOff = e.Pos.X-st.acc.X, cur
		st.prevX, st.prevT, st.vel = e.Pos.X, now, 0
		off := st.grabOff + st.acc.X
		n.limit(&off)
		st.off = off
		ctx.Repaint()
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.dragging {
			return false
		}
		st.dragging = false
		n.release(ctx, st, e.Kind == gift.EventPointerCancel)
		return true
	}
	return false
}

// limit holds the content back like a rubber band past what the actions of
// its side allow: past all of them without a full swipe, and at all on a side
// without actions.
func (n *swipeNode) limit(off *float32) {
	side := -1
	if *off > 0 {
		side = 1
	}
	allow := float32(0)
	if len(n.actions(side)) > 0 {
		allow = n.w
		if n.r.v.noFull {
			allow = n.sideWidth(side)
		}
	}
	if a := absf(*off); a > allow {
		*off = float32(side) * (allow + float32(motion.Rubber(float64(a-allow), float64(max(n.w, 1)))))
	}
}

// release decides where the content goes when the finger lifts: through, open
// or closed, by how far it went and how fast.
func (n *swipeNode) release(ctx *gift.EventContext, st *swipeState, cancelled bool) {
	off, v := st.off, st.vel
	if ctx.Now()-st.prevT > 50*time.Millisecond {
		// A finger that stopped before it lifted throws nothing.
		v = 0
	}
	target := 0
	switch {
	case cancelled:
	case n.full(off):
		side := -1
		if off > 0 {
			side = 1
		}
		n.runner(side, 0)()
		st.settledTo = 0
		st.springTo(ctx.Now(), 0, v)
		ctx.Animate(time.Second)
		return
	case off < 0 && len(n.r.v.trailing) > 0:
		if v < -swipeFlick || (v <= swipeFlick && -off > n.sideWidth(-1)/2) {
			target = -1
		}
	case off > 0 && len(n.r.v.leading) > 0:
		if v > swipeFlick || (v >= -swipeFlick && off > n.sideWidth(1)/2) {
			target = 1
		}
	}
	rest := float32(0)
	if target != 0 {
		rest = float32(target) * n.sideWidth(target)
	}
	st.settledTo = rest
	st.springTo(ctx.Now(), rest, v)
	ctx.Animate(time.Second)
	if target != n.r.open {
		n.r.setOpen(target)
	}
}

// --- an action -------------------------------------------------------------------

type swipeButton struct {
	a   SwipeActionView
	run func()
}

func (swipeButton) ViewType() gift.TypeID { return swipeButtonType }

func (b swipeButton) Build(*gift.BuildContext) gift.Element {
	white := RGB(255, 255, 255)
	var kids []gift.View
	if b.a.hasIcon {
		kids = append(kids, Icon(b.a.icon).Size(swipeIcon).Foreground(white))
	}
	kids = append(kids, Text(b.a.title).FontSize(swipeTitle).Foreground(white).MaxLines(1))
	n := &swipeButtonNode{run: b.run}
	return gift.Element{Key: "swipe-" + b.a.title, Label: b.a.title, Layouter: n, Interactor: n, Children: kids}
}

type swipeButtonNode struct{ run func() }

// Layout centres the icon over the title; unbounded, it reports the natural
// width the row measures it for.
func (n *swipeButtonNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	var sizes [2]geom.Size
	var w, h float32
	for i := range ctx.ChildCount() {
		sizes[i] = ctx.Measure(i, geom.Constraints{Max: geom.Sz(max(c.Max.W-2*swipePad, 0), c.Max.H)})
		w, h = max(w, sizes[i].W), h+sizes[i].H
	}
	size := c.Constrain(geom.Sz(w+2*swipePad, c.Max.H))
	y := (size.H - h) / 2
	for i := range ctx.ChildCount() {
		ctx.Place(i, geom.Pt((size.W-sizes[i].W)/2, y))
		y += sizes[i].H
	}
	return size
}

func (n *swipeButtonNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			n.run()
		}
		return true
	}
	return false
}

// --- the catcher ---------------------------------------------------------------------

// swipeCatcher is the top layer of an open row: four rectangles around its
// actions, so that a press anywhere but on them closes the row and does
// nothing else, as on iOS.
type swipeCatcher struct {
	side  int
	width float32
	close func()
}

func (swipeCatcher) ViewType() gift.TypeID { return swipeCatcherType }

func (c swipeCatcher) Build(*gift.BuildContext) gift.Element {
	kids := make([]gift.View, 4)
	for i := range kids {
		kids[i] = swipeShield{close: c.close}
	}
	return gift.Element{Key: "ui.swipeCatcher", Layouter: c, Children: kids}
}

func (c swipeCatcher) Layout(ctx *gift.LayoutContext, cs geom.Constraints) geom.Size {
	win := cs.Constrain(cs.Max)
	row, ok := ctx.Presenter()
	if !ok {
		row = geom.Rect{}
	}
	act := geom.Rc(row.Max.X-c.width, row.Min.Y, row.Max.X, row.Max.Y)
	if c.side > 0 {
		act = geom.Rc(row.Min.X, row.Min.Y, row.Min.X+c.width, row.Max.Y)
	}
	rects := [4]geom.Rect{
		geom.Rc(0, 0, win.W, act.Min.Y),
		geom.Rc(0, act.Max.Y, win.W, win.H),
		geom.Rc(0, act.Min.Y, act.Min.X, act.Max.Y),
		geom.Rc(act.Max.X, act.Min.Y, win.W, act.Max.Y),
	}
	for i, r := range rects {
		s := geom.Sz(max(r.Width(), 0), max(r.Height(), 0))
		ctx.Measure(i, geom.Tight(s))
		ctx.Place(i, r.Min)
	}
	return win
}

var swipeShieldType = gift.RegisterType("ui.swipeShield")

type swipeShield struct{ close func() }

func (swipeShield) ViewType() gift.TypeID { return swipeShieldType }

func (s swipeShield) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: fillShield{}, Interactor: s}
}

func (s swipeShield) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		s.close()
		return true
	case gift.EventPointerMove, gift.EventPointerUp, gift.EventPointerCancel:
		return true
	}
	return false
}

type fillShield struct{}

func (fillShield) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(c.Max)
}
