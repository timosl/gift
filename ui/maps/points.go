package maps

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geo"
)

// Point is one thing on a map: a photograph, a place of interest.
type Point struct {
	// ID identifies the point to the application; [MapView.OnSelect]
	// reports it.
	ID string
	// At is where the point is.
	At geo.Coordinate
	// Title is the name a pin shows.
	Title string
	// Photo is the picture of a photograph; nil for a place.
	Photo asset.Source
	// Time and Score choose the picture a cluster shows: the highest score,
	// then the newest, then the first of the slice.
	Time  time.Time
	Score float64
}

// Clustering configures how points that would cover each other merge.
type Clustering struct {
	// Radius is the distance in logical pixels within which points merge
	// into a cluster at a zoom: the size of a marker and some room. Zero
	// selects 80, for the 56 point markers of [PhotoMarkers]; a cluster sits
	// at the centre of what it holds, which moves it towards its neighbours,
	// and a radius of the marker's size alone lets them overlap.
	Radius float64
	// MaxZoom is the last zoom level at which points are clustered; above
	// it every point stands alone. Zero selects 17.
	MaxZoom int
	// Off shows every point, never merged. It is for a few points: a map
	// that shows thousands of them unmerged draws thousands of markers.
	Off bool
}

func (c Clustering) withDefaults() Clustering {
	if c.Radius <= 0 {
		c.Radius = 80
	}
	if c.MaxZoom <= 0 {
		c.MaxZoom = 17
	}
	return c
}

// Points is a set of points and its cluster index. Like [ui.Gallery] it is
// long lived and owned by the application, and the view only declares it:
// the index over a catalogue of photographs is built once and not per
// build.
//
// The index is built on a goroutine of its own, so [NewPoints] and
// [Points.Set] return at once; the map shows the points when the index is
// ready, and the previous index until then. It is the hierarchical greedy
// clustering of Mapbox's Supercluster: from the last level down, each zoom
// level merges the entries of the level above that lie within the radius,
// a cluster sits at the weighted centre of what it holds, and every entry
// knows its parent one level down. That relation is what makes a cluster
// splitting and merging an animation with a definite start and end.
type Points struct {
	cfg  Clustering
	idx  atomic.Pointer[clusterIndex]
	mu   sync.Mutex
	gen  uint64
	done chan struct{}
}

// NewPoints returns a set of pts, clustered by c, and starts building its
// index. pts is not copied and must not change afterwards; [Points.Set]
// replaces it.
func NewPoints(pts []Point, c Clustering) *Points {
	p := &Points{cfg: c.withDefaults()}
	p.Set(pts)
	return p
}

// Set replaces the points and rebuilds the index in the background.
func (p *Points) Set(pts []Point) {
	p.mu.Lock()
	p.gen++
	gen := p.gen
	done := make(chan struct{})
	p.done = done
	p.mu.Unlock()
	go func() {
		idx := buildIndex(pts, p.cfg)
		p.mu.Lock()
		if gen == p.gen {
			p.idx.Store(idx)
		}
		p.mu.Unlock()
		close(done)
	}()
}

// Wait blocks until the index of the last [Points.Set] is ready. A test and
// a small set of places use it; a map does not need it.
func (p *Points) Wait() {
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	<-done
}

// Len returns the number of points the current index holds.
func (p *Points) Len() int {
	if idx := p.idx.Load(); idx != nil {
		return len(idx.pts)
	}
	return 0
}

// Cluster is a point or a group of points as a map shows them at a zoom.
type Cluster struct {
	// At is the centre of what the cluster holds, weighted by count.
	At geo.Coordinate
	// Count is the number of points; one for a point alone.
	Count int
	// Representative is the point whose picture the cluster shows.
	Representative Point
	idx            *clusterIndex
	level, entry   int32
}

// Points returns the points of the cluster.
func (c Cluster) Points() []Point {
	if c.idx == nil {
		return nil
	}
	var out []Point
	c.idx.members(c.level, c.entry, func(i int32) { out = append(out, c.idx.pts[i]) })
	return out
}

// Bounds returns the smallest rectangle that holds the cluster's points.
func (c Cluster) Bounds() geo.Rect {
	var r geo.Rect
	first := true
	if c.idx == nil {
		return r
	}
	c.idx.members(c.level, c.entry, func(i int32) {
		if first {
			r, first = geo.Rect{Min: c.idx.pts[i].At, Max: c.idx.pts[i].At}, false
		} else {
			r = r.Extend(c.idx.pts[i].At)
		}
	})
	return r
}

// clusterIndex is the clusters of every level, built once and read only
// after: it is published with an atomic pointer and read by the UI.
type clusterIndex struct {
	pts    []Point
	cfg    Clustering
	levels []clusterLevel // index z is zoom level z; the last is the points
}

// clusterLevel is the entries of one zoom level.
type clusterLevel struct {
	x, y   []float64 // world coordinates, X in [0, 1)
	count  []int32
	rep    []int32 // the representative point
	parent []int32 // the entry one level down that holds this one
	// first and kids list the entries one level up that this one holds:
	// kids[first[i]:first[i+1]].
	first, kids []int32
	tree        *kdTree
}

func (l *clusterLevel) add(x, y float64, count, rep int32) int32 {
	l.x = append(l.x, x)
	l.y = append(l.y, y)
	l.count = append(l.count, count)
	l.rep = append(l.rep, rep)
	l.parent = append(l.parent, -1)
	return int32(len(l.x) - 1)
}

// better reports whether point a is a better picture for a cluster than b.
func better(pts []Point, a, b int32) bool {
	pa, pb := &pts[a], &pts[b]
	if pa.Score != pb.Score {
		return pa.Score > pb.Score
	}
	if !pa.Time.Equal(pb.Time) {
		return pa.Time.After(pb.Time)
	}
	return a < b
}

// buildIndex clusters pts on every level, O(n log n) per level.
func buildIndex(pts []Point, cfg Clustering) *clusterIndex {
	top := cfg.MaxZoom + 1
	if cfg.Off {
		top = 0
	}
	idx := &clusterIndex{pts: pts, cfg: cfg, levels: make([]clusterLevel, top+1)}
	l := &idx.levels[top]
	for i, p := range pts {
		if !p.At.Valid() {
			continue
		}
		w := geo.Project(p.At)
		l.add(w.Wrapped().X, w.Y, 1, int32(i))
	}
	l.tree = newKDTree(l.x, l.y)
	if cfg.Off {
		return idx
	}
	var near []int32
	for z := top - 1; z >= 0; z-- {
		up := &idx.levels[z+1]
		cur := &idx.levels[z]
		r := cfg.Radius / scaleOf(float64(z))
		seen := make([]bool, len(up.x))
		for i := range up.x {
			if seen[i] {
				continue
			}
			seen[i] = true
			near = up.tree.within(up.x[i], up.y[i], r, near[:0])
			count := up.count[i]
			wx, wy := up.x[i]*float64(count), up.y[i]*float64(count)
			rep := up.rep[i]
			merged := false
			for _, j := range near {
				if seen[j] {
					continue
				}
				seen[j] = true
				merged = true
				c := up.count[j]
				count += c
				wx += up.x[j] * float64(c)
				wy += up.y[j] * float64(c)
				if better(pts, up.rep[j], rep) {
					rep = up.rep[j]
				}
				up.parent[j] = int32(len(cur.x))
			}
			if merged {
				up.parent[i] = cur.add(wx/float64(count), wy/float64(count), count, rep)
			} else {
				up.parent[i] = cur.add(up.x[i], up.y[i], count, rep)
			}
		}
		cur.tree = newKDTree(cur.x, cur.y)
	}
	// Every level's list of the entries it holds, from the parents above.
	for z := 0; z < top; z++ {
		cur, up := &idx.levels[z], &idx.levels[z+1]
		cur.first = make([]int32, len(cur.x)+1)
		for _, p := range up.parent {
			cur.first[p+1]++
		}
		for i := 1; i < len(cur.first); i++ {
			cur.first[i] += cur.first[i-1]
		}
		cur.kids = make([]int32, len(up.parent))
		fill := append([]int32(nil), cur.first[:len(cur.x)]...)
		for j, p := range up.parent {
			cur.kids[fill[p]] = int32(j)
			fill[p]++
		}
	}
	return idx
}

// level returns the cluster level of a zoom.
func (idx *clusterIndex) level(zoom float64) int {
	return max(0, min(len(idx.levels)-1, int(math.Floor(zoom))))
}

// ancestor returns the entry at level to that holds entry e of level from.
func (idx *clusterIndex) ancestor(from int, e int32, to int) int32 {
	for z := from; z > to && e >= 0; z-- {
		e = idx.levels[z].parent[e]
	}
	return e
}

// members calls fn with every point an entry holds.
func (idx *clusterIndex) members(level, e int32, fn func(int32)) {
	if int(level) == len(idx.levels)-1 {
		fn(idx.levels[level].rep[e])
		return
	}
	l := &idx.levels[level]
	for _, k := range l.kids[l.first[e]:l.first[e+1]] {
		idx.members(level+1, k, fn)
	}
}

// cluster returns the public view of an entry.
func (idx *clusterIndex) cluster(level int, e int32) Cluster {
	l := &idx.levels[level]
	return Cluster{
		At:             geo.Point{X: l.x[e], Y: l.y[e]}.Coordinate(),
		Count:          int(l.count[e]),
		Representative: idx.pts[l.rep[e]],
		idx:            idx, level: int32(level), entry: e,
	}
}
