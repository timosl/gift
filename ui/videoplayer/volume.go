package videoplayer

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

var volumeType = gift.RegisterType("videoplayer.volume")

// Metrics of the volume bar, in logical pixels.
const (
	volumeTrack = float32(4)
	volumeKnob  = float32(18)
)

// volumeBar is the volume slider of a [PlayerView]: a thin white track with a
// white knob, which is what sits on a picture in iOS. A [ui.Slider] draws its
// knob in the theme's surface colour, a dark disc on the dark pill here, and
// the knob is not one of its modifiers.
type volumeBar struct {
	value    float64
	onChange func(float64)
}

func (v volumeBar) ViewType() gift.TypeID { return volumeType }

func (v volumeBar) Build(*gift.BuildContext) gift.Element {
	n := &volumeNode{v: v}
	return gift.Element{Key: "volume", Label: "Volume", Layouter: n, Painter: n, Interactor: n, Focusable: true}
}

type volumeNode struct{ v volumeBar }

func (n *volumeNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	ctx.ReportOverflow(geom.Size{})
	return c.Constrain(geom.Sz(volumeWidth, barButton))
}

// span is the part of the bounds the knob centre travels along.
func span(b geom.Rect) (x0, x1 float32) {
	return b.Min.X + volumeKnob/2, b.Max.X - volumeKnob/2
}

func (n *volumeNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	x0, x1 := span(b)
	cy := (b.Min.Y + b.Max.Y) / 2
	f := float32(min(max(n.v.value, 0), 1))
	kx := x0 + (x1-x0)*f
	track := geom.Rc(x0, cy-volumeTrack/2, x1, cy+volumeTrack/2)
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: track, Color: ui.RGBA(255, 255, 255, 90), CornerRadius: volumeTrack / 2})
	if kx > x0 {
		done := geom.Rc(x0, track.Min.Y, kx, track.Max.Y)
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: done, Color: ui.RGB(255, 255, 255), CornerRadius: volumeTrack / 2})
	}
	knob := geom.Rc(kx-volumeKnob/2, cy-volumeKnob/2, kx+volumeKnob/2, cy+volumeKnob/2)
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: knob, Color: ui.RGBA(0, 0, 0, 90), CornerRadius: volumeKnob / 2, Blur: 4})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: knob, Color: ui.RGB(255, 255, 255), CornerRadius: volumeKnob / 2})
	if ia := ctx.Interaction(); ia.FocusVisible {
		ring := knob.Inset(geom.InsetsAll(-3))
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: ring, Color: ui.RGB(255, 255, 255), CornerRadius: ring.Width() / 2, StrokeWidth: 2})
	}
}

// HandleEvent puts the knob under the finger, on a press anywhere on the bar
// and on every move while it is held.
func (n *volumeNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.ControlState()
	report := func() {
		d := ctx.DeviceBounds()
		s := deviceScale(ctx)
		x0, x1 := d.Min.X+volumeKnob/2*s, d.Max.X-volumeKnob/2*s
		if x1 <= x0 {
			return
		}
		v := float64(min(max((e.Pos.X-x0)/(x1-x0), 0), 1))
		if v != n.v.value && n.v.onChange != nil {
			n.v.onChange(v)
		}
	}
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		st.Grabbed = true
		ctx.SetControlState(st)
		report()
		return true
	case gift.EventPointerMove:
		if !st.Grabbed {
			return false
		}
		report()
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.Grabbed {
			return false
		}
		st.Grabbed = false
		ctx.SetControlState(st)
		return true
	case gift.EventKeyDown:
		step := 0.0
		switch e.Key {
		case gift.KeyLeft, gift.KeyDown:
			step = -0.1
		case gift.KeyRight, gift.KeyUp:
			step = 0.1
		default:
			return false
		}
		if n.v.onChange != nil {
			n.v.onChange(min(max(n.v.value+step, 0), 1))
		}
		return true
	}
	return false
}
