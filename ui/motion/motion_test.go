package motion

import (
	"math"
	"testing"
	"time"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestRubberIsUIKitsCurve(t *testing.T) {
	// The slope at the edge is c, so a small pull shows as about half of it.
	if r := Rubber(1, 1000); !near(r, 0.55, 0.001) {
		t.Errorf("Rubber(1, 1000) = %v, want ≈0.55", r)
	}
	// It never reaches the viewport size.
	if r := Rubber(1e9, 400); r >= 400 || r < 399 {
		t.Errorf("Rubber(huge, 400) = %v, want just below 400", r)
	}
	if r := Rubber(-50, 400); r >= 0 {
		t.Errorf("Rubber keeps the sign: %v", r)
	}
	for _, x := range []float64{-300, -1, 0, 3, 77, 900} {
		if back := RubberInverse(Rubber(x, 400), 400); !near(back, x, 1e-6*math.Max(1, math.Abs(x))) {
			t.Errorf("RubberInverse(Rubber(%v)) = %v", x, back)
		}
	}
	if b := Band(5, 0, 10, 100); b != 5 {
		t.Errorf("Band inside = %v", b)
	}
	if b := Band(20, 0, 10, 100); !(b > 10 && b < 20) {
		t.Errorf("Band past hi = %v, want between the edge and the pull", b)
	}
}

func TestProjectMatchesApplesFormula(t *testing.T) {
	// 1000 pt/s at the normal rate rests 499 pt further on.
	if p := Project(1000, DecelerationNormal); !near(p, 499, 0.01) {
		t.Errorf("Project = %v, want 499", p)
	}
	d := Decay{Rate: DecelerationNormal}
	if r := d.Rest(0, 1000); !near(r, Project(1000, DecelerationNormal), 5) {
		t.Errorf("the continuous rest %v and Apple's projection disagree by more than 1 %%", r)
	}
	x, v := d.At(time.Second, 0, 1000)
	if !(x > 400 && x < 499) || !(v > 100 && v < 140) {
		t.Errorf("after one second: x %v, v %v", x, v)
	}
}

func TestSpringsReachTheirTarget(t *testing.T) {
	for name, s := range map[string]Spring{"smooth": Smooth, "snappy": Snappy, "bouncy": Bouncy, "edge": Edge, "creeping": {Response: 300 * time.Millisecond, Damping: 2}} {
		x0, v0 := 100.0, -50.0
		if x, v := s.At(0, x0, 0, v0); !near(x, x0, 1e-9) || !near(v, v0, 1e-6) {
			t.Errorf("%s at 0: %v %v, want the initial conditions", name, x, v)
		}
		if !s.Settled(3*time.Second, x0, 0, v0, 0.01) {
			x, v := s.At(3*time.Second, x0, 0, v0)
			t.Errorf("%s has not settled after 3 s: %v %v", name, x, v)
		}
		// The velocity is the derivative of the position.
		const h = time.Millisecond
		a, _ := s.At(100*time.Millisecond-h, x0, 0, v0)
		b, _ := s.At(100*time.Millisecond+h, x0, 0, v0)
		_, v := s.At(100*time.Millisecond, x0, 0, v0)
		if num := (b - a) / (2 * h.Seconds()); !near(num, v, math.Abs(v)*0.01+0.5) {
			t.Errorf("%s velocity %v, numeric %v", name, v, num)
		}
	}
	// Critically damped does not cross the target from a standing start.
	for ms := 0; ms < 2000; ms += 4 {
		if x, _ := Smooth.At(time.Duration(ms)*time.Millisecond, 100, 0, 0); x < -1e-9 {
			t.Fatalf("smooth overshot to %v at %d ms", x, ms)
		}
	}
	if d := Edge.Duration(100, 0, 0.5); d < 200*time.Millisecond || d > 700*time.Millisecond {
		t.Errorf("Edge settles a 100 pt pull in %v, want about a third of a second", d)
	}
}

func TestGlideBouncesOffTheEdgeItCrosses(t *testing.T) {
	g := NewGlide(50, 2000, 0, 100, Decay{Rate: DecelerationNormal}, Edge)
	if g.Rest() != 100 {
		t.Fatalf("a fling past hi rests at %v, want 100", g.Rest())
	}
	peak := 0.0
	for ms := 0; ms < 3000; ms += 4 {
		x, _ := g.At(time.Duration(ms) * time.Millisecond)
		peak = math.Max(peak, x)
	}
	if peak <= 100 || peak > 160 {
		t.Errorf("the overshoot peaks at %v, want a little past 100", peak)
	}
	if !g.Done(3*time.Second, 0.5) {
		t.Error("the glide has not come to rest")
	}
	// Released past an edge, it springs back however it was moving.
	g = NewGlide(-30, 0, 0, 100, Decay{Rate: DecelerationNormal}, Edge)
	if x, _ := g.At(2 * time.Second); !near(x, 0, 0.5) {
		t.Errorf("a release past lo ends at %v", x)
	}
	// A slow fling stays inside and decays freely.
	g = NewGlide(50, 20, 0, 100, Decay{Rate: DecelerationNormal}, Edge)
	if r := g.Rest(); !near(r, 50+20*0.499, 0.1) {
		t.Errorf("a slow fling rests at %v", r)
	}
}
