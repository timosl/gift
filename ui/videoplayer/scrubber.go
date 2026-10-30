package videoplayer

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/video"
)

var scrubberType = gift.RegisterType("videoplayer.scrubber")

// Metrics of the scrubber, in logical pixels.
const (
	// scrubberHeight is the height of the control and therefore of the
	// touch target: the track is thin, the place a finger can find it is
	// not.
	scrubberHeight = float32(44)
	// scrubberTrack and scrubberTrackActive are the thickness of the track
	// at rest and under the finger, which is how iOS shows that the bar
	// has been taken hold of without a knob covering the picture.
	scrubberTrack       = float32(6)
	scrubberTrackActive = float32(12)
)

// scrubber is the position bar of a [PlayerView]. It is not a [ui.Slider],
// for two reasons: a slider reports a value the application stores and draws
// it back, while the position of a playing video changes every frame without
// a build; and a seek is too expensive to issue for every pixel a finger
// moves, so the bar needs to know where a drag ends, which a slider does not
// report.
type scrubber struct {
	p       *video.Player
	onBegin func()
	onMove  func(fraction float64)
	onEnd   func(fraction float64)
	label   string
}

func (s scrubber) ViewType() gift.TypeID { return scrubberType }

func (s scrubber) Build(*gift.BuildContext) gift.Element {
	n := &scrubberNode{s: s}
	return gift.Element{Key: "scrubber", Label: s.label, Layouter: n, Painter: n, Interactor: n, Flex: 1, Focusable: true}
}

type scrubberNode struct{ s scrubber }

func (n *scrubberNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	w := float32(200)
	if c.HasBoundedWidth() {
		w = c.Max.W
	}
	ctx.ReportOverflow(geom.Size{})
	return c.Constrain(geom.Sz(w, scrubberHeight))
}

// fraction is where the bar stands: the finger while it is held, the clock
// otherwise.
func (n *scrubberNode) fraction(st gift.ControlState) float32 {
	if st.Grabbed {
		return st.Target
	}
	d := n.s.p.Duration()
	if d <= 0 {
		return 0
	}
	return float32(min(max(float64(n.s.p.CurrentTime())/float64(d), 0), 1))
}

func (n *scrubberNode) Paint(ctx *gift.PaintContext) {
	st := ctx.ControlState()
	b := ctx.Bounds()
	th := scrubberTrack
	if st.Grabbed {
		th = scrubberTrackActive
	}
	cy := (b.Min.Y + b.Max.Y) / 2
	track := geom.Rc(b.Min.X, cy-th/2, b.Max.X, cy+th/2)
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: track, Color: ui.RGBA(255, 255, 255, 90), CornerRadius: th / 2})
	if f := n.fraction(st); f > 0 {
		done := track
		done.Max.X = track.Min.X + track.Width()*f
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: done, Color: ui.RGB(255, 255, 255), CornerRadius: th / 2})
	}
	if n.s.p.TimeControlStatus() == video.Playing {
		ctx.Animate(250 * time.Millisecond)
	}
}

// HandleEvent follows a finger anywhere on the bar. The press stores the
// fraction in [gift.ControlState.Target], so that the bar draws the finger
// and not the clock until the release; the release reports where it ended.
func (n *scrubberNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.ControlState()
	at := func() float32 {
		d := ctx.DeviceBounds()
		if d.Width() <= 0 {
			return 0
		}
		return min(max((e.Pos.X-d.Min.X)/d.Width(), 0), 1)
	}
	switch e.Kind {
	case gift.EventPointerDown:
		st.Grabbed, st.Target = true, at()
		ctx.SetControlState(st)
		ctx.Repaint()
		if n.s.onBegin != nil {
			n.s.onBegin()
		}
		if n.s.onMove != nil {
			n.s.onMove(float64(st.Target))
		}
		return true
	case gift.EventPointerMove:
		if !st.Grabbed {
			return false
		}
		st.Target = at()
		ctx.SetControlState(st)
		ctx.Repaint()
		if n.s.onMove != nil {
			n.s.onMove(float64(st.Target))
		}
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.Grabbed {
			return false
		}
		st.Grabbed = false
		ctx.SetControlState(st)
		ctx.Repaint()
		if n.s.onEnd != nil {
			n.s.onEnd(float64(st.Target))
		}
		return true
	case gift.EventKeyDown:
		// Arrow keys step five seconds, as on every player with a keyboard.
		d := n.s.p.Duration()
		switch e.Key {
		case gift.KeyLeft:
			n.s.p.Seek(n.s.p.CurrentTime() - 5*time.Second)
		case gift.KeyRight:
			n.s.p.Seek(min(n.s.p.CurrentTime()+5*time.Second, d))
		default:
			return false
		}
		ctx.Repaint()
		return true
	}
	return false
}
