package maps

import (
	"bytes"
	"context"
	"io"
	"math"
	"slices"
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

// srcKey names a tile at a pixel size: one source and one entry of the image
// pipeline.
type srcKey struct {
	t     TileID
	px    int
	style uint8 // the light or the dark style of a vector map
}

// tileCache is the map's side of the tiles: one [asset.Source] per tile and
// size, created once and kept, the passes at which each was last seen
// ready, which is what the coarser and finer stand-ins are chosen from, and
// whether its texture was there when it was last drawn.
//
// Ready and drawn are two things. A tile is ready when its pixels are
// decoded; its texture is uploaded by the frame that first draws it, within
// the backend's upload budget, which a screenful of tiles after a zoom
// exceeds many times over. Until a tile has been drawn, its stand-in stays
// under it, or the map would be blank where the tile is still on its way to
// the GPU.
type tileCache struct {
	srcs     map[srcKey]*srcEntry
	loaded   map[srcKey]uint64
	resident map[srcKey]bool
}

// srcEntry is the source of a tile and the pass it was last used in.
type srcEntry struct {
	src  asset.Source
	used uint64
}

func (c *tileCache) init() {
	c.srcs = make(map[srcKey]*srcEntry)
	c.loaded = make(map[srcKey]uint64)
	c.resident = make(map[srcKey]bool)
}

// prune forgets what has not been used for a while, once there is much of
// it: the pipeline holds the pixels and its own budgets, and these maps are
// only the index into it.
func (c *tileCache) prune(pass uint64) {
	const keep = 4096
	if len(c.srcs) < keep {
		return
	}
	for k, e := range c.srcs {
		if pass-e.used > 600 {
			delete(c.srcs, k)
			delete(c.loaded, k)
			delete(c.resident, k)
		}
	}
}

// tileSource is one tile as an [asset.Source], so that the tiles go through
// the image pipeline like every picture: its priorities, its budgets, its
// disk cache and its banded uploads, with no path of their own.
type tileSource struct {
	m  *Map
	k  srcKey
	md asset.Metadata
}

// source returns the source of a tile: the provider's own for a raster
// [SourceProvider], else one that asks [Provider.Tile].
func (m *Map) source(k srcKey, pass uint64) asset.Source {
	e := m.tiles.srcs[k]
	if e == nil {
		e = &srcEntry{}
		if sp, ok := m.cfg.Tiles.(SourceProvider); ok && m.info.Kind == Raster {
			e.src = sp.TileSource(k.t)
		} else {
			style := "-"
			if m.info.Kind == Vector {
				style = m.styles[k.style].id()
			}
			id := "maps:" + m.info.Name + "@" + m.info.Revision + "/" + style + "/" +
				strconv.Itoa(k.px) + "/" + k.t.String()
			e.src = &tileSource{m: m, k: k, md: asset.Metadata{
				ID: asset.ID(id), Revision: m.info.Revision,
				Width: uint32(k.px), Height: uint32(k.px),
			}}
		}
		m.tiles.srcs[k] = e
	}
	e.used = pass
	return e.src
}

func (s *tileSource) Metadata() asset.Metadata { return s.md }

// Open fetches the tile from the provider. It runs on a goroutine of the
// pipeline; everything it reads of the map is fixed at construction.
func (s *tileSource) Open(ctx context.Context) (io.ReadCloser, error) {
	data, err := s.m.fetch(ctx, s.k)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *tileSource) String() string { return string(s.md.ID) }

// fetch returns the bytes the pipeline decodes for k: a raster tile as the
// provider encoded it, or a vector tile wrapped for the style's rasterizer.
func (m *Map) fetch(ctx context.Context, k srcKey) ([]byte, error) {
	if m.info.Kind == Vector {
		return m.fetchVector(ctx, k)
	}
	t, err := m.cfg.Tiles.Tile(ctx, k.t)
	if err != nil {
		return nil, err
	}
	return t.Data, nil
}

// tileRef is one tile in the frame: where it is, in the unrotated screen
// space, and its picture.
type tileRef struct {
	k    srcKey
	r    geom.Rect
	pic  ui.Picture
	kind uint8 // exact, coarser or finer
}

const (
	refExact uint8 = iota
	refCoarser
	refFiner
)

// level is the tile level shown at zoom: the nearest whole level, so that a
// tile is drawn between 0.71 and 1.41 times its size, within what the
// provider has. Above its last level a raster provider's tiles are enlarged,
// and a vector provider's are drawn again from the last level's geometry.
func (m *Map) level(zoom float64) int {
	z := int(math.Round(zoom))
	hi := m.info.MaxZoom
	if m.info.Kind == Vector {
		hi = 24
	}
	return max(m.info.MinZoom, min(hi, z))
}

// layoutTiles chooses the tiles of the frame and asks the pipeline for them.
//
// Visible means intersecting the viewport, exactly: the polygon of the turned
// viewport against each tile's square, not its bounding box. Of a tile that
// has not arrived, the nearest coarser one that has stands in, or the finer
// ones that have; they are drawn first, under the exact tiles, so the map is
// never blank where it has ever been seen, and a zoom across a level does not
// flicker.
func (n *node) layoutTiles(ctx *gift.LayoutContext, v projection, pass uint64) {
	m := n.m
	n.f.refs = n.f.refs[:0]
	z := m.level(v.zoom)
	cells := math.Exp2(float64(z))
	ts := v.scale / cells
	px := int(math.Round(TileSize * float64(ctx.Density())))
	style := m.styleIndex()
	poly := v.corners()

	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range poly {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	// Column and row of a screen position, the columns unwrapped.
	col := func(x float64) int { return int(math.Floor((v.c.X + (x-v.w/2)/v.scale) * cells)) }
	row := func(y float64) int { return int(math.Floor((v.c.Y + (y-v.h/2)/v.scale) * cells)) }
	c0, c1 := col(minX), col(maxX)
	r0, r1 := max(0, row(minY)), min(int(cells)-1, row(maxY))

	s := Stats{Level: z}
	n.f.coarse = n.f.coarse[:0]
	n.f.loading = false
	for r := r0; r <= r1; r++ {
		for c := c0; c <= c1; c++ {
			x0 := v.w/2 + (float64(c)/cells-v.c.X)*v.scale
			y0 := v.h/2 + (float64(r)/cells-v.c.Y)*v.scale
			if !polygonIntersectsRect(poly, x0, y0, x0+ts, y0+ts) {
				continue
			}
			wc := ((c % int(cells)) + int(cells)) % int(cells)
			t := TileID{Z: z, X: wc, Y: r}
			if _, ok := m.dataTile(t); !ok {
				continue
			}
			s.Tiles++
			k := srcKey{t: t, px: px, style: style}
			if m.info.Kind != Vector && !m.inBounds(t) {
				// A raster tile outside the provider's region cannot be
				// drawn again from its ancestor's data; the ancestor
				// stands in, enlarged.
				s.Fallbacks += n.standIn(ctx, k, c, x0, y0, ts, pass)
				continue
			}
			pic := ui.RequestPicture(ctx, m.source(k, pass), px)
			rect := geom.Rect{
				Min: geom.Pt(float32(x0), float32(y0)),
				Max: geom.Pt(float32(x0+ts), float32(y0+ts)),
			}
			if pic.Ready() {
				m.tiles.loaded[k] = pass
				n.f.refs = append(n.f.refs, tileRef{k: k, r: rect, pic: pic, kind: refExact})
				s.Drawn++
				if !m.tiles.resident[k] {
					// Decoded, and not yet drawn: its upload may take
					// frames, and its stand-in stays under it until a
					// layout after it.
					n.f.loading = true
					s.Fallbacks += n.standIn(ctx, k, c, x0, y0, ts, pass)
				}
				continue
			}
			if pic.Err() == nil {
				// A request in flight, or one the pipeline turned away
				// for now, which is asked again only by a later layout.
				n.f.loading = true
			}
			// Still coming, or not there at all: whatever covers the place
			// stands in.
			s.Fallbacks += n.standIn(ctx, k, c, x0, y0, ts, pass)
		}
	}
	m.stats.Level, m.stats.Tiles, m.stats.Drawn, m.stats.Fallbacks = s.Level, s.Tiles, s.Drawn, s.Fallbacks
	// The ring of tiles around the view, fetched before it is panned to,
	// when the provider permits it: at prefetch priority, which never
	// delays a visible tile, and without a layout when one arrives.
	if m.info.Prefetch {
		n.f.prefetched = 0
		ring := func(c, r int) {
			if r < 0 || r > int(cells)-1 {
				return
			}
			wc := ((c % int(cells)) + int(cells)) % int(cells)
			t := TileID{Z: z, X: wc, Y: r}
			if _, ok := m.dataTile(t); !ok || (m.info.Kind != Vector && !m.inBounds(t)) {
				return
			}
			k := srcKey{t: t, px: px, style: style}
			ui.PrefetchPicture(ctx, m.source(k, pass), px)
			n.f.prefetched++
		}
		for r := r0 - 1; r <= r1+1; r++ {
			ring(c0-1, r)
			ring(c1+1, r)
		}
		for c := c0; c <= c1; c++ {
			ring(c, r0-1)
			ring(c, r1+1)
		}
	}
	m.tiles.prune(pass)
}

// tilesUnder appends to out the tiles of level z that the polygon poly, in
// the unrotated screen space of v, intersects, within the provider's
// coverage and with their columns wrapped, each once.
func (m *Map) tilesUnder(v projection, z int, poly [4][2]float64, out []TileID) []TileID {
	cells := math.Exp2(float64(z))
	ts := v.scale / cells
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range poly {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	col := func(x float64) int { return int(math.Floor((v.c.X + (x-v.w/2)/v.scale) * cells)) }
	row := func(y float64) int { return int(math.Floor((v.c.Y + (y-v.h/2)/v.scale) * cells)) }
	for r := max(0, row(minY)); r <= min(int(cells)-1, row(maxY)); r++ {
		for c := col(minX); c <= col(maxX); c++ {
			x0 := v.w/2 + (float64(c)/cells-v.c.X)*v.scale
			y0 := v.h/2 + (float64(r)/cells-v.c.Y)*v.scale
			if !polygonIntersectsRect(poly, x0, y0, x0+ts, y0+ts) {
				continue
			}
			wc := ((c % int(cells)) + int(cells)) % int(cells)
			// Outside the provider's region, the ancestor the place is
			// drawn from, as its picture is.
			t, ok := m.dataTile(TileID{Z: z, X: wc, Y: r})
			if !ok {
				continue
			}
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// boundsReach is how many levels above a tile outside the provider's bounds
// its data may come from: the nearest ancestor inside them.
//
// A regional archive holds at every level the whole tiles that touch its
// region, so a coarse level shows further beyond the region's edge than a
// fine one. Without this the map dropped what the coarser level had just
// shown, a lake a few kilometres outside, at the zoom that crossed into the
// next level, as if it had left the window. Four levels is a tile enlarged
// sixteen times, drawn again from its ancestor's geometry for a vector map;
// beyond that the map ends.
const boundsReach = 4

// inBounds reports whether the tile t touches the provider's bounds.
func (m *Map) inBounds(t TileID) bool {
	cells := math.Exp2(float64(t.Z))
	nw, se := m.info.Bounds.Projected()
	return t.X >= int(math.Floor(nw.X*cells)) && t.X <= int(math.Ceil(se.X*cells))-1 &&
		t.Y >= int(math.Floor(nw.Y*cells)) && t.Y <= int(math.Ceil(se.Y*cells))-1
}

// dataTile returns the tile of the provider whose data draws the tile t: t
// itself, its ancestor at the provider's last level when t is above it, and
// for a t outside the provider's bounds its nearest ancestor inside them,
// at most boundsReach levels up. It reports false where there is none.
func (m *Map) dataTile(t TileID) (TileID, bool) {
	src := t
	if src.Z > m.info.MaxZoom {
		src = src.Ancestor(m.info.MaxZoom)
	}
	for d := 0; ; d++ {
		if m.inBounds(src) {
			return src, true
		}
		if d >= boundsReach || src.Z <= m.info.MinZoom {
			return TileID{}, false
		}
		src = src.Ancestor(src.Z - 1)
	}
}

// standIn finds what can be drawn in place of the tile k, whose unwrapped
// column is c and whose square starts at x0, y0 with edge ts: the nearest
// coarser tile whose texture is there, or else the finer ones whose
// textures are, or else the nearest coarser tile that was ever ready, which
// is uploaded first. It returns how many it added.
//
// A texture that is there is preferred to a nearer one that is not: a zoom
// out comes back to a level that was evicted from the GPU long ago, and the
// tiles of the level it left are still there and show the place now.
func (n *node) standIn(ctx *gift.LayoutContext, k srcKey, c int, x0, y0, ts float64, pass uint64) int {
	m := n.m
	var alt tileRef
	for d := 1; d <= 8 && k.t.Z-d >= m.info.MinZoom; d++ {
		a := srcKey{t: k.t.Ancestor(k.t.Z - d), px: k.px, style: k.style}
		if _, ok := m.tiles.loaded[a]; !ok {
			continue
		}
		// One coarse tile covers many missing ones; it is drawn once.
		for _, have := range n.f.coarse {
			if have == a {
				return 0
			}
		}
		pic := ui.RequestPicture(ctx, m.source(a, pass), a.px)
		if !pic.Ready() {
			continue
		}
		f := float64(int(1) << d)
		// The coarse tile's square, from the unwrapped column of k.
		ac := math.Floor(float64(c) / f)
		ax := x0 - (float64(c)-ac*f)*ts
		ay := y0 - float64(k.t.Y-a.t.Y*int(f))*ts
		ref := tileRef{k: a, pic: pic, kind: refCoarser, r: geom.Rect{
			Min: geom.Pt(float32(ax), float32(ay)),
			Max: geom.Pt(float32(ax+ts*f), float32(ay+ts*f)),
		}}
		if !m.tiles.resident[a] {
			if !alt.pic.Ready() {
				alt = ref
			}
			continue
		}
		n.f.coarse = append(n.f.coarse, a)
		n.f.refs = append(n.f.refs, ref)
		return 1
	}
	added := 0
	for i := range 4 {
		ch := srcKey{t: TileID{Z: k.t.Z + 1, X: 2*k.t.X + i%2, Y: 2*k.t.Y + i/2}, px: k.px, style: k.style}
		if !m.tiles.resident[ch] {
			continue
		}
		pic := ui.RequestPicture(ctx, m.source(ch, pass), ch.px)
		if !pic.Ready() {
			continue
		}
		h := ts / 2
		cx, cy := x0+float64(i%2)*h, y0+float64(i/2)*h
		n.f.refs = append(n.f.refs, tileRef{k: ch, pic: pic, kind: refFiner, r: geom.Rect{
			Min: geom.Pt(float32(cx), float32(cy)),
			Max: geom.Pt(float32(cx+h), float32(cy+h)),
		}})
		added++
	}
	if added < 4 && alt.pic.Ready() {
		n.f.coarse = append(n.f.coarse, alt.k)
		n.f.refs = append(n.f.refs, alt)
		added++
	}
	return added
}

// paintTiles draws the tiles of the frame, stand-ins first: coarser, then
// finer, then the exact ones over both. What it draws, and what it could
// not because the texture is still on its way, is what the next layout
// chooses the stand-ins by.
func (n *node) paintTiles(ctx *gift.PaintContext, origin geom.Point) {
	// Without a GPU nothing is ever drawn, and nothing is waited for.
	headless := ctx.Images() == nil
	for _, kind := range [...]uint8{refCoarser, refFiner, refExact} {
		for i := range n.f.refs {
			ref := &n.f.refs[i]
			if ref.kind != kind {
				continue
			}
			ok := ref.pic.DrawTile(ctx, ref.r.Translate(origin), ui.OpaqueWhite) || headless
			if ok != n.m.tiles.resident[ref.k] {
				if ok {
					n.m.tiles.resident[ref.k] = true
				} else {
					delete(n.m.tiles.resident, ref.k)
				}
			}
		}
	}
}
