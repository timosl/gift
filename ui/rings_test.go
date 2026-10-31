package ui_test

import (
	"math"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

type arcOp struct {
	bounds       geom.Rect
	start, sweep float32
	width        float32
}

func arcs(h *gifttest.Harness) []arcOp {
	var out []arcOp
	l := h.List()
	for _, op := range h.Ops() {
		if op.Kind == render.OpArc {
			m := l.Material(op.Material)
			out = append(out, arcOp{op.Bounds, m.Arc.Start, m.Arc.Sweep, op.StrokeWidth})
		}
	}
	return out
}

func near32(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestAProgressRingFillsClockwiseAndEasesToANewValue(t *testing.T) {
	var set func(float64)
	root := func(ctx *gift.Context) gift.View {
		f := ctx.State("f", 0.25)
		set = f.Set
		return ui.VStack(ui.ProgressRing(ctx.Read(f)).Size(40).Key("ring")).Padding(10)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(200, 200), Font: loadTestFont(t)})
	h.Settle()
	a := arcs(h)
	if len(a) != 2 || !near32(a[0].sweep, 2*math.Pi) || !near32(a[1].sweep, math.Pi/2) || a[1].start != 0 {
		t.Fatalf("a quarter ring draws %+v, want a whole track and a quarter from twelve o'clock", a)
	}
	if b := h.Find(gifttest.ByKey("ring")).LayoutBounds(); b.Width() != 40 || a[0].bounds != b {
		t.Fatalf("the ring is %v and its arcs %v, want both 40 across", b, a[0].bounds)
	}
	h.App().BeginInput(h.Now())
	set(0.75)
	h.Settle()
	h.AdvanceTicks(6, gifttest.FrameInterval)
	if s := arcs(h)[1].sweep; !(s > math.Pi/2+0.01 && s < 3*math.Pi/2-0.01) {
		t.Fatalf("a tenth of a second after the change the ring sweeps %v, want on its way", s)
	}
	h.AdvanceTicks(60, gifttest.FrameInterval)
	if s := arcs(h)[1].sweep; !near32(s, 3*math.Pi/2) {
		t.Fatalf("at rest the ring sweeps %v, want three quarters", s)
	}
}

func TestAnIndeterminateRingIsASpinner(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{View: ui.ProgressRing(0).Indeterminate().Size(40), Size: geom.Sz(100, 100), Font: loadTestFont(t)})
	h.Settle()
	if len(arcs(h)) != 0 {
		t.Fatal("a spinner drew arcs")
	}
	if n := countSpokes(h); n != 8 {
		t.Fatalf("the spinner has %d spokes", n)
	}
}

func TestAGaugeIsOpenAtTheBottomWithItsLabelsAtTheEnds(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		View: ui.VStack(ui.Gauge(20, 10, 30).CurrentValueLabel("20°").MinMaxLabels("10", "30").Size(100).Key("gauge")).Padding(10),
		Size: geom.Sz(200, 200), Font: loadTestFont(t),
	})
	h.Settle()
	a := arcs(h)
	if len(a) != 2 || !near32(a[0].start, 5*math.Pi/4) || !near32(a[0].sweep, 3*math.Pi/2) || !near32(a[1].sweep, 3*math.Pi/4) {
		t.Fatalf("a gauge half way draws %+v, want an open track from half past seven and half of it", a)
	}
	g := h.Find(gifttest.ByKey("gauge")).LayoutBounds()
	lo := h.Find(gifttest.ByText("10")).LayoutBounds()
	hi := h.Find(gifttest.ByText("30")).LayoutBounds()
	cur := h.Find(gifttest.ByText("20°")).LayoutBounds()
	if !(lo.Max.X < hi.Min.X && lo.Min.Y > g.Min.Y+g.Height()/2 && hi.Min.Y > g.Min.Y+g.Height()/2) {
		t.Fatalf("the end labels are at %v and %v in a gauge at %v, want low left and right", lo, hi, g)
	}
	if c := centreOf(cur); math.Abs(float64(c.X-centreOf(g).X)) > 1 || math.Abs(float64(c.Y-centreOf(g).Y)) > 1 {
		t.Fatalf("the current value is at %v, want the middle of %v", cur, g)
	}
	// Capacity closes the ring and starts at twelve o'clock.
	k := gifttest.New(t, gifttest.Options{View: ui.Gauge(0.5, 0, 1).Capacity(true), Size: geom.Sz(200, 200), Font: loadTestFont(t)})
	k.Settle()
	if a := arcs(k); len(a) != 2 || a[1].start != 0 || !near32(a[0].sweep, 2*math.Pi) || !near32(a[1].sweep, math.Pi) {
		t.Fatalf("a capacity gauge half full draws %+v", a)
	}
}

// sliderScene is a circular slider bound to state, 140 across, centred at
// (100,100).
func sliderScene(t *testing.T, start float64, decorate func(ui.CircularSliderView) ui.CircularSliderView) (*gifttest.Harness, *float64) {
	t.Helper()
	val := new(float64)
	root := func(ctx *gift.Context) gift.View {
		v := ctx.State("v", start)
		*val = ctx.Read(v)
		s := ui.CircularSlider(v.Get(), v.Set).Key("knob")
		if decorate != nil {
			s = decorate(s)
		}
		return ui.ZStack(s)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(200, 200), Font: loadTestFont(t), Theme: ui.LightTheme()})
	h.Settle()
	return h, val
}

func TestACircularSliderFollowsTheAngleAndHoldsAtTheEndsOfItsGap(t *testing.T) {
	h, val := sliderScene(t, 0.5, nil)
	c := centreOf(h.Find(gifttest.ByKey("knob")).Bounds())
	r := float32(55)
	// At three o'clock the ring is five sixths along: it starts at half past
	// seven and sweeps three quarters of a turn.
	h.FingerDown(1, geom.Pt(c.X+r, c.Y))
	if math.Abs(*val-5.0/6) > 0.01 {
		t.Fatalf("a press at three o'clock set %v, want 5/6", *val)
	}
	// Round through the bottom, into the gap: it holds at the top end.
	for _, a := range []float64{100, 120, 140, 170, 200, 230} {
		rad := a * math.Pi / 180
		h.FingerMove(1, geom.Pt(c.X+r*float32(math.Sin(rad)), c.Y-r*float32(math.Cos(rad))))
	}
	if *val != 1 {
		t.Fatalf("dragged through the gap the slider is at %v, want held at 1", *val)
	}
	h.FingerUp(1, c)
	// A press in the middle is not the slider's.
	h.TapAt(c)
	if *val != 1 {
		t.Fatalf("a tap in the middle moved the slider to %v", *val)
	}
}

func TestTheKeysStepACircularSlider(t *testing.T) {
	h, val := sliderScene(t, 20, func(s ui.CircularSliderView) ui.CircularSliderView { return s.Range(16, 28).Step(0.5) })
	h.Find(gifttest.ByKey("knob")).Focus()
	h.Key(gift.KeyRight)
	h.Key(gift.KeyRight)
	if *val != 21 {
		t.Fatalf("two steps up reached %v, want 21", *val)
	}
	h.Key(gift.KeyEnd)
	if *val != 28 {
		t.Fatalf("end reached %v", *val)
	}
	h.Key(gift.KeyRight)
	if *val != 28 {
		t.Fatalf("a step past the end reached %v", *val)
	}
}
