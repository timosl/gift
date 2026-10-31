package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/gesture"
	"github.com/worldiety/gift/ui/motion"
)

var (
	wheelPickerType = gift.RegisterType("ui.WheelPicker")
	wheelColumnType = gift.RegisterType("ui.wheelColumn")
)

// Metrics of a wheel, in logical pixels: the ones of iOS's UIPickerView.
const (
	wheelRowHeight = float32(36)
	wheelRows      = 5
	wheelFontSize  = float32(21)
	wheelBandInset = float32(8)
	wheelBandR     = float32(8)
	// wheelScaleSteps is the resolution of the scale a row is drawn at; see
	// wheelNode.Paint.
	wheelScaleSteps = 64
)

// wheelSpring settles a wheel on its row: critically damped, so that a flick
// that was projected onto a row arrives there and does not swing past it.
var wheelSpring = motion.Spring{Response: 420 * time.Millisecond, Damping: 1}

// WheelColumn is one wheel of a [WheelPicker]: its rows and which of them is
// chosen. It is created by [Wheel].
type WheelColumn struct {
	count    int
	title    func(int) string
	selected int
	onChange func(int)
	bind     gift.Binding[int]
	cyclic   bool
	width    float32
	align    TextAlign
	key      string
	label    string
}

// Wheel returns a wheel of count rows, row i reading title(i).
func Wheel(count int, title func(int) string) WheelColumn {
	return WheelColumn{count: max(0, count), title: title, align: AlignCenter}
}

// Selected sets the chosen row.
func (w WheelColumn) Selected(i int) WheelColumn { w.selected = i; return w }

// OnChange sets the function called with the row the wheel comes to rest on.
// It is called when the finger lifts, with the row a flick will stop at, so the
// application knows the choice while the wheel is still turning.
func (w WheelColumn) OnChange(fn func(int)) WheelColumn { w.onChange = fn; return w }

// Bind reads the chosen row from b and writes the choice back.
func (w WheelColumn) Bind(b gift.Binding[int]) WheelColumn { w.bind = b; return w }

// Cyclic turns the wheel round: after the last row comes the first, as the
// hours and minutes of iOS do. A wheel of fewer rows than it shows at once
// does not turn round.
func (w WheelColumn) Cyclic(b bool) WheelColumn { w.cyclic = b; return w }

// Width fixes the width of the wheel. Without it the wheels share the width of
// the picker equally.
func (w WheelColumn) Width(px float32) WheelColumn { w.width = max(0, px); return w }

// Align sets how the rows line up in the wheel. The default is centred.
func (w WheelColumn) Align(a TextAlign) WheelColumn { w.align = a; return w }

// Key sets the key of the wheel, for a test to find it by. The default is
// "wheel-" followed by its position.
func (w WheelColumn) Key(s string) WheelColumn { w.key = s; return w }

// Label sets the accessible name of the wheel.
func (w WheelColumn) Label(s string) WheelColumn { w.label = s; return w }

func (w WheelColumn) chosen() int {
	if !w.bind.IsZero() {
		return w.bind.Get()
	}
	return w.selected
}

func (w WheelColumn) choose(i int) {
	if !w.bind.IsZero() {
		w.bind.Set(i)
	}
	if w.onChange != nil {
		w.onChange(i)
	}
}

// WheelPickerView is one or more wheels side by side, with the chosen row of
// each in a band across the middle. It is created by [WheelPicker].
type WheelPickerView struct {
	cols       []WheelColumn
	rows       int
	rowHeight  float32
	background Color
	key, label string
	flex       float32
	width      float32
}

// WheelPicker returns wheels side by side, the counterpart of SwiftUI's
// Picker with the wheel style and of iOS's UIPickerView.
//
//	sizes := []string{"S", "M", "L", "XL"}
//	ui.WheelPicker(
//		ui.Wheel(len(sizes), func(i int) string { return sizes[i] }).Bind(size.Binding()),
//	)
//
// The rows sit on a turning cylinder: they shrink and fade towards the top and
// the bottom, and the chosen one is in the band across the middle. A finger
// turns a wheel and a flick lets it run on, decelerating as a UIScrollView
// does, onto the row it would come to rest at; past the first and the last row
// it rubber bands. A tap on a row turns the wheel to it. With a mouse the wheel
// steps a row per notch, and a focused wheel steps with the arrow keys, pages
// with page up and down, and goes to the ends with home and end.
//
// The fading is drawn in the picker's background, [ColorSurface] by default;
// set [WheelPickerView.Background] to what the picker sits on.
func WheelPicker(cols ...WheelColumn) WheelPickerView {
	return WheelPickerView{cols: cols, rows: wheelRows, rowHeight: wheelRowHeight, background: ColorSurface}
}

// VisibleRows sets how many rows the picker is tall. The default is five.
func (v WheelPickerView) VisibleRows(n int) WheelPickerView { v.rows = max(3, n|1); return v }

// RowHeight sets the height of a row. The default is 36.
func (v WheelPickerView) RowHeight(h float32) WheelPickerView { v.rowHeight = max(16, h); return v }

// Background sets the colour behind the wheels and the colour the rows fade
// into.
func (v WheelPickerView) Background(c Color) WheelPickerView { v.background = c; return v }

// Width fixes the width of the picker. Without it the picker fills the width
// it is given, or takes the widths of its wheels when they are all fixed.
func (v WheelPickerView) Width(px float32) WheelPickerView { v.width = max(0, px); return v }

// Key sets the reconciliation key.
func (v WheelPickerView) Key(s string) WheelPickerView { v.key = s; return v }

// Label sets the accessible name of the picker.
func (v WheelPickerView) Label(s string) WheelPickerView { v.label = s; return v }

// Flex sets the flex factor in a stack.
func (v WheelPickerView) Flex(f float32) WheelPickerView { v.flex = f; return v }

// ViewType implements gift.View.
func (WheelPickerView) ViewType() gift.TypeID { return wheelPickerType }

// Build implements gift.View.
func (v WheelPickerView) Build(*gift.BuildContext) gift.Element {
	bg := ResolveColor(v.background)
	fg := ResolveColor(ColorLabel)
	kids := make([]gift.View, len(v.cols))
	for i, c := range v.cols {
		if c.key == "" {
			c.key = "wheel-" + strconv.Itoa(i)
		}
		kids[i] = wheelColumn{c: c, rowH: v.rowHeight, rows: v.rows, fg: fg}
	}
	n := &wheelPickerNode{v: v, bg: bg, band: ResolveColor(Fade(ColorLabel, 0.08))}
	return gift.Element{
		Key:      v.key,
		Label:    v.label,
		Layouter: n,
		Painter:  n,
		Children: kids,
		Flex:     v.flex,
	}
}

type wheelPickerNode struct {
	v        WheelPickerView
	bg, band Color
}

func (n *wheelPickerNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	h := float32(n.v.rows) * n.v.rowHeight
	fixed, flexible := float32(0), 0
	for _, col := range n.v.cols {
		if col.width > 0 {
			fixed += col.width
		} else {
			flexible++
		}
	}
	w := n.v.width
	switch {
	case w > 0:
	case flexible == 0:
		w = fixed
	case c.HasBoundedWidth():
		w = c.Max.W
	default:
		w = fixed + float32(flexible)*120
	}
	size := c.Constrain(geom.Sz(w, h))
	share := float32(0)
	if flexible > 0 {
		share = max(0, size.W-fixed) / float32(flexible)
	}
	x := float32(0)
	for i, col := range n.v.cols {
		cw := col.width
		if cw == 0 {
			cw = share
		}
		ctx.Measure(i, geom.Tight(geom.Sz(cw, size.H)))
		ctx.Place(i, geom.Pt(x, 0))
		x += cw
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

// Paint draws the background and the band, the wheels over them, and the
// fading over the wheels.
func (n *wheelPickerNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	if !n.bg.IsTransparent() {
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: n.bg})
	}
	rh := n.v.rowHeight
	cy := (b.Min.Y + b.Max.Y) / 2
	band := geom.Rc(b.Min.X+wheelBandInset, cy-rh/2, b.Max.X-wheelBandInset, cy+rh/2)
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: band, Color: n.band, CornerRadius: wheelBandR})
	ctx.PaintChildren()
	if n.bg.IsTransparent() {
		return
	}
	// The rows fade into the background towards the top and the bottom:
	// two gradients from the opaque background to a transparent one.
	clear := Color{}
	fade := (b.Height() - rh) / 2
	paintBackground(ctx, styleSpec{background: n.bg, grad: true, gradTo: clear}, geom.Rc(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+fade))
	paintBackground(ctx, styleSpec{background: clear, grad: true, gradTo: n.bg}, geom.Rc(b.Min.X, b.Max.Y-fade, b.Max.X, b.Max.Y))
}

// --- one wheel -----------------------------------------------------------------

type wheelColumn struct {
	c    WheelColumn
	rowH float32
	rows int
	fg   Color
}

func (wheelColumn) ViewType() gift.TypeID { return wheelColumnType }

func (w wheelColumn) Build(*gift.BuildContext) gift.Element {
	kids := make([]gift.View, w.c.count)
	for i := range w.c.count {
		title := ""
		if w.c.title != nil {
			title = w.c.title(i)
		}
		kids[i] = Text(title).FontSize(wheelFontSize).Foreground(w.fg).MaxLines(1).Align(w.c.align)
	}
	n := &wheelNode{w: w}
	n.gs = []gesture.Gesture{
		gesture.Drag().Axis(gesture.Vertical).OnChanged(n.onDrag).OnEnded(n.onDragEnd).OnCancelled(n.onCancel),
		gesture.Tap().OnEnded(n.onTap),
		gesture.Wheel().OnChanged(n.onWheel),
	}
	return gift.Element{
		Key:        w.c.key,
		Label:      w.c.label,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Focusable:  true,
		Children:   kids,
	}
}

// wheelState is what a wheel keeps across builds: where it is, what it is
// doing, and the row it last reported, which tells an echo of its own choice
// from a change the application made.
type wheelState struct {
	inited   bool
	off      float64 // the offset of the centre, in pixels: row i is at i·rowH
	raw      float64
	dragging bool
	trans    float32
	moving   bool
	t0       time.Duration
	from, to float64
	v0       float64
	reported int
	wheelAcc float32
}

type wheelNode struct {
	w   wheelColumn
	gs  []gesture.Gesture
	st  *wheelState
	ctx *gift.EventContext
	now time.Duration
}

func (n *wheelNode) cyclic() bool { return n.w.c.cyclic && n.w.c.count > n.w.rows+2 }

func (n *wheelNode) rowH() float64 { return float64(n.w.rowH) }

// at is the offset at now.
func (s *wheelState) at(now time.Duration) (float64, bool) {
	if !s.moving {
		return s.off, false
	}
	t := now - s.t0
	x, _ := wheelSpring.At(t, s.from, s.to, s.v0)
	return x, !wheelSpring.Settled(t, s.from, s.to, s.v0, 0.25)
}

func (s *wheelState) settle(now time.Duration) {
	s.off, _ = s.at(now)
	s.moving = false
}

func (s *wheelState) animate(now time.Duration, to, v0 float64) {
	s.settle(now)
	s.from, s.to, s.v0, s.t0, s.moving = s.off, to, v0, now, true
}

// index is the row an offset rests on, turned round for a cyclic wheel.
func (n *wheelNode) index(off float64) int {
	i := int(math.Round(off / n.rowH()))
	c := n.w.c.count
	if c == 0 {
		return 0
	}
	if n.cyclic() {
		return (i%c + c) % c
	}
	return max(0, min(c-1, i))
}

func (n *wheelNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := ctx.Retained[wheelState]()
	size := c.Constrain(c.Max)
	sel := n.w.c.chosen()
	count := n.w.c.count
	if count > 0 {
		sel = max(0, min(count-1, sel))
	}
	rh := n.rowH()
	switch {
	case !st.inited:
		st.inited, st.off, st.reported = true, float64(sel)*rh, sel
	case sel != st.reported && !st.dragging:
		// The application chose a row: turn to it, the short way round.
		st.reported = sel
		cur, _ := st.at(ctx.Now())
		to := float64(sel) * rh
		if n.cyclic() {
			span := float64(count) * rh
			to += math.Round((cur-to)/span) * span
		}
		st.animate(ctx.Now(), to, 0)
		ctx.Animate(time.Second)
	}
	for i := range ctx.ChildCount() {
		s := ctx.Measure(i, geom.Constraints{Max: geom.Sz(max(0, size.W-16), n.w.rowH)})
		ctx.Place(i, geom.Pt(8+alignOffset(n.w.c.align, size.W-16, s.W), (size.H-s.H)/2))
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

// Paint draws the rows on the cylinder: a row d pixels of unrolled wheel from
// the middle is at the angle d/R, shown R·sin of it from the middle and made
// smaller as it turns away, where R is half the height of the wheel.
func (n *wheelNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[wheelState]()
	b := ctx.Bounds()
	count := n.w.c.count
	if count == 0 {
		return
	}
	off, moving := st.at(ctx.Now())
	rh := n.rowH()
	r := float64(b.Height()) / 2
	cy := float64(b.Min.Y+b.Max.Y) / 2
	ctx.PushClip(deviceMapping(ctx)(b))
	centre := int(math.Floor(off / rh))
	reach := int(math.Ceil(r*math.Pi/2/rh)) + 1
	for k := -reach; k <= reach; k++ {
		i := centre + k
		theta := (float64(i)*rh - off) / r
		if math.Abs(theta) >= math.Pi/2 {
			continue
		}
		child := i
		if n.cyclic() {
			child = (i%count + count) % count
		} else if i < 0 || i >= count {
			continue
		}
		// Shown at the projection of its angle, and smaller the further
		// it turns away, about the middle of the wheel: a perspective
		// rather than a squash, because glyphs squashed one way only are
		// rasterised smaller both ways and come out letter spaced.
		//
		// The scale moves on a ladder of wheelScaleSteps and not with the
		// angle. Each scale is a size the glyph atlas rasterises and
		// keeps, and a continuous one was a new size for every row on
		// every frame of a turn, which filled the atlas until it turned
		// glyphs away. A step is two thirds of a pixel of a row at
		// density 2, which no eye follows on a turning wheel.
		y := r * math.Sin(theta)
		u := float32(math.Round((0.7+0.3*math.Cos(theta))*wheelScaleSteps) / wheelScaleSteps)
		cx := (b.Min.X + b.Max.X) / 2
		m := geom.Translate(geom.Pt(-cx, float32(-cy))).
			Mul(geom.Scale(u, u)).
			Mul(geom.Translate(geom.Pt(cx, float32(cy+y))))
		ctx.PushTransform(m)
		ctx.PaintChild(child)
		ctx.PopTransform()
	}
	ctx.PopClip()
	if ia := ctx.Interaction(); ia.FocusVisible {
		ring := b.Inset(geom.InsetsAll(2))
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: ring, Color: ResolveColor(ColorAccent), CornerRadius: wheelBandR, StrokeWidth: 2})
	}
	if moving {
		ctx.Animate(100 * time.Millisecond)
	}
}

func (n *wheelNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	n.st, n.ctx, n.now = ctx.Retained[wheelState](), ctx, ctx.Now()
	defer func() { n.ctx = nil }()
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		// A finger on a turning wheel stops it where it is, as on iOS.
		if n.st.moving {
			n.st.settle(n.now)
			ctx.Repaint()
		}
	case gift.EventKeyDown:
		return n.key(e)
	}
	return gesture.Handle(ctx, e, n.gs)
}

func (n *wheelNode) moved() {
	n.ctx.Repaint()
	n.ctx.Animate(time.Second)
}

// limits are the offsets of the first and the last row.
func (n *wheelNode) limits() (lo, hi float64) {
	return 0, float64(max(0, n.w.c.count-1)) * n.rowH()
}

func (n *wheelNode) onDrag(v gesture.DragValue) {
	st := n.st
	if v.Phase == gesture.Began {
		st.settle(n.now)
		st.dragging, st.raw, st.trans = true, st.off, v.Translation.Y
		return
	}
	d := v.Translation.Y - st.trans
	st.trans = v.Translation.Y
	st.raw -= float64(d)
	if n.cyclic() {
		st.off = st.raw
	} else {
		lo, hi := n.limits()
		st.off = motion.Band(st.raw, lo, hi, float64(n.w.rowH)*float64(n.w.rows))
	}
	n.moved()
}

// onDragEnd lets the wheel run on and settle on the row Apple's projection of
// the flick lands on.
func (n *wheelNode) onDragEnd(v gesture.DragValue) {
	st := n.st
	st.dragging = false
	vel := -float64(v.Velocity.Y)
	rest := st.off + motion.Project(vel, motion.DecelerationNormal)
	i := math.Round(rest / n.rowH())
	if !n.cyclic() {
		lo, hi := n.limits()
		i = math.Max(lo/n.rowH(), math.Min(hi/n.rowH(), i))
	}
	n.settleOn(i, vel)
}

func (n *wheelNode) onCancel() {
	st := n.st
	st.dragging = false
	n.settleOn(math.Round(st.off/n.rowH()), 0)
}

// settleOn turns the wheel to the unrolled row i and reports it.
func (n *wheelNode) settleOn(i, vel float64) {
	st := n.st
	st.animate(n.now, i*n.rowH(), vel)
	idx := n.index(i * n.rowH())
	if idx != st.reported {
		st.reported = idx
		n.w.c.choose(idx)
	}
	n.moved()
}

// onTap turns the wheel to the row that was tapped.
func (n *wheelNode) onTap(v gesture.TapValue) {
	st := n.st
	b := v.Bounds
	r := float64(b.Height()) / 2
	y := float64(v.Location.Y-(b.Min.Y+b.Max.Y)/2) / r
	if math.Abs(y) >= 1 {
		return
	}
	cur, _ := st.at(n.now)
	i := math.Round((cur + math.Asin(y)*r) / n.rowH())
	if !n.cyclic() {
		lo, hi := n.limits()
		i = math.Max(lo/n.rowH(), math.Min(hi/n.rowH(), i))
	}
	n.settleOn(i, 0)
}

// onWheel steps a row per notch; a trackpad's fractions of a notch add up.
func (n *wheelNode) onWheel(v gesture.WheelValue) bool {
	st := n.st
	st.wheelAcc += v.Delta.Y
	steps := int(st.wheelAcc)
	if steps == 0 {
		return true
	}
	st.wheelAcc -= float32(steps)
	n.step(-steps)
	return true
}

// step turns the wheel by k rows from where it is heading.
func (n *wheelNode) step(k int) {
	st := n.st
	target := st.off
	if st.moving {
		target = st.to
	}
	i := math.Round(target/n.rowH()) + float64(k)
	if !n.cyclic() {
		lo, hi := n.limits()
		i = math.Max(lo/n.rowH(), math.Min(hi/n.rowH(), i))
	}
	n.settleOn(i, 0)
}

func (n *wheelNode) key(e gift.Event) bool {
	switch e.Key {
	case gift.KeyUp:
		n.step(-1)
	case gift.KeyDown:
		n.step(1)
	case gift.KeyPageUp:
		n.step(-n.w.rows)
	case gift.KeyPageDown:
		n.step(n.w.rows)
	case gift.KeyHome:
		n.st.settle(n.now)
		n.settleOn(0, 0)
	case gift.KeyEnd:
		n.st.settle(n.now)
		n.settleOn(float64(max(0, n.w.c.count-1)), 0)
	default:
		return false
	}
	return true
}
