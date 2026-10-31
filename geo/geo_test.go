package geo_test

import (
	"math"
	"testing"

	"github.com/worldiety/gift/geo"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestProjectionMapsTheCornersOfTheSquare(t *testing.T) {
	for _, c := range []struct {
		in   geo.Coordinate
		x, y float64
	}{
		{geo.LatLon(0, 0), 0.5, 0.5},
		{geo.LatLon(geo.MaxLatitude, -180), 0, 0},
		{geo.LatLon(-geo.MaxLatitude, 180), 1, 1},
		// Clamped, not infinite.
		{geo.LatLon(90, 0), 0.5, 0},
	} {
		p := geo.Project(c.in)
		if !near(p.X, c.x, 1e-12) || !near(p.Y, c.y, 1e-12) {
			t.Errorf("Project(%v) = %v, want (%v, %v)", c.in, p, c.x, c.y)
		}
	}
}

func TestProjectionRoundTrips(t *testing.T) {
	for _, c := range []geo.Coordinate{
		geo.LatLon(53.1435, 8.2146), geo.LatLon(-33.8688, 151.2093),
		geo.LatLon(64.1466, -21.9426), geo.LatLon(0.0001, -179.9999),
	} {
		back := geo.Project(c).Coordinate()
		if !near(back.Lat, c.Lat, 1e-9) || !near(back.Lon, c.Lon, 1e-9) {
			t.Errorf("%v came back as %v", c, back)
		}
	}
}

// TestTheTileOfOldenburg checks the projection against the tile numbers the
// Protomaps extract of the project plan, section 37, was cut by: the centre
// of Oldenburg is in tile 17131/10652 at zoom 15.
func TestTheTileOfOldenburg(t *testing.T) {
	p := geo.Project(geo.LatLon(53.1435, 8.2146))
	n := math.Exp2(15)
	if x, y := int(p.X*n), int(p.Y*n); x != 17131 || y != 10652 {
		t.Fatalf("tile %d/%d, want 17131/10652", x, y)
	}
}

func TestAPointInTheNextWorldNamesTheSamePlace(t *testing.T) {
	p := geo.Project(geo.LatLon(10, 170))
	q := geo.Point{X: p.X + 1, Y: p.Y}
	if c := q.Coordinate(); !near(c.Lon, 170, 1e-9) {
		t.Fatalf("longitude %v, want 170", c.Lon)
	}
}

func TestDistanceOfOneDegreeOfLatitude(t *testing.T) {
	d := geo.Distance(geo.LatLon(53, 8), geo.LatLon(54, 8))
	if !near(d, 111195, 50) {
		t.Fatalf("one degree is %v m", d)
	}
}

func TestRects(t *testing.T) {
	r := geo.RectOf(geo.LatLon(53.05, 8.05), geo.LatLon(53.23, 8.35))
	if !r.Contains(geo.LatLon(53.14, 8.2)) || r.Contains(geo.LatLon(52, 8.2)) {
		t.Fatal("Contains")
	}
	if !r.Intersects(geo.World) || r.Intersect(geo.World) != r {
		t.Fatal("the world does not contain Oldenburg")
	}
	g := geo.RegionOf(r).Rect()
	if !near(g.Min.Lat, r.Min.Lat, 1e-12) || !near(g.Max.Lon, r.Max.Lon, 1e-12) {
		t.Fatalf("region round trip: %v", g)
	}
	nw, se := r.Projected()
	if !(nw.X < se.X && nw.Y < se.Y) {
		t.Fatalf("projected corners out of order: %v %v", nw, se)
	}
}
