package maps

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
)

// catalogue is n photographs with a fixed seed, in clumps around a few
// cities as a camera roll is: most near home, some on journeys.
func catalogue(n int) []Point {
	rng := rand.New(rand.NewPCG(37, 2026))
	cities := []geo.Coordinate{
		geo.LatLon(53.1435, 8.2146), geo.LatLon(53.5511, 9.9937), geo.LatLon(52.52, 13.405),
		geo.LatLon(48.137, 11.575), geo.LatLon(41.9028, 12.4964), geo.LatLon(38.7223, -9.1393),
		geo.LatLon(64.1466, -21.9426), geo.LatLon(40.7128, -74.006),
	}
	weights := []float64{0.5, 0.12, 0.1, 0.08, 0.06, 0.05, 0.05, 0.04}
	pts := make([]Point, n)
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range pts {
		u, c := rng.Float64(), 0
		for c < len(weights)-1 && u > weights[c] {
			u -= weights[c]
			c++
		}
		spread := 0.02 + 0.1*rng.Float64()*rng.Float64()
		pts[i] = Point{
			ID:   strconv.Itoa(i),
			At:   geo.LatLon(cities[c].Lat+rng.NormFloat64()*spread, cities[c].Lon+rng.NormFloat64()*spread*1.6),
			Time: t0.Add(time.Duration(rng.Int64N(int64(6 * 365 * 24 * time.Hour)))),
		}
	}
	return pts
}

func TestEveryLevelHoldsEveryPointOnce(t *testing.T) {
	pts := catalogue(5000)
	idx := buildIndex(pts, Clustering{}.withDefaults())
	if len(idx.levels) != 19 {
		t.Fatalf("%d levels, want 0 to 18", len(idx.levels))
	}
	for z := range idx.levels {
		l := &idx.levels[z]
		var sum int32
		for _, c := range l.count {
			sum += c
		}
		if int(sum) != len(pts) {
			t.Fatalf("level %d holds %d points, want %d", z, sum, len(pts))
		}
		if z < len(idx.levels)-1 {
			for e := range l.x {
				seen := map[int32]bool{}
				idx.members(int32(z), int32(e), func(i int32) { seen[i] = true })
				if len(seen) != int(l.count[e]) {
					t.Fatalf("level %d entry %d counts %d and holds %d", z, e, l.count[e], len(seen))
				}
			}
		}
	}
	// The world at level zero is a handful of clusters, not five thousand.
	if n := len(idx.levels[0].x); n > 20 {
		t.Errorf("%d clusters at level 0", n)
	}
	if n := len(idx.levels[18].x); n != len(pts) {
		t.Errorf("%d entries at the last level, want every point", n)
	}
}

func TestAClusterShowsItsNewestPhotograph(t *testing.T) {
	t0 := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	pts := []Point{
		{ID: "a", At: geo.LatLon(53.14, 8.21), Time: t0},
		{ID: "b", At: geo.LatLon(53.1401, 8.2101), Time: t0.Add(time.Hour)},
		{ID: "c", At: geo.LatLon(53.1402, 8.2102), Time: t0.Add(-time.Hour)},
	}
	idx := buildIndex(pts, Clustering{}.withDefaults())
	c := idx.cluster(5, 0)
	if c.Count != 3 || c.Representative.ID != "b" {
		t.Fatalf("cluster %d with %q, want 3 with b", c.Count, c.Representative.ID)
	}
	if got := len(c.Points()); got != 3 {
		t.Errorf("%d points", got)
	}
	if b := c.Bounds(); b.Min.Lat != 53.14 || b.Max.Lon != 8.2102 {
		t.Errorf("bounds %v", b)
	}
	// Splitting from level 5 to the last, every entry knows where it came
	// from.
	last := len(idx.levels) - 1
	for e := range idx.levels[last].x {
		if a := idx.ancestor(last, int32(e), 5); a != 0 {
			t.Errorf("entry %d descends from %d, want 0", e, a)
		}
	}
}

func TestTheQueryFindsWhatIsInTheBox(t *testing.T) {
	pts := catalogue(20000)
	idx := buildIndex(pts, Clustering{}.withDefaults())
	l := &idx.levels[len(idx.levels)-1]
	nw, se := geo.RectOf(geo.LatLon(53.0, 8.0), geo.LatLon(53.3, 8.4)).Projected()
	got := l.tree.rangeQuery(nw.X, nw.Y, se.X, se.Y, nil)
	want := 0
	for i := range l.x {
		if l.x[i] >= nw.X && l.x[i] <= se.X && l.y[i] >= nw.Y && l.y[i] <= se.Y {
			want++
		}
	}
	if len(got) != want || want == 0 {
		t.Fatalf("query found %d, brute force %d", len(got), want)
	}
	buf := make([]int32, 0, want)
	if a := testing.AllocsPerRun(20, func() { buf = l.tree.rangeQuery(nw.X, nw.Y, se.X, se.Y, buf[:0]) }); a != 0 {
		t.Errorf("the query allocates %v times", a)
	}
}

func BenchmarkBuild100k(b *testing.B) {
	pts := catalogue(100000)
	for b.Loop() {
		buildIndex(pts, Clustering{}.withDefaults())
	}
}

func BenchmarkQueryAViewport(b *testing.B) {
	pts := catalogue(100000)
	idx := buildIndex(pts, Clustering{}.withDefaults())
	// A city at zoom 12 on a full HD screen.
	v := newProjection(geo.Project(geo.LatLon(53.14, 8.21)), 12, 0, 1920, 1080)
	l := &idx.levels[12]
	a, c := v.toWorld(0, 0), v.toWorld(1920, 1080)
	buf := make([]int32, 0, 1024)
	b.ResetTimer()
	for b.Loop() {
		buf = l.tree.rangeQuery(math.Min(a.X, c.X), math.Min(a.Y, c.Y), math.Max(a.X, c.X), math.Max(a.Y, c.Y), buf[:0])
	}
	b.ReportMetric(float64(len(buf)), "entries")
}
