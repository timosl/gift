package maps

import (
	"bytes"
	"context"
	"io"
	"math"
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

// srcKey names a tile at a pixel size: one source and one entry of the image
// pipeline.
type srcKey struct {
	t  TileID
	px int
}

// tileCache is the map's side of the tiles: one [asset.Source] per tile and
// size, created once and kept, and the passes at which each was last seen
// ready, which is what the coarser and finer stand-ins are chosen from.
type tileCache struct {
	srcs   map[srcKey]*tileSource
	loaded map[srcKey]uint64
}

func (c *tileCache) init() {
	c.srcs = make(map[srcKey]*tileSource)
	c.loaded = make(map[srcKey]uint64)
}

// prune forgets what has not been used for a while, once there is much of
// it: the pipeline holds the pixels and its own budgets, and these maps are
// only the index into it.
func (c *tileCache) prune(pass uint64) {
	const keep = 4096
	if len(c.srcs) < keep {
		return
	}
	for k, s := range c.srcs {
		if pass-s.used > 600 {
			delete(c.srcs, k)
			delete(c.loaded, k)
		}
	}
}

// tileSource is one tile as an [asset.Source], so that the tiles go through
// the image pipeline like every picture: its priorities, its budgets, its
// disk cache and its banded uploads, with no path of their own.
type tileSource struct {
	m    *Map
	k    srcKey
	md   asset.Metadata
	used uint64
}

func (m *Map) source(k srcKey, pass uint64) *tileSource {
	s := m.tiles.srcs[k]
	if s == nil {
		style := "-"
		if m.info.Kind == Vector {
			style = m.style.id()
		}
		id := "maps:" + m.info.Name + "@" + m.info.Revision + "/" + style + "/" +
			strconv.Itoa(k.px) + "/" + k.t.String()
		s = &tileSource{m: m, k: k, md: asset.Metadata{
			ID: asset.ID(id), Revision: m.info.Revision,
			Width: uint32(k.px), Height: uint32(k.px),
		}}
		m.tiles.srcs[k] = s
	}
	s.used = pass
	return s
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
	// The provider's coverage, as columns and rows of this level.
	nw, se := m.info.Bounds.Projected()
	bc0, bc1 := int(math.Floor(nw.X*cells)), int(math.Ceil(se.X*cells))-1
	br0, br1 := int(math.Floor(nw.Y*cells)), int(math.Ceil(se.Y*cells))-1

	s := Stats{Level: z}
	n.f.coarse = n.f.coarse[:0]
	for r := r0; r <= r1; r++ {
		if r < br0 || r > br1 {
			continue
		}
		for c := c0; c <= c1; c++ {
			x0 := v.w/2 + (float64(c)/cells-v.c.X)*v.scale
			y0 := v.h/2 + (float64(r)/cells-v.c.Y)*v.scale
			if !polygonIntersectsRect(poly, x0, y0, x0+ts, y0+ts) {
				continue
			}
			wc := ((c % int(cells)) + int(cells)) % int(cells)
			if wc < bc0 || wc > bc1 {
				continue
			}
			s.Tiles++
			k := srcKey{t: TileID{Z: z, X: wc, Y: r}, px: px}
			pic := ui.RequestPicture(ctx, m.source(k, pass), px)
			rect := geom.Rect{
				Min: geom.Pt(float32(x0), float32(y0)),
				Max: geom.Pt(float32(x0+ts), float32(y0+ts)),
			}
			if pic.Ready() {
				m.tiles.loaded[k] = pass
				n.f.refs = append(n.f.refs, tileRef{k: k, r: rect, pic: pic, kind: refExact})
				s.Drawn++
				continue
			}
			// Still coming, or not there at all: whatever covers the place
			// stands in.
			s.Fallbacks += n.standIn(ctx, k, c, x0, y0, ts, pass)
		}
	}
	m.stats.Level, m.stats.Tiles, m.stats.Drawn, m.stats.Fallbacks = s.Level, s.Tiles, s.Drawn, s.Fallbacks
	m.tiles.prune(pass)
}

// standIn finds what can be drawn in place of the tile k, whose unwrapped
// column is c and whose square starts at x0, y0 with edge ts: the nearest
// coarser tile that was ever ready, or else the finer ones that were. It
// returns how many it added.
func (n *node) standIn(ctx *gift.LayoutContext, k srcKey, c int, x0, y0, ts float64, pass uint64) int {
	m := n.m
	for d := 1; d <= 8 && k.t.Z-d >= m.info.MinZoom; d++ {
		a := srcKey{t: k.t.Ancestor(k.t.Z - d), px: k.px}
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
		n.f.coarse = append(n.f.coarse, a)
		f := float64(int(1) << d)
		// The coarse tile's square, from the unwrapped column of k.
		ac := math.Floor(float64(c) / f)
		ax := x0 - (float64(c)-ac*f)*ts
		ay := y0 - float64(k.t.Y-a.t.Y*int(f))*ts
		n.f.refs = append(n.f.refs, tileRef{k: a, pic: pic, kind: refCoarser, r: geom.Rect{
			Min: geom.Pt(float32(ax), float32(ay)),
			Max: geom.Pt(float32(ax+ts*f), float32(ay+ts*f)),
		}})
		return 1
	}
	added := 0
	for i := range 4 {
		ch := srcKey{t: TileID{Z: k.t.Z + 1, X: 2*k.t.X + i%2, Y: 2*k.t.Y + i/2}, px: k.px}
		if _, ok := m.tiles.loaded[ch]; !ok {
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
	return added
}

// paintTiles draws the tiles of the frame, stand-ins first: coarser, then
// finer, then the exact ones over both.
func (n *node) paintTiles(ctx *gift.PaintContext, origin geom.Point) {
	for _, kind := range [...]uint8{refCoarser, refFiner, refExact} {
		for i := range n.f.refs {
			ref := &n.f.refs[i]
			if ref.kind != kind {
				continue
			}
			ref.pic.DrawTile(ctx, ref.r.Translate(origin), ui.OpaqueWhite)
		}
	}
}
