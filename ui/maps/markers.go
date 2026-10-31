package maps

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/motion"
)

// MarkerKind is how the points of a layer are drawn.
type MarkerKind uint8

const (
	// PhotoMarker draws a point as the thumbnail of its photograph, a
	// cluster as the thumbnail of its representative with the count on a
	// badge: the map of the iOS Photos app.
	PhotoMarker MarkerKind = iota
	// PinMarker draws a point as a coloured disc with its title below, and
	// a cluster as a larger disc with its count.
	PinMarker
)

// MarkerStyle is how the points of a layer look.
type MarkerStyle struct {
	Kind MarkerKind
	// Size is the edge of a photo marker or the diameter of a pin, in
	// logical pixels.
	Size float32
	// Color is the colour of a pin and of a cluster's badge.
	Color ui.Color
}

// PhotoMarkers is the style of a map of photographs.
func PhotoMarkers() MarkerStyle {
	return MarkerStyle{Kind: PhotoMarker, Size: 56, Color: ui.RGB(255, 59, 48)}
}

// PinMarkers is the style of places, in c.
func PinMarkers(c ui.Color) MarkerStyle { return MarkerStyle{Kind: PinMarker, Size: 22, Color: c} }

// pointLayer is one set of points on a map.
type pointLayer struct {
	p     *Points
	style MarkerStyle
}

// Points adds a set of points to the map, drawn in style; see [Points] and
// [MarkerStyle]. Points are drawn above the overlays and the labels, the
// layers in the order they were added.
func (v MapView) Points(p *Points, style MarkerStyle) MapView {
	if style.Size <= 0 {
		style.Size = 56
		if style.Kind == PinMarker {
			style.Size = 22
		}
	}
	v.points = append(v.points[:len(v.points):len(v.points)], pointLayer{p: p, style: style})
	return v
}

// OnSelect sets the function called when a single point is tapped.
func (v MapView) OnSelect(fn func(Point)) MapView { v.onSelect = fn; return v }

// OnCluster sets the function called when a cluster is tapped. Without
// one, a tap on a cluster zooms in until it comes apart, as MapKit does;
// with one, the application decides, the Photos app showing the
// photographs of the cluster in a grid, say.
func (v MapView) OnCluster(fn func(Cluster)) MapView { v.onCluster = fn; return v }

// The transition of a level change: a critically damped ease of 300 ms,
// the time the Photos app takes.
const (
	clusterMove       = 300 * time.Millisecond
	clusterHysteresis = 0.1
	maxMarkers        = 1000
)

// layerState is what a map keeps per set of points across frames: the
// level shown, with hysteresis, and the transition in progress.
type layerState struct {
	idx          *clusterIndex
	level        int
	from         int
	t0           time.Duration
	transitioned bool
}

// marker is one marker of the frame.
type marker struct {
	layer int
	level int
	entry int32
	c     geom.Point // centre, map coordinates
	alpha float32
	scale float32
	pic   ui.Picture
	count int32
	title string
	z     int32 // the drawing layer; see layerMarkers
}

// layoutPoints chooses the markers of the frame: the entries of each layer's
// level whose marker touches the map, and, while the level changes, where
// each of them is on its way from or to the cluster that holds it.
func (n *node) layoutPoints(ctx *gift.LayoutContext, v projection) {
	f := n.f
	m := n.m
	f.markers = f.markers[:0]
	f.pointsBusy = false
	now := ctx.Now()
	if m.layers == nil {
		m.layers = make(map[*Points]*layerState)
	}
	for li, pl := range n.v.points {
		idx := pl.p.idx.Load()
		if idx == nil {
			f.pointsBusy = true
			continue
		}
		st := m.layers[pl.p]
		if st == nil {
			st = &layerState{}
			m.layers[pl.p] = st
		}
		if st.idx != idx {
			*st = layerState{idx: idx, level: idx.level(v.zoom), from: -1}
		}
		want := idx.level(v.zoom)
		if (want > st.level && v.zoom >= float64(st.level+1)+clusterHysteresis) ||
			(want < st.level && v.zoom < float64(st.level)-clusterHysteresis) {
			st.from, st.level, st.t0 = st.level, want, now
		}
		s := 1.0
		if st.from >= 0 {
			s = easeOut(float64(now-st.t0) / float64(clusterMove))
			if s >= 1 {
				st.from = -1
			}
		}
		size := float64(pl.style.Size)
		px := int(math.Ceil(size * float64(ctx.Density())))
		switch {
		case st.from < 0:
			n.queryMarkers(ctx, li, pl, idx, st.level, v, size, px, func(e int32, at geo.Point) (geo.Point, float32, float32) {
				return at, 1, 1
			})
		case st.level > st.from:
			// Coming apart: every entry starts at the cluster that held it.
			from := &idx.levels[st.from]
			n.queryMarkers(ctx, li, pl, idx, st.level, v, size, px, func(e int32, at geo.Point) (geo.Point, float32, float32) {
				a := idx.ancestor(st.level, e, st.from)
				start := geo.Point{X: from.x[a], Y: from.y[a]}
				return lerpWorld(start, at, s), float32(s), float32(0.6 + 0.4*s)
			})
		default:
			// Merging: the entries of the old level fly into the cluster
			// that now holds them and fade, which appears as they arrive.
			to := &idx.levels[st.level]
			n.queryMarkers(ctx, li, pl, idx, st.from, v, size, px, func(e int32, at geo.Point) (geo.Point, float32, float32) {
				a := idx.ancestor(st.from, e, st.level)
				end := geo.Point{X: to.x[a], Y: to.y[a]}
				return lerpWorld(at, end, s), float32(1 - s), 1
			})
			n.queryMarkers(ctx, li, pl, idx, st.level, v, size, px, func(e int32, at geo.Point) (geo.Point, float32, float32) {
				return at, float32(s), float32(0.8 + 0.2*s)
			})
		}
		if st.from >= 0 {
			f.pointsBusy = true
		}
	}
	// The southern markers in front, as on a globe seen from above.
	slices.SortStableFunc(f.markers, func(a, b marker) int {
		switch {
		case a.c.Y < b.c.Y:
			return -1
		case a.c.Y > b.c.Y:
			return 1
		}
		return 0
	})
	m.stats.Points = len(f.markers)
	// The markers keep the labels of the map away, and the ones that
	// overlap are drawn in layers; see paintMarkers.
	first := len(f.chrome)
	for i := range f.markers {
		k := &f.markers[i]
		half := n.v.points[k.layer].style.Size/2 + 6
		f.chrome = append(f.chrome, geom.Rect{Min: k.c.Sub(geom.Pt(half, half)), Max: k.c.Add(geom.Pt(half, half))})
	}
	n.layerMarkers(f.chrome[first:])
}

// queryMarkers appends the markers of the entries of one level whose
// marker touches the map. place says where each is drawn this frame.
func (n *node) queryMarkers(ctx *gift.LayoutContext, li int, pl pointLayer, idx *clusterIndex, level int,
	v projection, size float64, px int, place func(int32, geo.Point) (geo.Point, float32, float32)) {
	f := n.f
	l := &idx.levels[level]
	if l.tree == nil || len(l.x) == 0 {
		return
	}
	// The world box of the turned viewport, grown by half a marker.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{0, 0}, {v.w, 0}, {v.w, v.h}, {0, v.h}} {
		w := v.toWorld(c[0], c[1])
		minX, maxX = math.Min(minX, w.X), math.Max(maxX, w.X)
		minY, maxY = math.Min(minY, w.Y), math.Max(maxY, w.Y)
	}
	pad := (size/2 + 4) / v.scale
	minX, minY, maxX, maxY = minX-pad, minY-pad, maxX+pad, maxY+pad
	f.hits = f.hits[:0]
	// The index holds X in [0, 1); a viewport across the antimeridian is
	// two boxes.
	for off := math.Floor(minX); off <= maxX; off++ {
		f.hits = l.tree.rangeQuery(math.Max(0, minX-off), minY, math.Min(1, maxX-off), maxY, f.hits)
	}
	half := float32(size / 2)
	for _, e := range f.hits {
		if len(f.markers) >= maxMarkers {
			return
		}
		at, alpha, scale := place(e, geo.Point{X: l.x[e], Y: l.y[e]})
		x, y := v.toScreen(at)
		c := geom.Pt(float32(x), float32(y))
		// Exactly: a marker that does not touch the map is not drawn.
		if c.X+half < 0 || c.Y+half < 0 || c.X-half > float32(v.w) || c.Y-half > float32(v.h) {
			continue
		}
		k := marker{layer: li, level: level, entry: e, c: c, alpha: alpha, scale: scale, count: l.count[e]}
		p := &idx.pts[l.rep[e]]
		if pl.style.Kind == PhotoMarker && p.Photo != nil {
			k.pic = ui.RequestPicture(ctx, p.Photo, px)
			if !k.pic.Ready() && k.pic.Err() == nil {
				f.loading = true
			}
		}
		if k.count == 1 {
			k.title = p.Title
		}
		f.markers = append(f.markers, k)
	}
}

func lerpWorld(a, b geo.Point, s float64) geo.Point {
	dx := b.X - a.X
	dx -= math.Round(dx)
	return geo.Point{X: a.X + dx*s, Y: motion.Lerp(a.Y, b.Y, s)}
}

func easeOut(t float64) float64 {
	t = math.Max(0, math.Min(1, t))
	return 1 - (1-t)*(1-t)*(1-t)
}

// The parts of a photo marker, drawn in this order.
type markerPass uint8

const (
	passShadow markerPass = 1 << iota
	passFrame
	passPhoto
	passBadge
	passText
)

// paintMarkers draws the markers of the frame.
//
// Not marker by marker but part by part — every shadow, then every frame,
// every picture, every badge and every count — within each of a few layers
// in which no two markers overlap. A marker is five materials, and a backend
// batches only consecutive operations of one material: thirty markers drawn
// one after the other are some hundred and fifty draw calls. A marker is in
// a layer above every marker behind it that it overlaps, so the order where
// the order shows is kept; see layerMarkers.
func (n *node) paintMarkers(ctx *gift.PaintContext, origin geom.Point) {
	f := n.f
	for layer := int32(0); layer < f.markerLayers; layer++ {
		for _, pass := range [...]markerPass{passShadow, passFrame, passPhoto, passBadge, passText} {
			for i := range f.markers {
				if f.markers[i].z == layer {
					n.paintMarker(ctx, origin, &f.markers[i], pass)
				}
			}
		}
	}
}

// layerMarkers puts every marker, back to front, into the lowest layer above
// all the markers before it that it overlaps.
func (n *node) layerMarkers(rects []geom.Rect) {
	f := n.f
	f.markerLayers = 0
	for i := range f.markers {
		z := int32(0)
		for j := range i {
			if f.markers[j].z >= z && rects[j].Overlaps(rects[i]) {
				z = f.markers[j].z + 1
			}
		}
		f.markers[i].z = z
		f.markerLayers = max(f.markerLayers, z+1)
	}
}

func (n *node) paintMarker(ctx *gift.PaintContext, origin geom.Point, k *marker, pass markerPass) {
	st := n.v.points[k.layer].style
	c := k.c.Add(origin)
	switch st.Kind {
	case PhotoMarker:
		paintPhotoMarker(ctx, c, st, k, pass)
	default:
		paintPin(ctx, c, st, k, pass)
	}
}

func paintPhotoMarker(ctx *gift.PaintContext, c geom.Point, st MarkerStyle, k *marker, pass markerPass) {
	s := st.Size * k.scale
	r := geom.Rect{Min: c.Sub(geom.Pt(s/2, s/2)), Max: c.Add(geom.Pt(s/2, s/2))}
	a := k.alpha
	if pass&passShadow != 0 {
		ctx.Add(render.Op{Kind: render.OpShadow, Bounds: r, CornerRadius: 10, Color: fade(ui.RGBA(0, 0, 0, 90), a), Blur: 8})
	}
	if pass&passFrame != 0 {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: r, CornerRadius: 10, Color: fade(ui.RGB(255, 255, 255), a)})
	}
	inner := r.Inset(geom.Insets{Left: 3, Top: 3, Right: 3, Bottom: 3})
	if pass&passPhoto != 0 && !k.pic.DrawFit(ctx, inner, ui.FitCover, fade(ui.OpaqueWhite, a), 7) {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: inner, CornerRadius: 7, Color: fade(ui.RGB(222, 224, 228), a)})
	}
	if k.count > 1 {
		paintBadge(ctx, geom.Pt(r.Max.X-4, r.Min.Y+4), k.count, st.Color, a, pass)
	}
}

// paintBadge draws a count on a pill centred at c.
func paintBadge(ctx *gift.PaintContext, c geom.Point, count int32, col ui.Color, a float32, pass markerPass) {
	s := strconv.Itoa(int(count))
	ts := ui.TextStyle{Size: 12, Color: fade(ui.RGB(255, 255, 255), a)}
	sz := ui.MeasureText(s, ts)
	if pass&passBadge != 0 {
		h := float32(20)
		w := max(h, sz.W+12)
		b := geom.Rect{Min: geom.Pt(c.X-w/2, c.Y-h/2), Max: geom.Pt(c.X+w/2, c.Y+h/2)}
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, CornerRadius: h / 2, Color: fade(ui.ResolveColor(col), a)})
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b, CornerRadius: h / 2, StrokeWidth: 1.5, Color: fade(ui.RGB(255, 255, 255), a)})
	}
	if pass&passText != 0 {
		ui.DrawText(ctx, s, geom.Pt(c.X-sz.W/2, c.Y-sz.H/2), ts)
	}
}

func paintPin(ctx *gift.PaintContext, c geom.Point, st MarkerStyle, k *marker, pass markerPass) {
	a := k.alpha
	d := st.Size * k.scale
	if k.count > 1 {
		d *= 1.4
	}
	r := geom.Rect{Min: c.Sub(geom.Pt(d/2, d/2)), Max: c.Add(geom.Pt(d/2, d/2))}
	if pass&passShadow != 0 {
		ctx.Add(render.Op{Kind: render.OpShadow, Bounds: r, CornerRadius: d / 2, Color: fade(ui.RGBA(0, 0, 0, 80), a), Blur: 5})
	}
	if pass&passFrame != 0 {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: r, CornerRadius: d / 2, Color: fade(ui.ResolveColor(st.Color), a)})
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: r, CornerRadius: d / 2, StrokeWidth: 2, Color: fade(ui.RGB(255, 255, 255), a)})
	}
	if k.count > 1 {
		if pass&passText != 0 {
			s := strconv.Itoa(int(k.count))
			ts := ui.TextStyle{Size: 12, Color: fade(ui.RGB(255, 255, 255), a)}
			sz := ui.MeasureText(s, ts)
			ui.DrawText(ctx, s, geom.Pt(c.X-sz.W/2, c.Y-sz.H/2), ts)
		}
		return
	}
	if pass&passFrame != 0 {
		dot := d / 5
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, CornerRadius: dot / 2, Color: fade(ui.RGB(255, 255, 255), a),
			Bounds: geom.Rect{Min: c.Sub(geom.Pt(dot/2, dot/2)), Max: c.Add(geom.Pt(dot/2, dot/2))}})
	}
	if k.title != "" && pass&passText != 0 {
		ts := ui.TextStyle{Size: 12, Color: fade(ui.RGB(30, 32, 38), a), Halo: fade(ui.RGBA(255, 255, 255, 230), a)}
		sz := ui.MeasureText(k.title, ts)
		ui.DrawText(ctx, k.title, geom.Pt(c.X-sz.W/2, r.Max.Y+2), ts)
	}
}

// hitMarker returns the marker under a map position, the one in front.
func (n *node) hitMarker(at geom.Point) (*marker, bool) {
	for i := len(n.f.markers) - 1; i >= 0; i-- {
		k := &n.f.markers[i]
		half := n.v.points[k.layer].style.Size / 2
		if at.X >= k.c.X-half && at.X <= k.c.X+half && at.Y >= k.c.Y-half && at.Y <= k.c.Y+half {
			return k, true
		}
	}
	return nil, false
}

// tapMarker handles a tap on a marker: a point is selected, a cluster is
// handed to the application or zoomed into until it comes apart.
func (n *node) tapMarker(k *marker) {
	st := n.m.layers[n.v.points[k.layer].p]
	if st == nil || st.idx == nil {
		return
	}
	c := st.idx.cluster(k.level, k.entry)
	if c.Count == 1 {
		if n.v.onSelect != nil {
			n.v.onSelect(c.Representative)
		}
		return
	}
	if n.v.onCluster != nil {
		n.v.onCluster(c)
		return
	}
	n.zoomToCluster(c)
}

// zoomToCluster flies to the rectangle of a cluster's points, but at least
// to the level at which it comes apart.
func (n *node) zoomToCluster(c Cluster) {
	m := n.m
	cam := m.fit(c.Bounds())
	cam.Zoom = math.Max(cam.Zoom, float64(c.level)+1+clusterHysteresis)
	cam.Zoom = math.Min(cam.Zoom, m.maxZ)
	m.mv.flyTo(n.now, geo.Project(cam.Center), cam.Zoom, radians(cam.Heading), true)
	n.moved()
}
