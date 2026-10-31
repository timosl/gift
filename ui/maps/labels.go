package maps

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

// The labels of a vector map are not drawn into its tiles but set on the
// map in every frame: upright at any heading, a place name not cut in half
// at a tile's edge, and the more important label winning where two would
// overlap. This is what MapKit and Mapbox do, and what makes rotation
// possible at all; see the project plan, section 37.
//
// A tile's labels are read on a goroutine of the map's own from the same
// provider as its picture, and handed to the user interface by the image
// pipeline's executor ([asset.Pipeline.Post]). They are read separately and
// not while the picture is drawn because a picture can come from a cache,
// where no drawing happens.

// labelCand is one label a tile offers.
type labelCand struct {
	text string
	// at is the anchor in world coordinates.
	at geo.Point
	// angle is the direction of the line a label runs along, in the
	// unrotated screen space, and seg the length of that line's segment in
	// world units; both zero for a point.
	angle, seg float64
	rule       int
	rank       float64
	minZoom    float64
}

type labelKey struct {
	t     TileID
	style uint8
}

// labelStore is the labels of the tiles read so far.
type labelStore struct {
	tiles   map[labelKey][]labelCand
	used    map[labelKey]uint64
	pending map[labelKey]bool
	queue   chan labelKey
	running bool
}

// placed is one label of the frame.
type placed struct {
	text  string
	c     geom.Point // centre, map coordinates
	size  geom.Size
	angle float64 // on the screen
	rule  int
	alpha float32
}

// labelLevel is the tile level the labels of zoom are read from: the tiles'
// level, but at most the provider's last one, whose tiles hold everything.
func (m *Map) labelLevel(zoom float64) int { return min(m.level(zoom), m.info.MaxZoom) }

// labelsFor returns the labels of a tile, asking for them when they have
// not been read.
func (m *Map) labelsFor(k labelKey, pass uint64) ([]labelCand, bool) {
	s := &m.labels
	if s.tiles == nil {
		s.tiles = make(map[labelKey][]labelCand)
		s.used = make(map[labelKey]uint64)
		s.pending = make(map[labelKey]bool)
		s.queue = make(chan labelKey, 256)
	}
	if c, ok := s.tiles[k]; ok {
		s.used[k] = pass
		return c, true
	}
	if s.pending[k] {
		return nil, false
	}
	pipe := ui.ImagePipeline()
	if pipe == nil {
		return nil, false
	}
	select {
	case s.queue <- k:
		s.pending[k] = true
	default:
		// Full: asked again by a later layout.
		return nil, false
	}
	if !s.running {
		s.running = true
		go m.readLabels(s.queue, m.styles, m.cfg.Tiles, func(fn func()) { pipe.Post(fn) })
	}
	return nil, false
}

// hasLabels reports whether the labels of a tile have been read, without
// asking for them.
func (m *Map) hasLabels(k labelKey, pass uint64) bool {
	if _, ok := m.labels.tiles[k]; ok {
		m.labels.used[k] = pass
		return true
	}
	return false
}

// readLabels is the label goroutine of a map. It touches nothing of the map
// but the provider and the styles, which are fixed, and hands every result
// back through post.
func (m *Map) readLabels(queue <-chan labelKey, styles [2]*Style, p Provider, post func(func())) {
	for k := range queue {
		st := styles[k.style]
		var cands []labelCand
		if t, err := p.Tile(context.Background(), k.t); err == nil && t.Format == FormatMVT {
			cands = extractLabels(st, k.t, t.Data)
		}
		post(func() {
			s := &m.labels
			delete(s.pending, k)
			s.tiles[k] = cands
			s.used[k] = 0
			if len(s.tiles) > 512 {
				for old, pass := range s.used {
					if len(s.tiles) <= 384 {
						break
					}
					if pass+300 < m.frameNo {
						delete(s.tiles, old)
						delete(s.used, old)
					}
				}
			}
			m.request()
		})
	}
}

// extractLabels reads the labels of a tile: every named feature a rule of
// the style chooses, anchored at its point, in the middle of its longest
// segment, or in the middle of its outline.
func extractLabels(st *Style, t TileID, data []byte) []labelCand {
	want := func(name string) bool {
		for i := range st.Labels {
			if st.Labels[i].Source == name {
				return true
			}
		}
		return false
	}
	tile, err := decodeMVT(data, want)
	if err != nil {
		return nil
	}
	var out []labelCand
	cells := math.Exp2(float64(t.Z))
	for ri := range st.Labels {
		r := &st.Labels[ri]
		ml := tile.layer(r.Source)
		if ml == nil {
			continue
		}
		ext := float64(ml.extent)
		world := func(x, y float64) geo.Point {
			return geo.Point{X: (float64(t.X) + x/ext) / cells, Y: (float64(t.Y) + y/ext) / cells}
		}
		for fi := range ml.features {
			f := &ml.features[fi]
			name := ml.str(f, "name")
			if name == "" || !matches(r.Kinds, nil, ml.str(f, "kind"), "") {
				continue
			}
			c := labelCand{text: name, rule: ri, minZoom: ml.num(f, "min_zoom", 0)}
			c.rank = float64(r.Rank)*1000 - ml.num(f, "population_rank", 0)*10 + c.minZoom
			switch f.typ {
			case geomPoint:
				var px, py float64
				f.path(func(x, y float64) { px, py = x, y }, func(float64, float64) {}, func() {})
				if px < 0 || py < 0 || px >= ext || py >= ext {
					// In the buffer: the neighbour owns it.
					continue
				}
				c.at = world(px, py)
			case geomLine:
				x0, y0, x1, y1, ok := longestSegment(f, ext)
				if !ok {
					continue
				}
				c.at = world((x0+x1)/2, (y0+y1)/2)
				c.angle = math.Atan2(y1-y0, x1-x0)
				c.seg = math.Hypot(x1-x0, y1-y0) / ext / cells
			case geomPolygon:
				minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
				f.path(func(x, y float64) {
					minX, maxX, minY, maxY = math.Min(minX, x), math.Max(maxX, x), math.Min(minY, y), math.Max(maxY, y)
				}, func(x, y float64) {
					minX, maxX, minY, maxY = math.Min(minX, x), math.Max(maxX, x), math.Min(minY, y), math.Max(maxY, y)
				}, func() {})
				cx, cy := (minX+maxX)/2, (minY+maxY)/2
				if cx < 0 || cy < 0 || cx >= ext || cy >= ext {
					continue
				}
				c.at = world(cx, cy)
			default:
				continue
			}
			out = append(out, c)
		}
	}
	return out
}

// longestSegment is the longest segment of a line inside the tile.
func longestSegment(f *mvtFeature, ext float64) (x0, y0, x1, y1 float64, ok bool) {
	var px, py, best float64
	in := func(x, y float64) bool { return x >= 0 && y >= 0 && x <= ext && y <= ext }
	f.path(func(x, y float64) { px, py = x, y }, func(x, y float64) {
		if in(px, py) && in(x, y) {
			if d := math.Hypot(x-px, y-py); d > best {
				best, x0, y0, x1, y1, ok = d, px, py, x, y, true
			}
		}
		px, py = x, y
	}, func() {})
	return
}

// labelPadding is the free space kept around a label, and labelRepeat the
// distance under which the same street name is not set twice. labelReach is
// how far outside the map an anchor may be and its label still reach in.
const (
	labelPadding      = 3
	labelRepeat       = 220
	labelReach        = 160
	maxLabels         = 160
	maxShapedPerFrame = 12
	labelFade         = 180 * time.Millisecond
	labelGrace        = 250 * time.Millisecond
)

// measureSize is the font size a label is measured at. Its size at any other
// is that one scaled: the size of a label follows the zoom, and a zoom
// passes through a new size in every frame, which measured anew would leave
// most labels unmeasured, and so off the map, until the zoom ends.
const measureSize = 32

// measured returns the size of a label at the font size fs, if it was
// measured before.
func (m *Map) measured(s string, fs float32) (geom.Size, bool) {
	v, ok := m.sizes[s]
	return scaled(v, fs), ok
}

// measure shapes a label and remembers its size, and returns it at the font
// size fs. The table is emptied when it grows past a few thousand entries
// rather than kept in order: a map moved across a whole city meets that many
// names, and measuring them again is the cost of a few frames.
func (m *Map) measure(s string, fs float32) geom.Size {
	if m.sizes == nil || len(m.sizes) > 4096 {
		m.sizes = make(map[string]geom.Size, 512)
	}
	v := ui.MeasureText(s, ui.TextStyle{Size: measureSize})
	m.sizes[s] = v
	return scaled(v, fs)
}

// labelFontSize is the font size of a label of the rule r at zoom, in steps
// of half a point. The glyphs of a label are rasterised for the size they
// are drawn at, and a zoom that went through a new size in every frame
// would rasterise every label again in every frame, and fill the glyph atlas
// with sizes it never needs again until it turns glyphs away. In steps, a
// zoom across a level passes two or three.
func labelFontSize(r *LabelRule, zoom float64) float32 {
	return float32(math.Round(r.Size.At(zoom)*2) / 2)
}

// scaled is a size measured at measureSize at the font size fs.
func scaled(v geom.Size, fs float32) geom.Size {
	k := fs / measureSize
	return geom.Size{W: v.W * k, H: v.H * k}
}

// layoutLabels places the labels of the frame.
func (n *node) layoutLabels(ctx *gift.LayoutContext, v projection, pass uint64) {
	m := n.m
	f := n.f
	f.labels = f.labels[:0]
	f.resetBoxes(float32(v.w), float32(v.h))
	m.frameNo = pass
	// What the map draws over its labels keeps them away: the attribution
	// and the compass.
	for _, b := range f.chrome {
		f.addBox(b)
	}
	if m.info.Kind != Vector {
		return
	}
	si := m.styleIndex()
	st := m.styles[si]
	if st == nil || len(st.Labels) == 0 {
		return
	}
	// The tiles of the label level under the viewport, whether their
	// pictures have arrived or not. Of a tile whose labels have not been
	// read, those of the level the map comes from stand in, its parent's or
	// its children's, as its picture's stand-in does: a zoom across a level
	// would otherwise take the labels off the map until the new ones are
	// read.
	lz := m.labelLevel(v.zoom)
	f.ltiles = f.ltiles[:0]
	add := func(t TileID) {
		if !slices.Contains(f.ltiles, t) {
			f.ltiles = append(f.ltiles, t)
		}
	}
	for _, t := range f.vtiles {
		if t.Z > lz {
			t = t.Ancestor(lz)
		}
		if _, ok := m.labelsFor(labelKey{t: t, style: si}, pass); ok {
			add(t)
			continue
		}
		if t.Z > m.info.MinZoom {
			if p := t.Ancestor(t.Z - 1); m.hasLabels(labelKey{t: p, style: si}, pass) {
				add(p)
				continue
			}
		}
		for i := range 4 {
			if ch := (TileID{Z: t.Z + 1, X: 2*t.X + i%2, Y: 2*t.Y + i/2}); m.hasLabels(labelKey{t: ch, style: si}, pass) {
				add(ch)
			}
		}
	}
	// The candidates change with the tiles, their labels arriving and the
	// zoom crossing a quarter level, not with every frame of a pan: they are
	// collected and sorted again only then.
	key := candKey{style: si, level: lz, quarter: int(math.Floor(v.zoom * 4))}
	if key != f.candKey || !slices.Equal(f.ltiles, f.candTiles) {
		f.candKey = key
		f.candTiles = append(f.candTiles[:0], f.ltiles...)
		f.cands = f.cands[:0]
		for _, t := range f.ltiles {
			cs, _ := m.labelsFor(labelKey{t: t, style: si}, pass)
			for _, c := range cs {
				r := &st.Labels[c.rule]
				if !r.applies(v.zoom) || c.minZoom > v.zoom+0.5 {
					continue
				}
				f.cands = append(f.cands, c)
			}
		}
		slices.SortStableFunc(f.cands, func(a, b labelCand) int { return cmp.Compare(a.rank, b.rank) })
	}

	w, h := float32(v.w), float32(v.h)
	now := ctx.Now()
	shaped := 0
	f.more = false
	for _, c := range f.cands {
		if len(f.labels) >= maxLabels {
			break
		}
		// The anchor first: a label whose anchor is far outside is not
		// measured at all, which is most of the candidates of a dense tile.
		x, y := v.toScreen(c.at)
		if x < -labelReach || y < -labelReach || x > v.w+labelReach || y > v.h+labelReach {
			continue
		}
		r := &st.Labels[c.rule]
		fs := labelFontSize(r, v.zoom)
		size, ok := m.measured(c.text, fs)
		if !ok {
			// A string seen for the first time is shaped, which costs a
			// fraction of a millisecond on a Raspberry Pi; a pan into a
			// street full of new names would pay for all of them in one
			// frame. A frame shapes a few and leaves the rest to the next.
			if shaped >= maxShapedPerFrame {
				f.more = true
				continue
			}
			shaped++
			size = m.measure(c.text, fs)
		}
		p := placed{text: c.text, c: geom.Pt(float32(x), float32(y)), size: size, rule: c.rule}
		box := geom.Rect{Min: geom.Pt(p.c.X-size.W/2, p.c.Y-size.H/2), Max: geom.Pt(p.c.X+size.W/2, p.c.Y+size.H/2)}
		if r.Along {
			if c.seg*v.scale < float64(size.W)+8 {
				continue
			}
			a := normalizeAngle(c.angle - v.heading)
			if a > math.Pi/2 {
				a -= math.Pi
			} else if a < -math.Pi/2 {
				a += math.Pi
			}
			p.angle = a
			// The bounding box of the turned label.
			s, co := math.Sincos(a)
			bw := float32(math.Abs(co)*float64(size.W) + math.Abs(s)*float64(size.H))
			bh := float32(math.Abs(s)*float64(size.W) + math.Abs(co)*float64(size.H))
			box = geom.Rect{Min: geom.Pt(p.c.X-bw/2, p.c.Y-bh/2), Max: geom.Pt(p.c.X+bw/2, p.c.Y+bh/2)}
		}
		// Nothing outside the map is placed, and nothing is drawn where it
		// would be cut by the edge.
		if box.Min.X < 0 || box.Min.Y < 0 || box.Max.X > w || box.Max.Y > h {
			continue
		}
		if n.collides(box, c.text, p.c) {
			continue
		}
		f.labels = append(f.labels, p)
		f.addBox(box)
	}

	// Fading in: a label is new when it was not placed for a while. One
	// that misses a few frames, because the order of the candidates changed
	// or its tile's labels were read again, comes back as it was rather than
	// fading in once more, which would make it blink.
	if m.fade == nil {
		m.fade = make(map[string]labelFadeState)
	}
	f.fading = false
	for i := range f.labels {
		p := &f.labels[i]
		st, ok := m.fade[p.text]
		if !ok || now-st.seen > labelGrace {
			st.since = now
		}
		st.seen = now
		m.fade[p.text] = st
		p.alpha = float32(min(1, float64(now-st.since)/float64(labelFade)))
		if p.alpha < 1 {
			f.fading = true
		}
	}
	for k, st := range m.fade {
		if now-st.seen > labelGrace {
			delete(m.fade, k)
		}
	}
	if f.more {
		f.fading = true
	}
	m.stats.Labels = len(f.labels)
}

// collides reports whether a label in box overlaps one already placed, or
// repeats a name placed nearby: a street or a river crosses many tiles, and
// each of them offers its name.
func (n *node) collides(box geom.Rect, text string, c geom.Point) bool {
	f := n.f
	pad := box.Outset(geom.Insets{Left: labelPadding, Top: labelPadding, Right: labelPadding, Bottom: labelPadding})
	c0, r0, c1, r1 := f.cellsOf(pad)
	for r := r0; r <= r1; r++ {
		for col := c0; col <= c1; col++ {
			for _, i := range f.grid[r*f.cols+col] {
				if pad.Overlaps(f.boxes[i]) {
					return true
				}
			}
		}
	}
	for i := range f.labels {
		if l := &f.labels[i]; l.text == text {
			d := l.c.Sub(c)
			if d.X*d.X+d.Y*d.Y < labelRepeat*labelRepeat {
				return true
			}
		}
	}
	return false
}

// labelCell is the edge of a cell of the collision grid: a label is tested
// against the boxes of the cells it touches, not against all of them.
const labelCell = 64

// candKey is what the candidates of a frame depend on, besides the tiles.
type candKey struct {
	style   uint8
	level   int
	quarter int
}

// labelFadeState is when a label was first placed, for its fade in, and
// when it was placed last.
type labelFadeState struct{ since, seen time.Duration }

// resetBoxes empties the placed boxes and sizes the grid to a w by h map.
func (f *frame) resetBoxes(w, h float32) {
	f.boxes = f.boxes[:0]
	f.cols, f.rows = int(w/labelCell)+1, int(h/labelCell)+1
	n := f.cols * f.rows
	for len(f.grid) < n {
		f.grid = append(f.grid, nil)
	}
	for i := range f.grid[:n] {
		f.grid[i] = f.grid[i][:0]
	}
}

func (f *frame) cellsOf(r geom.Rect) (c0, r0, c1, r1 int) {
	clampi := func(v float32, hi int) int { return max(0, min(hi, int(v/labelCell))) }
	return clampi(r.Min.X, f.cols-1), clampi(r.Min.Y, f.rows-1), clampi(r.Max.X, f.cols-1), clampi(r.Max.Y, f.rows-1)
}

// addBox records a placed box.
func (f *frame) addBox(b geom.Rect) {
	i := int32(len(f.boxes))
	f.boxes = append(f.boxes, b)
	c0, r0, c1, r1 := f.cellsOf(b)
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			f.grid[r*f.cols+c] = append(f.grid[r*f.cols+c], i)
		}
	}
}

// paintLabels draws the labels of the frame.
func (n *node) paintLabels(ctx *gift.PaintContext, origin geom.Point) {
	st := n.m.styles[n.m.styleIndex()]
	if st == nil {
		return
	}
	for i := range n.f.labels {
		p := &n.f.labels[i]
		r := &st.Labels[p.rule]
		ts := ui.TextStyle{
			Size:  labelFontSize(r, n.f.proj.zoom),
			Color: fade(ui.ResolveColor(r.Color), p.alpha),
			Halo:  fade(ui.ResolveColor(r.Halo), p.alpha),
		}
		c := p.c.Add(origin)
		at := geom.Pt(c.X-p.size.W/2, c.Y-p.size.H/2)
		if p.angle == 0 {
			ui.DrawText(ctx, p.text, at, ts)
			continue
		}
		ctx.PushTransform(geom.Translate(geom.Pt(-c.X, -c.Y)).Mul(geom.Rotate(float32(p.angle))).Mul(geom.Translate(c)))
		ui.DrawText(ctx, p.text, at, ts)
		ctx.PopTransform()
	}
}

// fade scales a premultiplied colour by a.
func fade(c ui.Color, a float32) ui.Color {
	return ui.Color{R: c.R * a, G: c.G * a, B: c.B * a, A: c.A * a}
}
