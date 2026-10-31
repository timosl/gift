package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/motion"
)

var (
	progressRingType   = gift.RegisterType("ui.ProgressRing")
	gaugeType          = gift.RegisterType("ui.Gauge")
	circularSliderType = gift.RegisterType("ui.CircularSlider")
)

// ringSpring moves a ring to a new value, as a progress view eases to it.
var ringSpring = motion.Spring{Response: 400 * time.Millisecond, Damping: 1}

// The open ring of a gauge and a circular slider: from half past seven
// clockwise to half past four, with the gap at the bottom.
const (
	openStart = float32(5 * math.Pi / 4)
	openSweep = float32(3 * math.Pi / 2)
)

// easedValue is a fraction on its way to a new one: kept across builds, so
// that a ring given another value moves there instead of jumping.
type easedValue struct {
	inited    bool
	shown, to float64
	from, v0  float64
	t0        time.Duration
	moving    bool
}

// set aims at f from wherever the value shows at now.
func (e *easedValue) set(now time.Duration, f float64) {
	switch {
	case !e.inited:
		e.inited, e.shown, e.to = true, f, f
	case f != e.to:
		e.shown, _ = e.at(now)
		e.from, e.to, e.v0, e.t0, e.moving = e.shown, f, 0, now, true
	}
}

func (e *easedValue) at(now time.Duration) (float64, bool) {
	if !e.moving {
		return e.to, false
	}
	t := now - e.t0
	if ringSpring.Settled(t, e.from, e.to, e.v0, 0.001) {
		e.moving = false
		return e.to, false
	}
	x, _ := ringSpring.At(t, e.from, e.to, e.v0)
	return x, true
}

// --- progress ring -------------------------------------------------------------

// ProgressRingView shows how far a task has come as a ring that fills
// clockwise, or that a task is under way as a spinner. It is created by
// [ProgressRing].
type ProgressRingView struct {
	base
	fraction      float64
	indeterminate bool
	tint          Color
	hasTint       bool
	size, line    float32
	name          string
}

// ProgressRing returns a ring that is fraction full, clockwise from twelve
// o'clock, over a faint track: the circular counterpart of [ProgressBar], as
// iOS draws an activity ring. A new fraction is eased to rather than jumped
// to. Like the bar, a fraction above 1 is full and a negative one panics.
//
//	ui.ProgressRing(0.4)
//	ui.ProgressRing(0).Indeterminate() // the spinner of UIActivityIndicatorView
func ProgressRing(fraction float64) ProgressRingView {
	return ProgressRingView{fraction: min(checkFraction("ProgressRing", fraction), 1), size: 28}
}

// Indeterminate shows the turning spinner of iOS instead of a ring, for a
// task whose end nobody knows.
func (v ProgressRingView) Indeterminate() ProgressRingView { v.indeterminate = true; return v }

// Tint sets the colour of the ring, [ColorAccent] by default, or of the
// spinner, [ColorSecondaryLabel] by default.
func (v ProgressRingView) Tint(c Color) ProgressRingView { v.tint, v.hasTint = c, true; return v }

// Size sets the diameter; the default is 28.
func (v ProgressRingView) Size(px float32) ProgressRingView { v.size = max(px, 4); return v }

// LineWidth sets the width of the ring; the default is a seventh of the
// size.
func (v ProgressRingView) LineWidth(px float32) ProgressRingView { v.line = max(px, 0.5); return v }

// Label sets the accessible name.
func (v ProgressRingView) Label(s string) ProgressRingView { v.name = s; return v }

// Key sets the reconciliation key.
func (v ProgressRingView) Key(s string) ProgressRingView { v.setKey(s); return v }

// Flex sets the flex factor in a stack.
func (v ProgressRingView) Flex(f float32) ProgressRingView { v.setFlex(f); return v }

// ViewType implements gift.View.
func (v ProgressRingView) ViewType() gift.TypeID { return progressRingType }

// Build implements gift.View.
func (v ProgressRingView) Build(*gift.BuildContext) gift.Element {
	tint := Color(ColorAccent)
	if v.indeterminate {
		tint = ColorSecondaryLabel
	}
	if v.hasTint {
		tint = v.tint
	}
	n := &progressRingNode{v: v, tint: ResolveColor(tint), track: ResolveColor(Fade(tint, 0.2))}
	return gift.Element{Key: v.key, Flex: v.flex, Label: v.name, Layouter: n, Painter: n}
}

type progressRingNode struct {
	v           ProgressRingView
	tint, track Color
}

func (n *progressRingNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	ctx.Retained[easedValue]().set(ctx.Now(), n.v.fraction)
	return c.Constrain(geom.Sz(n.v.size, n.v.size))
}

func (n *progressRingNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	r := min(b.Width(), b.Height()) / 2
	if n.v.indeterminate {
		paintSpinner(ctx, c, 2*r/spinnerSize, n.tint, spinnerSpokes, true)
		return
	}
	w := n.v.line
	if w == 0 {
		w = max(r*2/7, 2)
	}
	f, moving := ctx.Retained[easedValue]().at(ctx.Now())
	paintArc(ctx, c, r, w, 0, 2*math.Pi, n.track)
	paintArc(ctx, c, r, w, 0, float32(f)*2*math.Pi, n.tint)
	if moving {
		ctx.Animate(50 * time.Millisecond)
	}
}

// --- gauge ------------------------------------------------------------------------

// GaugeView shows a value within a range on a ring. It is created by
// [Gauge].
type GaugeView struct {
	value, min, max float64
	capacity        bool
	current         string
	minLabel        string
	maxLabel        string
	tint            Color
	hasTint         bool
	size, line      float32
	name, key       string
	flex            float32
}

// Gauge returns value between min and max on a ring, the counterpart of
// SwiftUI's Gauge in its circular styles. By default the ring is open at the
// bottom, like a speedometer, and filled from its left end to the value, with
// [GaugeView.MinMaxLabels] in the gap; [GaugeView.Capacity] closes it into a
// ring filled clockwise from twelve o'clock, for how full something is. The
// [GaugeView.CurrentValueLabel] stands in the middle.
//
//	ui.Gauge(21.5, 10, 30).CurrentValueLabel("21.5°").MinMaxLabels("10", "30")
//	ui.Gauge(0.62, 0, 1).Capacity(true).CurrentValueLabel("62 %")
//
// A value outside the range shows at its nearer end. A new value is eased
// to.
func Gauge(value, min, max float64) GaugeView {
	checkFinite("Gauge", value)
	checkFinite("Gauge", min)
	checkFinite("Gauge", max)
	return GaugeView{value: value, min: min, max: max, size: 72}
}

// Capacity closes the ring and fills it clockwise from twelve o'clock, as
// SwiftUI's accessoryCircularCapacity style does.
func (v GaugeView) Capacity(b bool) GaugeView { v.capacity = b; return v }

// CurrentValueLabel sets the text in the middle of the ring.
func (v GaugeView) CurrentValueLabel(s string) GaugeView { v.current = s; return v }

// MinMaxLabels sets the texts at the two ends of an open ring.
func (v GaugeView) MinMaxLabels(min, max string) GaugeView {
	v.minLabel, v.maxLabel = min, max
	return v
}

// Tint sets the colour of the filled part; the track is a quarter of it. The
// default is [ColorAccent].
func (v GaugeView) Tint(c Color) GaugeView { v.tint, v.hasTint = c, true; return v }

// Size sets the diameter; the default is 72.
func (v GaugeView) Size(px float32) GaugeView { v.size = max(px, 16); return v }

// LineWidth sets the width of the ring; the default is a tenth of the size.
func (v GaugeView) LineWidth(px float32) GaugeView { v.line = max(px, 1); return v }

// Label sets the accessible name.
func (v GaugeView) Label(s string) GaugeView { v.name = s; return v }

// Key sets the reconciliation key.
func (v GaugeView) Key(s string) GaugeView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v GaugeView) Flex(f float32) GaugeView { v.flex = f; return v }

// ViewType implements gift.View.
func (v GaugeView) ViewType() gift.TypeID { return gaugeType }

func (v GaugeView) fraction() float64 {
	if v.max <= v.min {
		return 0
	}
	return math.Max(0, math.Min(1, (v.value-v.min)/(v.max-v.min)))
}

func (v GaugeView) lineWidth() float32 {
	if v.line > 0 {
		return v.line
	}
	return max(v.size/10, 2)
}

// Build implements gift.View.
func (v GaugeView) Build(*gift.BuildContext) gift.Element {
	tint := Color(ColorAccent)
	if v.hasTint {
		tint = v.tint
	}
	n := &gaugeNode{v: v, tint: ResolveColor(tint), track: ResolveColor(Fade(tint, 0.25))}
	kids := []gift.View{Text(v.current).FontSize(v.size * 0.24).Foreground(ColorLabel).MaxLines(1)}
	if !v.capacity {
		small := max(v.size*0.13, 9)
		kids = append(kids,
			Text(v.minLabel).FontSize(small).Foreground(ColorSecondaryLabel).MaxLines(1),
			Text(v.maxLabel).FontSize(small).Foreground(ColorSecondaryLabel).MaxLines(1))
	}
	return gift.Element{Key: v.key, Flex: v.flex, Label: v.name, Layouter: n, Painter: n, Children: kids}
}

type gaugeNode struct {
	v           GaugeView
	tint, track Color
}

func (n *gaugeNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	ctx.Retained[easedValue]().set(ctx.Now(), n.v.fraction())
	size := c.Constrain(geom.Sz(n.v.size, n.v.size))
	r := min(size.W, size.H) / 2
	w := n.v.lineWidth()
	inner := 2 * (r - w - 2)
	cs := ctx.Measure(0, geom.Constraints{Max: geom.Sz(max(inner, 0), max(inner, 0))})
	ctx.Place(0, geom.Pt((size.W-cs.W)/2, (size.H-cs.H)/2))
	if ctx.ChildCount() == 3 {
		// The two ends of the ring, at half past seven and half past four;
		// each label stands centred under its end.
		c := geom.Pt(size.W/2, size.H/2)
		mid := r - w/2
		for i, a := range [2]float32{openStart, openStart + openSweep} {
			s := ctx.Measure(1+i, geom.Constraints{Max: geom.Sz(r, r)})
			p := onCircle(c, mid, a)
			ctx.Place(1+i, geom.Pt(p.X-s.W/2, p.Y+w/2+1))
		}
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

func (n *gaugeNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	r := min(b.Width(), b.Height()) / 2
	w := n.v.lineWidth()
	f, moving := ctx.Retained[easedValue]().at(ctx.Now())
	if n.v.capacity {
		paintArc(ctx, c, r, w, 0, 2*math.Pi, n.track)
		paintArc(ctx, c, r, w, 0, float32(f)*2*math.Pi, n.tint)
	} else {
		paintArc(ctx, c, r, w, openStart, openSweep, n.track)
		// A value at the very start still shows as a dot, as SwiftUI's does.
		paintArc(ctx, c, r, w, openStart, max(float32(f)*openSweep, 0.001), n.tint)
	}
	ctx.PaintChildren()
	if moving {
		ctx.Animate(50 * time.Millisecond)
	}
}

// --- circular slider -----------------------------------------------------------------

// CircularSliderView picks a number from a range on an open ring. It is
// created by [CircularSlider].
type CircularSliderView struct {
	base
	value, min, max, step float64
	onChange              func(float64)
	tint                  Color
	hasTint               bool
	size, line            float32
	name                  string
	disabled              bool
}

// CircularSlider returns a knob: an open ring, like a [Gauge], whose knob a
// finger drags round it, the circular counterpart of [Slider] for a
// thermostat or a volume dial. It shows no number; put one in its middle with
// a [ZStack].
//
//	t := ctx.State("t", 21.0)
//	ui.ZStack(
//		ui.CircularSlider(t.Get(), t.Set).Range(16, 28).Step(0.5),
//		ui.Text(fmt.Sprintf("%.1f°", ctx.Read(t))),
//	).Align(geom.Center)
//
// A press on the ring takes the knob there and a drag moves it round; a drag
// into the gap at the bottom holds it at the nearer end and never jumps it
// across. Focused, the arrow keys step and home and end go to the ends; the
// wheel steps too. The range is 0 to 1 by default.
func CircularSlider(value float64, onChange func(float64)) CircularSliderView {
	return CircularSliderView{value: checkFinite("CircularSlider", value), max: 1, onChange: onChange, size: 140}
}

// Range sets the smallest and the largest value.
func (v CircularSliderView) Range(min, max float64) CircularSliderView {
	checkFinite("CircularSlider.Range", min)
	checkFinite("CircularSlider.Range", max)
	if min < max {
		v.min, v.max = min, max
	}
	return v
}

// Step makes the value move in steps of s from the minimum; zero is
// continuous, and the keys then step by a hundredth of the range.
func (v CircularSliderView) Step(s float64) CircularSliderView {
	if isFinite64(s) && s >= 0 {
		v.step = s
	}
	return v
}

// Tint sets the colour of the filled part of the ring; the default is
// [ColorAccent].
func (v CircularSliderView) Tint(c Color) CircularSliderView { v.tint, v.hasTint = c, true; return v }

// Size sets the diameter; the default is 140.
func (v CircularSliderView) Size(px float32) CircularSliderView { v.size = max(px, 48); return v }

// LineWidth sets the width of the ring; the default is 10.
func (v CircularSliderView) LineWidth(px float32) CircularSliderView {
	v.line = max(px, 2)
	return v
}

// Label sets the accessible name.
func (v CircularSliderView) Label(s string) CircularSliderView { v.name = s; return v }

// Disabled takes the slider out of input.
func (v CircularSliderView) Disabled(b bool) CircularSliderView { v.disabled = b; return v }

// Key sets the reconciliation key.
func (v CircularSliderView) Key(s string) CircularSliderView { v.setKey(s); return v }

// Flex sets the flex factor in a stack.
func (v CircularSliderView) Flex(f float32) CircularSliderView { v.setFlex(f); return v }

// ViewType implements gift.View.
func (v CircularSliderView) ViewType() gift.TypeID { return circularSliderType }

func (v CircularSliderView) lineWidth() float32 {
	if v.line > 0 {
		return v.line
	}
	return 10
}

func (v CircularSliderView) fraction() float64 {
	return math.Max(0, math.Min(1, (v.value-v.min)/(v.max-v.min)))
}

// valueAt is the value at fraction f, on the grid of the step.
func (v CircularSliderView) valueAt(f float64) float64 {
	x := v.min + f*(v.max-v.min)
	if v.step > 0 {
		x = v.min + math.Round((x-v.min)/v.step)*v.step
	}
	return math.Max(v.min, math.Min(v.max, x))
}

func (v CircularSliderView) set(x float64) {
	if x != v.value && v.onChange != nil {
		v.onChange(x)
	}
}

// Build implements gift.View.
func (v CircularSliderView) Build(*gift.BuildContext) gift.Element {
	tint := Color(ColorAccent)
	if v.hasTint {
		tint = v.tint
	}
	n := &circularSliderNode{
		v:         v,
		fill:      ResolveColor(tint),
		track:     ResolveColor(defaultSliderTrack),
		knob:      ResolveColor(defaultSliderKnob),
		border:    resolveBorder(defaultSliderKnobBorder),
		focusRing: resolveBorder(defaultSliderFocusRing),
	}
	if v.disabled {
		n.fill = ResolveColor(Fade(tint, 0.35))
		n.track = ResolveColor(ColorControlDisabled)
	}
	return gift.Element{
		Key: v.key, Flex: v.flex, Label: v.name,
		Layouter: n, Painter: n, Interactor: n,
		Focusable: !v.disabled, Disabled: v.disabled,
	}
}

type circularSliderNode struct {
	v                 CircularSliderView
	fill, track, knob Color
	border, focusRing Border
}

// circularState is the drag under way: the fraction it last set, which a
// drag through the gap holds on to.
type circularState struct {
	grabbed bool
	last    float64
}

func (n *circularSliderNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(n.v.size, n.v.size))
}

// geometry is the centre of the ring, the radius of its outer edge and its
// width in the bounds b, with room for the knob, which stands out of it.
func (n *circularSliderNode) geometry(b geom.Rect) (geom.Point, float32, float32) {
	w := n.v.lineWidth()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	r := min(b.Width(), b.Height())/2 - n.knobRadius() + w/2
	return c, r, w
}

func (n *circularSliderNode) knobRadius() float32 { return max(n.v.lineWidth()*1.2, 12) }

func (n *circularSliderNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	c, r, w := n.geometry(b)
	f := float32(n.v.fraction())
	paintArc(ctx, c, r, w, openStart, openSweep, n.track)
	paintArc(ctx, c, r, w, openStart, max(f*openSweep, 0.001), n.fill)
	// The knob of a Slider, on the centre line of the ring at the value.
	k := n.knobRadius()
	p := onCircle(c, r-w/2, openStart+f*openSweep)
	knob := geom.Rc(p.X-k, p.Y-k, p.X+k, p.Y+k)
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: knob.Translate(geom.Pt(0, 1.5)), Color: RGBA(0, 0, 0, 40), CornerRadius: k, Blur: 6})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: knob, Color: n.knob, CornerRadius: k})
	paintBorder(ctx, styleSpec{border: n.border, radius: k}, knob)
	if ia := ctx.Interaction(); ia.FocusVisible && !ia.Disabled {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: k + 3}, knob.Outset(geom.InsetsAll(3)))
	}
}

// AccessibleValue implements [gift.ValueReporter]: the value.
func (n *circularSliderNode) AccessibleValue(gift.ControlState) string {
	return strconv.FormatFloat(n.v.value, 'f', -1, 64)
}

// fractionAt is the fraction of the ring at the device point p: the angle of
// p round the centre, along the open ring; in the gap, the nearer end.
func (n *circularSliderNode) fractionAt(ctx *gift.EventContext, p geom.Point) (float64, float32) {
	b := ctx.DeviceBounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	dx, dy := float64(p.X-c.X), float64(p.Y-c.Y)
	a := math.Atan2(dx, -dy) // clockwise from twelve o'clock
	rel := math.Mod(a-float64(openStart)+4*math.Pi, 2*math.Pi)
	sweep := float64(openSweep)
	f := rel / sweep
	if rel > sweep {
		f = 1
		if rel > sweep+(2*math.Pi-sweep)/2 {
			f = 0
		}
	}
	return f, float32(math.Hypot(dx, dy))
}

func (n *circularSliderNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.Retained[circularState]()
	v := n.v
	if v.disabled {
		return false
	}
	switch e.Kind {
	case gift.EventPointerDown:
		f, d := n.fractionAt(ctx, e.Pos)
		_, r, w := n.geometry(ctx.Bounds())
		// The ring and a hand's breadth either side of it; the middle is
		// left to whatever stands there.
		if d < r-w-24 || d > r+24 {
			return false
		}
		ctx.RequestFocus()
		st.grabbed, st.last = true, f
		v.set(v.valueAt(f))
		ctx.Repaint()
		return true
	case gift.EventPointerMove:
		if !st.grabbed {
			return false
		}
		f, _ := n.fractionAt(ctx, e.Pos)
		if math.Abs(f-st.last) > 0.5 {
			// Through the gap: stay at the end it was near.
			f = math.Round(st.last)
		}
		st.last = f
		v.set(v.valueAt(f))
		ctx.Repaint()
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.grabbed {
			return false
		}
		st.grabbed = false
		return true
	case gift.EventWheel:
		if e.Delta.Y == 0 {
			return false
		}
		n.stepBy(math.Copysign(1, float64(e.Delta.Y)))
		return true
	case gift.EventKeyDown:
		switch e.Key {
		case gift.KeyLeft, gift.KeyDown:
			n.stepBy(-1)
		case gift.KeyRight, gift.KeyUp:
			n.stepBy(1)
		case gift.KeyPageDown:
			n.stepBy(-10)
		case gift.KeyPageUp:
			n.stepBy(10)
		case gift.KeyHome:
			v.set(v.min)
		case gift.KeyEnd:
			v.set(v.max)
		default:
			return false
		}
		return true
	}
	return false
}

// stepBy moves the value by k steps, or by k hundredths of the range for a
// continuous slider.
func (n *circularSliderNode) stepBy(k float64) {
	v := n.v
	s := v.step
	if s <= 0 {
		s = (v.max - v.min) / 100
	}
	x := math.Max(v.min, math.Min(v.max, v.value+k*s))
	if v.step > 0 {
		x = v.valueAt((x - v.min) / (v.max - v.min))
	}
	v.set(x)
}
