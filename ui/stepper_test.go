package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// stepperScene is a stepper bound to state, and the values it reported.
func stepperScene(t *testing.T, start float64, decorate func(ui.StepperView) ui.StepperView) (*gifttest.Harness, *[]float64) {
	t.Helper()
	got := new([]float64)
	root := func(ctx *gift.Context) gift.View {
		v := ctx.State("v", start)
		s := ui.Stepper(ctx.Read(v), func(x float64) { *got = append(*got, x); v.Set(x) }).Key("stepper")
		if decorate != nil {
			s = decorate(s)
		}
		return ui.VStack(s).Padding(20)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(300, 200), Font: loadTestFont(t)})
	h.Settle()
	return h, got
}

func halves(h *gifttest.Harness) (minus, plus geom.Point) {
	b := h.Find(gifttest.ByKey("stepper")).Bounds()
	cy := (b.Min.Y + b.Max.Y) / 2
	return geom.Pt(b.Min.X+b.Width()/4, cy), geom.Pt(b.Max.X-b.Width()/4, cy)
}

func TestATapOnAHalfStepsAndTheEndsStop(t *testing.T) {
	h, got := stepperScene(t, 2, func(s ui.StepperView) ui.StepperView { return s.Range(0, 3) })
	b := h.Find(gifttest.ByKey("stepper")).LayoutBounds()
	if b.Width() != 94 || b.Height() != 32 {
		t.Fatalf("a stepper is %v, want 94 by 32", b.Size())
	}
	minus, plus := halves(h)
	h.TapAt(plus)
	h.Advance(time.Second)
	h.TapAt(plus)
	h.Advance(time.Second)
	h.TapAt(minus)
	want := []float64{3, 2}
	if len(*got) != len(want) || (*got)[0] != 3 || (*got)[1] != 2 {
		t.Fatalf("plus, plus at the end, minus reported %v, want %v", *got, want)
	}
	if v := h.Find(gifttest.ByKey("stepper")).Value(); v != "2" {
		t.Fatalf("the stepper reports %q", v)
	}
}

func TestAStepperThatWrapsGoesRound(t *testing.T) {
	h, got := stepperScene(t, 3, func(s ui.StepperView) ui.StepperView { return s.Range(0, 3).Wraps(true) })
	_, plus := halves(h)
	h.TapAt(plus)
	if len(*got) != 1 || (*got)[0] != 0 {
		t.Fatalf("plus past the end reported %v, want the start", *got)
	}
}

func TestAHeldHalfRepeatsFasterAndFaster(t *testing.T) {
	h, got := stepperScene(t, 0, func(s ui.StepperView) ui.StepperView { return s.Range(0, 1000) })
	_, plus := halves(h)
	h.FingerDown(1, plus)
	h.AdvanceTicks(24, gifttest.FrameInterval) // 0.4 s: no repeat yet
	if len(*got) != 1 {
		t.Fatalf("within the delay the stepper stepped %d times, want once", len(*got))
	}
	h.AdvanceTicks(36, gifttest.FrameInterval) // to 1 s: about five repeats
	n1 := len(*got)
	if n1 < 4 || n1 > 8 {
		t.Fatalf("after a second the stepper stepped %d times", n1)
	}
	h.AdvanceTicks(60, gifttest.FrameInterval) // to 2 s
	n2 := len(*got)
	h.AdvanceTicks(60, gifttest.FrameInterval) // to 3 s: faster
	n3 := len(*got)
	h.FingerUp(1, plus)
	h.AdvanceTicks(30, gifttest.FrameInterval)
	if len(*got) != n3 {
		t.Fatal("the stepper went on after the finger lifted")
	}
	if n3-n2 < 15 || n3-n2 <= n2-n1 {
		t.Fatalf("the second second stepped %d times and the third %d, want the third faster", n2-n1, n3-n2)
	}
	if last := (*got)[len(*got)-1]; last != float64(len(*got)) {
		t.Fatalf("the last value is %v after %d steps", last, len(*got))
	}
}

func TestTheKeysStepAndATenthDoesNotDrift(t *testing.T) {
	h, got := stepperScene(t, 0, func(s ui.StepperView) ui.StepperView { return s.Range(0, 1).Step(0.1) })
	h.Find(gifttest.ByKey("stepper")).Focus()
	for range 10 {
		h.Key(gift.KeyRight)
	}
	if last := (*got)[len(*got)-1]; last != 1 {
		t.Fatalf("ten steps of a tenth reached %v, want 1", last)
	}
	h.Key(gift.KeyLeft)
	if last := (*got)[len(*got)-1]; last != 0.9 {
		t.Fatalf("one back reached %v, want 0.9", last)
	}
	h.Key(gift.KeyHome)
	if last := (*got)[len(*got)-1]; last != 0 {
		t.Fatalf("home reached %v", last)
	}
}
