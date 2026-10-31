package maps

import "math"

// kdTree is a static k-d tree over points in the plane, after Vladimir
// Agafonkin's KDBush, which Supercluster builds on: the indices sorted in
// place into a balanced tree, leaves of up to kdLeaf points, and a range and
// radius query that appends to the caller's slice and allocates nothing.
type kdTree struct {
	ids  []int32
	x, y []float64
}

const kdLeaf = 64

// newKDTree indexes the points x[i], y[i]. The slices are kept, not copied.
func newKDTree(x, y []float64) *kdTree {
	t := &kdTree{ids: make([]int32, len(x)), x: x, y: y}
	for i := range t.ids {
		t.ids[i] = int32(i)
	}
	t.sort(0, len(t.ids)-1, 0)
	return t
}

func (t *kdTree) coord(i int32, axis int) float64 {
	if axis == 0 {
		return t.x[i]
	}
	return t.y[i]
}

// sort arranges ids[lo..hi] so that the median of each range is in its
// middle, split on the axis of its depth.
func (t *kdTree) sort(lo, hi, axis int) {
	if hi-lo <= kdLeaf {
		return
	}
	m := (lo + hi) / 2
	t.selectK(m, lo, hi, axis)
	t.sort(lo, m-1, 1-axis)
	t.sort(m+1, hi, 1-axis)
}

// selectK is Floyd and Rivest's selection: ids[k] becomes the k-th smallest
// of ids[lo..hi] along axis, with the smaller ones before it.
func (t *kdTree) selectK(k, lo, hi, axis int) {
	for hi > lo {
		if hi-lo > 600 {
			n := float64(hi - lo + 1)
			m := float64(k - lo + 1)
			z := math.Log(n)
			s := 0.5 * math.Exp(2*z/3)
			sd := 0.5 * math.Sqrt(z*s*(n-s)/n)
			if m-n/2 < 0 {
				sd = -sd
			}
			nlo := max(lo, int(math.Floor(float64(k)-m*s/n+sd)))
			nhi := min(hi, int(math.Floor(float64(k)+(n-m)*s/n+sd)))
			t.selectK(k, nlo, nhi, axis)
		}
		v := t.coord(t.ids[k], axis)
		i, j := lo, hi
		t.swap(lo, k)
		if t.coord(t.ids[hi], axis) > v {
			t.swap(lo, hi)
		}
		for i < j {
			t.swap(i, j)
			i++
			j--
			for t.coord(t.ids[i], axis) < v {
				i++
			}
			for t.coord(t.ids[j], axis) > v {
				j--
			}
		}
		if t.coord(t.ids[lo], axis) == v {
			t.swap(lo, j)
		} else {
			j++
			t.swap(j, hi)
		}
		if j <= k {
			lo = j + 1
		}
		if k <= j {
			hi = j - 1
		}
	}
}

func (t *kdTree) swap(i, j int) { t.ids[i], t.ids[j] = t.ids[j], t.ids[i] }

// rangeQuery appends the points inside the box to out.
func (t *kdTree) rangeQuery(minX, minY, maxX, maxY float64, out []int32) []int32 {
	type frame struct{ lo, hi, axis int }
	var stack [64]frame
	sp := 0
	stack[sp] = frame{0, len(t.ids) - 1, 0}
	sp++
	for sp > 0 {
		sp--
		f := stack[sp]
		if f.hi-f.lo <= kdLeaf {
			for i := f.lo; i <= f.hi; i++ {
				id := t.ids[i]
				if x, y := t.x[id], t.y[id]; x >= minX && x <= maxX && y >= minY && y <= maxY {
					out = append(out, id)
				}
			}
			continue
		}
		m := (f.lo + f.hi) / 2
		id := t.ids[m]
		x, y := t.x[id], t.y[id]
		if x >= minX && x <= maxX && y >= minY && y <= maxY {
			out = append(out, id)
		}
		c := x
		lo, hi := minX, maxX
		if f.axis == 1 {
			c, lo, hi = y, minY, maxY
		}
		if lo <= c && sp < len(stack) {
			stack[sp] = frame{f.lo, m - 1, 1 - f.axis}
			sp++
		}
		if hi >= c && sp < len(stack) {
			stack[sp] = frame{m + 1, f.hi, 1 - f.axis}
			sp++
		}
	}
	return out
}

// within appends the points within r of qx, qy to out.
func (t *kdTree) within(qx, qy, r float64, out []int32) []int32 {
	start := len(out)
	out = t.rangeQuery(qx-r, qy-r, qx+r, qy+r, out)
	keep := start
	r2 := r * r
	for _, id := range out[start:] {
		dx, dy := t.x[id]-qx, t.y[id]-qy
		if dx*dx+dy*dy <= r2 {
			out[keep] = id
			keep++
		}
	}
	return out[:keep]
}
