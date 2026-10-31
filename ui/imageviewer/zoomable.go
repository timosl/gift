package imageviewer

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/gesture"
)

var zoomableType = gift.RegisterType("imageviewer.Zoomable")

// WheelMode says what the wheel does over a zoomable picture.
type WheelMode uint8

const (
	// WheelWithModifier zooms with the wheel while control or command is
	// held and leaves the plain wheel to the ancestors, so that a picture
	// inside a scrolling page does not capture the page's scrolling. It is
	// the default of [Zoomable].
	WheelWithModifier WheelMode = iota
	// WheelZooms zooms with every wheel movement, about the cursor: the
	// behaviour of a photo viewer that has the screen to itself, and the
	// default of [Fullscreen].
	WheelZooms
	// WheelOff ignores the wheel.
	WheelOff
)

// wheelZoom is the zoom factor of one wheel notch: e^0.2, about 22 %. A
// trackpad reports fractions of a notch and zooms smoothly by the same rule.
const wheelZoom = 0.2

// ZoomableView is a picture that can be zoomed and panned in place: pinched,
// double tapped, wheeled and moved with the keys. It is created by
// [Zoomable].
type ZoomableView struct {
	src        asset.Source
	maxZoom    float64
	doubleTap  float64
	wheel      WheelMode
	background ui.Color
	key, label string
	w, h       float32
	flex       float32
}

// Zoomable returns a zoomable view of src.
//
// The picture fits the view and can be zoomed to [ZoomableView.MaxZoom];
// pinched past either limit it follows the fingers with growing resistance and
// springs back when they lift, as a UIScrollView with bouncesZoom does. Zoomed
// in, it pans under one finger, glides on when it is flicked and bounces off
// its edges. A double tap zooms in about the tap and out again.
//
// Not zoomed, the view leaves a one finger drag and the plain wheel to its
// ancestors, so it can sit in a scrolling page. It loads the picture in up to
// three stages through the shared image service: a preview, the size of the
// view, and a larger decode while it is zoomed; see [PreviewSize].
func Zoomable(src asset.Source) ZoomableView {
	return ZoomableView{src: src, background: ui.RGBA(0, 0, 0, 0)}
}

// MaxZoom sets the largest zoom relative to the fit. Zero, the default, is
// twice the picture's own pixels or three times the fit, whichever is more.
func (v ZoomableView) MaxZoom(f float64) ZoomableView { v.maxZoom = f; return v }

// DoubleTapZoom sets the zoom a double tap goes to. Zero, the default, is
// the zoom at which the picture fills the view, but at least two.
func (v ZoomableView) DoubleTapZoom(f float64) ZoomableView { v.doubleTap = f; return v }

// Wheel sets what the wheel does; see [WheelMode].
func (v ZoomableView) Wheel(m WheelMode) ZoomableView { v.wheel = m; return v }

// Background sets the colour behind the picture, where it does not cover the
// view. The default is transparent.
func (v ZoomableView) Background(c ui.Color) ZoomableView { v.background = c; return v }

// Frame fixes the size of the view. Without it the view fills what it is
// given, and takes the picture's aspect along an unbounded axis.
func (v ZoomableView) Frame(w, h float32) ZoomableView { v.w, v.h = w, h; return v }

// Flex sets the flex factor in a stack.
func (v ZoomableView) Flex(f float32) ZoomableView { v.flex = f; return v }

// Key sets the reconciliation key.
func (v ZoomableView) Key(s string) ZoomableView { v.key = s; return v }

// Label sets the accessible name.
func (v ZoomableView) Label(s string) ZoomableView { v.label = s; return v }

// ViewType implements gift.View.
func (ZoomableView) ViewType() gift.TypeID { return zoomableType }

// Build implements gift.View.
func (v ZoomableView) Build(*gift.BuildContext) gift.Element {
	n := &zoomableNode{v: v}
	n.gs = []gesture.Gesture{
		gesture.Magnify().
			OnChanged(n.onMagnify).
			OnEnded(n.onMagnify).
			OnCancelled(n.onCancel),
		gesture.Drag().
			ShouldBegin(func(d gesture.DragValue) bool { return n.st.z.zoomed() || d.Fingers > 1 || n.st.z.gesturing }).
			OnChanged(n.onDrag).
			OnEnded(n.onDragEnd).
			OnCancelled(n.onCancel),
		gesture.Tap().Count(2).OnEnded(n.onDoubleTap),
		gesture.Wheel().OnChanged(n.onWheel),
	}
	label := v.label
	if label == "" {
		label = "Picture"
	}
	return gift.Element{
		Key:        v.key,
		Label:      label,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Focusable:  true,
		MultiTouch: true,
		Flex:       v.flex,
	}
}

// zoomableState is what a zoomable view keeps across builds.
type zoomableState struct {
	z zoomer
	// mag and anchor are the magnification and centroid of the last pinch
	// update, and trans the translation of the last drag update: the
	// gestures report totals and the zoomer takes steps.
	mag              float64
	anchor           geom.Point
	trans            geom.Point
	pinching, panned bool
	stages           stages
	inited           bool
}

type zoomableNode struct {
	v  ZoomableView
	gs []gesture.Gesture
	st *zoomableState
	// now is the time of the event being handled, for the callbacks.
	now time.Duration
	ctx *gift.EventContext
}

// bind attaches the retained state to the node object of this build, for the
// gesture callbacks, which were created before the state could be found.
func (n *zoomableNode) bind(st *zoomableState) {
	n.st = st
	if !st.inited {
		st.z, st.inited = newZoomer(), true
	}
}

func (n *zoomableNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := ctx.Retained[zoomableState]()
	n.bind(st)
	natural := longestEdge(n.v.src)
	size := n.size(c, st.stages.aspect())
	st.z.maxZoom = maxZoom(n.v.maxZoom, natural, size, ctx.Density(), st.stages.aspect())
	z := st.z.z
	if st.z.mode == springing {
		z = st.z.to[0]
	}
	st.stages = requestStages(ctx, n.v.src, size.W, size.H, ctx.Density(), z, natural, false)
	st.z.setViewport(float64(size.W), float64(size.H), st.stages.aspect())
	ctx.ReportOverflow(geom.Size{})
	return size
}

// size is the frame, or what the constraints give, with the picture's aspect
// along an unbounded axis.
func (n *zoomableNode) size(c geom.Constraints, aspect float64) geom.Size {
	if n.v.w > 0 && n.v.h > 0 {
		return c.Constrain(geom.Sz(n.v.w, n.v.h))
	}
	if aspect <= 0 {
		aspect = 1
	}
	w, h := c.Max.W, c.Max.H
	switch {
	case c.HasBoundedWidth() && !c.HasBoundedHeight():
		h = w / float32(aspect)
	case c.HasBoundedHeight() && !c.HasBoundedWidth():
		w = h * float32(aspect)
	case !c.HasBoundedWidth() && !c.HasBoundedHeight():
		w, h = 320, 320/float32(aspect)
	}
	return c.Constrain(geom.Sz(w, h))
}

// maxZoom is the configured limit, or twice the picture's pixels on the
// device, but at least three times the fit.
func maxZoom(set float64, natural int, view geom.Size, density float32, aspect float64) float64 {
	if set > 0 {
		return math.Max(1, set)
	}
	if natural <= 0 || aspect <= 0 || view.W <= 0 || view.H <= 0 {
		return 4
	}
	// The fitted longest edge in device pixels.
	fw, fh := float64(view.W), float64(view.W)/aspect
	if fh > float64(view.H) {
		fh, fw = float64(view.H), float64(view.H)*aspect
	}
	fit := math.Max(fw, fh) * float64(density)
	return math.Max(3, 2*float64(natural)/fit)
}

func (n *zoomableNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[zoomableState]()
	b := ctx.Bounds()
	if !n.v.background.IsTransparent() {
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: n.v.background})
	}
	if !st.inited {
		return
	}
	z, ox, oy, moving := st.z.at(ctx.Now())
	r := st.z.rect(z, ox, oy).Translate(b.Min)
	ctx.PushClip(ctx.DeviceBounds())
	st.stages.draw(ctx, r, ui.OpaqueWhite, 0)
	ctx.PopClip()
	if moving {
		ctx.Animate(100 * time.Millisecond)
	}
}

func (n *zoomableNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	n.bind(ctx.Retained[zoomableState]())
	n.now, n.ctx = ctx.Now(), ctx
	defer func() { n.ctx = nil }()
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
	case gift.EventKeyDown:
		return n.key(e)
	case gift.EventRune:
		return n.rune(e.Rune)
	}
	return gesture.Handle(ctx, e, n.gs)
}

// local converts a device space position of a gesture into the viewport.
func local(c gesture.Common, p geom.Point) geom.Point { return c.Local(p) }

func (n *zoomableNode) moved(d time.Duration) {
	n.ctx.Repaint()
	n.ctx.Animate(d)
}

func (n *zoomableNode) onMagnify(v gesture.MagnifyValue) {
	st := n.st
	c := local(v.Common, v.Location)
	switch v.Phase {
	case gesture.Began:
		if !st.z.gesturing {
			st.z.begin(n.now)
		}
		st.pinching, st.mag, st.anchor = true, 1, local(v.Common, v.StartAnchor)
		fallthrough
	case gesture.Changed:
		st.z.pinch(st.anchor, c, v.Magnification/st.mag)
		st.mag, st.anchor = v.Magnification, c
		n.moved(time.Second)
	case gesture.Ended:
		st.pinching = false
		if !st.panned {
			n.release(0, 0, st.anchor)
		}
	}
}

func (n *zoomableNode) onDrag(v gesture.DragValue) {
	st := n.st
	if v.Phase == gesture.Began {
		if !st.z.gesturing {
			st.z.begin(n.now)
		}
		st.panned, st.trans = true, v.Translation
		return
	}
	d := v.Translation.Sub(st.trans)
	st.trans = v.Translation
	if st.pinching {
		// The pinch pans with the centroid already.
		return
	}
	st.z.pan(float64(d.X), float64(d.Y), nil)
	n.moved(time.Second)
}

func (n *zoomableNode) onDragEnd(v gesture.DragValue) {
	st := n.st
	st.panned = false
	if st.pinching {
		return
	}
	n.release(float64(v.Velocity.X), float64(v.Velocity.Y), local(v.Common, v.Location))
}

func (n *zoomableNode) onCancel() {
	st := n.st
	st.pinching, st.panned = false, false
	if st.z.gesturing {
		n.release(0, 0, geom.Pt(float32(st.z.vw/2), float32(st.z.vh/2)))
	}
}

// release ends a gesture and lets the picture settle. The layout that follows
// asks for the detail the new zoom needs.
func (n *zoomableNode) release(vx, vy float64, anchor geom.Point) {
	n.st.z.end(n.now, vx, vy, anchor)
	n.moved(3 * time.Second)
	n.ctx.RequestLayout()
}

func (n *zoomableNode) onDoubleTap(v gesture.TapValue) {
	n.st.z.toggle(n.now, local(v.Common, v.Location), n.v.doubleTap)
	n.moved(time.Second)
	n.ctx.RequestLayout()
}

func (n *zoomableNode) onWheel(v gesture.WheelValue) bool {
	switch n.v.wheel {
	case WheelOff:
		return false
	case WheelWithModifier:
		if !v.Mods.Has(gift.ModControl) && !v.Mods.Has(gift.ModMeta) {
			return false
		}
	}
	if v.Delta.Y == 0 {
		return false
	}
	n.st.z.scale(n.now, math.Exp(wheelZoom*float64(v.Delta.Y)), local(v.Common, v.Location))
	n.ctx.Repaint()
	n.ctx.RequestLayout()
	return true
}

// keyStep is how far an arrow key pans a zoomed picture, in logical pixels.
const keyStep = 60

func (n *zoomableNode) key(e gift.Event) bool {
	st := n.st
	if !st.z.zoomed() {
		return false
	}
	var dx, dy float64
	switch e.Key {
	case gift.KeyLeft:
		dx = keyStep
	case gift.KeyRight:
		dx = -keyStep
	case gift.KeyUp:
		dy = keyStep
	case gift.KeyDown:
		dy = -keyStep
	default:
		return false
	}
	st.z.begin(n.now)
	st.z.pan(dx, dy, nil)
	st.z.end(n.now, 0, 0, geom.Point{})
	n.moved(time.Second)
	return true
}

// rune handles the zoom keys: plus and minus zoom by half about the middle,
// zero goes back to the fit.
func (n *zoomableNode) rune(r rune) bool {
	return zoomKey(&n.st.z, n.now, r, func() {
		n.moved(time.Second)
		n.ctx.RequestLayout()
	})
}

// zoomKey applies a zoom key to m and reports whether r was one.
func zoomKey(m *zoomer, now time.Duration, r rune, changed func()) bool {
	mid := geom.Pt(float32(m.vw/2), float32(m.vh/2))
	switch r {
	case '+', '=':
		m.settle(now)
		m.zoomTo(now, m.z*1.5, mid)
	case '-', '_':
		m.settle(now)
		m.zoomTo(now, m.z/1.5, mid)
	case '0':
		m.settle(now)
		m.animate(now, 1, 0, 0, zoomSpring)
	default:
		return false
	}
	changed()
	return true
}
