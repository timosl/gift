package imageviewer

import (
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift/geom"
)

// photo is a 3:2 picture in an 800 by 480 viewport: it fits at 720 by 480,
// with bars at the sides.
func photo() zoomer {
	m := newZoomer()
	m.maxZoom = 4
	m.setViewport(800, 480, 1.5)
	return m
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestPinchKeepsThePointUnderTheFingers(t *testing.T) {
	m := photo()
	// Already larger than the viewport both ways: a picture that is
	// narrower than the viewport stays centred on that axis, as in Photos,
	// and the point under the fingers then moves on purpose.
	m.z = 1.2
	m.begin(0)
	c := geom.Pt(600, 300)
	before := m.rectAt(0)
	// The point of the picture under c, as a fraction of the picture.
	fx := (float64(c.X) - float64(before.Min.X)) / float64(before.Width())
	fy := (float64(c.Y) - float64(before.Min.Y)) / float64(before.Height())
	for range 10 {
		m.pinch(c, c, 1.1)
	}
	after := m.rectAt(0)
	gx := (float64(c.X) - float64(after.Min.X)) / float64(after.Width())
	gy := (float64(c.Y) - float64(after.Min.Y)) / float64(after.Height())
	if !near(fx, gx, 0.002) || !near(fy, gy, 0.002) {
		t.Errorf("the point under the fingers moved from (%.3f, %.3f) to (%.3f, %.3f)", fx, fy, gx, gy)
	}
	if !near(m.z, 1.2*math.Pow(1.1, 10), 1e-9) {
		t.Errorf("z = %v", m.z)
	}
}

func TestAPinchPastTheLimitsResistsAndComesBack(t *testing.T) {
	m := photo()
	m.begin(0)
	for range 40 {
		m.pinch(geom.Pt(400, 240), geom.Pt(400, 240), 1.1) // raw zoom 45
	}
	if m.z <= 4 || m.z >= 6 {
		t.Fatalf("pinched far past 4, the zoom shows %v; want a little past 4 and at most 6", m.z)
	}
	m.end(0, 0, 0, geom.Pt(400, 240))
	if z, _, _, moving := m.at(2 * time.Second); !near(z, 4, 1e-3) || moving {
		t.Errorf("after the release the zoom settles at %v (moving %v), want 4", z, moving)
	}
	// Below the fit it shrinks to half at most and comes back centred.
	m = photo()
	m.begin(0)
	for range 40 {
		m.pinch(geom.Pt(100, 100), geom.Pt(100, 100), 0.9)
	}
	if m.z >= 1 || m.z <= 0.5 {
		t.Fatalf("pinched far below the fit, the zoom shows %v", m.z)
	}
	m.end(0, 0, 0, geom.Pt(100, 100))
	z, ox, oy, _ := m.at(2 * time.Second)
	if !near(z, 1, 1e-3) || !near(ox, 0, 0.5) || !near(oy, 0, 0.5) {
		t.Errorf("after the release: z %v, offset (%v, %v), want the centred fit", z, ox, oy)
	}
}

func TestAPanPastTheEdgeRubberBandsAndGlidesBack(t *testing.T) {
	m := photo()
	m.z = 2 // 1440 by 960: 320 px of travel each way horizontally
	m.begin(0)
	m.pan(500, 0, nil)
	if m.ox <= 320 || m.ox >= 320+180 {
		t.Fatalf("panned 180 px past the edge, the picture shows %v; want between 320 and 500", m.ox)
	}
	m.end(0, 0, 0, geom.Point{})
	if _, ox, _, moving := m.at(3 * time.Second); !near(ox, 320, 0.5) || moving {
		t.Errorf("after the release the offset settles at %v, want the edge at 320", ox)
	}
	// A fling inside the edges decelerates and stays inside.
	m.begin(3 * time.Second)
	m.pan(-320, 0, nil)
	m.end(3*time.Second, 2000, 0, geom.Point{})
	if r := m.gx.Rest(); r != 320 {
		t.Errorf("a fast fling rests at %v, want the edge it runs into", r)
	}
}

func TestPanOverflowIsHandedOn(t *testing.T) {
	m := photo()
	m.z = 2
	m.begin(0)
	var handed float64
	m.pan(-400, 0, func(rest float64) bool { handed = rest; return true })
	if !near(m.ox, -320, 1e-9) || !near(handed, -80, 1e-9) {
		t.Errorf("offset %v, handed on %v; want the edge and the 80 px past it", m.ox, handed)
	}
}

func TestDoubleTapZoomsAboutTheTapAndBack(t *testing.T) {
	m := photo()
	p := geom.Pt(700, 100)
	m.toggle(0, p, 0)
	z, ox, oy, _ := m.at(2 * time.Second)
	if !near(z, 2, 1e-3) {
		t.Fatalf("a double tap zoomed to %v, want 2 (the fill is 1.11)", z)
	}
	// Towards the top right: the picture moved left and down.
	if ox >= 0 || oy <= 0 {
		t.Errorf("offset (%v, %v) after a tap at the top right", ox, oy)
	}
	m.toggle(2*time.Second, p, 0)
	if z, _, _, _ := m.at(4 * time.Second); !near(z, 1, 1e-3) {
		t.Errorf("the second double tap went to %v, want back to the fit", z)
	}
}

func TestWheelZoomStaysInsideTheLimits(t *testing.T) {
	m := photo()
	for range 30 {
		m.scale(0, 1.25, geom.Pt(400, 240))
	}
	if m.z != 4 {
		t.Errorf("z = %v, want the limit", m.z)
	}
	for range 30 {
		m.scale(0, 0.8, geom.Pt(10, 10))
	}
	if m.z != 1 || m.ox != 0 || m.oy != 0 {
		t.Errorf("zoomed out fully: z %v, offset (%v, %v)", m.z, m.ox, m.oy)
	}
}
