package maps

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
)

// oldenburg is the extract of the project plan, section 37, when it has been
// cut on this machine; tests that need real data skip without it.
func oldenburg(t testing.TB) Provider {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(dir, "gift", "maps", "oldenburg.pmtiles")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no Oldenburg extract at %s; see PLAN.md, section 37", path)
	}
	p, err := PMTiles(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTileIDsFollowTheHilbertCurve(t *testing.T) {
	for _, c := range []struct {
		t  TileID
		id uint64
	}{
		{TileID{0, 0, 0}, 0},
		{TileID{1, 0, 0}, 1}, {TileID{1, 0, 1}, 2}, {TileID{1, 1, 1}, 3}, {TileID{1, 1, 0}, 4},
		{TileID{2, 0, 0}, 5},
	} {
		if got := tileIDOf(c.t); got != c.id {
			t.Errorf("tileIDOf(%v) = %d, want %d", c.t, got, c.id)
		}
	}
	// Every ID of a level once.
	seen := map[uint64]bool{}
	for x := range 8 {
		for y := range 8 {
			id := tileIDOf(TileID{3, x, y})
			if id < 21 || id >= 85 || seen[id] {
				t.Fatalf("tile 3/%d/%d has id %d", x, y, id)
			}
			seen[id] = true
		}
	}
}

// writePMTiles writes an uncompressed archive of tiles, the way the
// reference writer lays one out: header, root directory, metadata, data.
func writePMTiles(t *testing.T, tiles map[TileID][]byte, meta string) []byte {
	t.Helper()
	type ent struct {
		id   uint64
		data []byte
	}
	var es []ent
	for k, v := range tiles {
		es = append(es, ent{tileIDOf(k), v})
	}
	for i := range es {
		for j := i + 1; j < len(es); j++ {
			if es[j].id < es[i].id {
				es[i], es[j] = es[j], es[i]
			}
		}
	}
	var dir, data []byte
	put := func(v uint64) { dir = binary.AppendUvarint(dir, v) }
	put(uint64(len(es)))
	var last uint64
	for _, e := range es {
		put(e.id - last)
		last = e.id
	}
	for range es {
		put(1)
	}
	for _, e := range es {
		put(uint64(len(e.data)))
	}
	for _, e := range es {
		put(uint64(len(data)) + 1)
		data = append(data, e.data...)
	}
	h := make([]byte, pmHeaderLen)
	copy(h, "PMTiles")
	h[7] = 3
	le := binary.LittleEndian
	rootOff := uint64(pmHeaderLen)
	metaOff := rootOff + uint64(len(dir))
	dataOff := metaOff + uint64(len(meta))
	le.PutUint64(h[8:], rootOff)
	le.PutUint64(h[16:], uint64(len(dir)))
	le.PutUint64(h[24:], metaOff)
	le.PutUint64(h[32:], uint64(len(meta)))
	le.PutUint64(h[56:], dataOff)
	le.PutUint64(h[64:], uint64(len(data)))
	h[97], h[98], h[99] = pmCompNone, pmCompNone, pmTypeMVT
	h[100], h[101] = 0, 14
	e7 := func(v float64) uint32 { return uint32(int32(v * 1e7)) }
	le.PutUint32(h[102:], e7(-180))
	le.PutUint32(h[106:], e7(-85))
	le.PutUint32(h[110:], e7(180))
	le.PutUint32(h[114:], e7(85))
	out := append(h, dir...)
	out = append(out, meta...)
	return append(out, data...)
}

func TestAnArchiveIsReadBackTileByTile(t *testing.T) {
	tiles := map[TileID][]byte{
		{0, 0, 0}: []byte("world"), {3, 4, 2}: []byte("three"), {14, 8565, 5326}: []byte("oldenburg"),
	}
	raw := writePMTiles(t, tiles, `{"name":"Protomaps Basemap","version":"4.15.2","attribution":"<a href=\"x\">&copy; OpenStreetMap</a>"}`)
	path := filepath.Join(t.TempDir(), "a.pmtiles")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := PMTiles(path)
	if err != nil {
		t.Fatal(err)
	}
	info := p.Info()
	if info.Kind != Vector || info.Schema != "protomaps-v4" || info.Attribution != "© OpenStreetMap" || info.MaxZoom != 14 {
		t.Errorf("info %+v", info)
	}
	for k, want := range tiles {
		got, err := p.Tile(context.Background(), k)
		if err != nil || !bytes.Equal(got.Data, want) || got.Format != FormatMVT {
			t.Errorf("tile %v: %q, %v", k, got.Data, err)
		}
	}
	if _, err := p.Tile(context.Background(), TileID{3, 4, 3}); err != ErrNoTile {
		t.Errorf("a missing tile gave %v, want ErrNoTile", err)
	}
}

func TestTheOldenburgExtractIsReadAndDrawn(t *testing.T) {
	p := oldenburg(t)
	info := p.Info()
	if info.Kind != Vector || info.Schema != "protomaps-v4" || info.MaxZoom != 15 {
		t.Fatalf("info %+v", info)
	}
	want := TileID{15, 17131, 10652}
	tile, err := p.Tile(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if len(tile.Data) != 187503 {
		t.Errorf("the centre tile is %d bytes unpacked, the reference tool gave 187503", len(tile.Data))
	}
	s := ProtomapsLight()
	j := vectorJob{style: s.register(), want: want, src: want, px: 512, scale: 2}
	start := time.Now()
	img, err := rasterize(s, j, tile.Data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the densest tile of Oldenburg drawn at 512 px in %v", time.Since(start))
	if out := os.Getenv("GIFT_MAPS_TILE_OUT"); out != "" {
		f, _ := os.Create(out)
		png.Encode(f, img)
		f.Close()
	}
	// Mostly not the sea: the centre of Oldenburg is land.
	sea := rgba8(s.Background)
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] == sea.R && img.Pix[i+1] == sea.G && img.Pix[i+2] == sea.B {
			n++
		}
	}
	if n > len(img.Pix)/4/4 {
		t.Errorf("%d of %d pixels are sea in the centre of Oldenburg", n, len(img.Pix)/4)
	}
}

func BenchmarkRasterizeTheDensestTile(b *testing.B) {
	p := oldenburg(b)
	want := TileID{15, 17131, 10652}
	tile, err := p.Tile(context.Background(), want)
	if err != nil {
		b.Fatal(err)
	}
	s := ProtomapsLight()
	for _, px := range []int{256, 512} {
		b.Run(strconv.Itoa(px), func(b *testing.B) {
			j := vectorJob{style: s.register(), want: want, src: want, px: px, scale: float32(px) / TileSize}
			for b.Loop() {
				if _, err := rasterize(s, j, tile.Data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestATileOutsideTheBoundsIsDrawnFromAnAncestorInside: a regional archive
// holds at every level the tiles that touch its region, so a coarse level
// reaches further beyond its edge than a fine one. A tile outside is drawn
// from its nearest ancestor inside, a few levels up and no further.
func TestATileOutsideTheBoundsIsDrawnFromAnAncestorInside(t *testing.T) {
	m := &Map{info: ProviderInfo{Kind: Vector, MinZoom: 0, MaxZoom: 15,
		Bounds: geo.Rect{Min: geo.LatLon(53.05, 8.05), Max: geo.LatLon(53.23, 8.35)}}}
	at := func(lat, lon float64, z int) TileID {
		p := geo.Project(geo.LatLon(lat, lon))
		n := math.Exp2(float64(z))
		return TileID{Z: z, X: int(p.X * n), Y: int(p.Y * n)}
	}

	inside := at(53.14, 8.21, 13)
	if got, ok := m.dataTile(inside); !ok || got != inside {
		t.Errorf("a tile inside is drawn from %v, %v; want itself", got, ok)
	}
	if got, ok := m.dataTile(at(53.14, 8.21, 17)); !ok || got != at(53.14, 8.21, 15) {
		t.Errorf("a tile above the last level is drawn from %v, %v; want its ancestor at 15", got, ok)
	}

	// The Zwischenahner Meer, a few kilometres west of the region.
	lake := at(53.20, 8.01, 13)
	if m.inBounds(lake) {
		t.Fatal("the lake is inside the bounds at level 13; the test needs a tile outside")
	}
	got, ok := m.dataTile(lake)
	if !ok {
		t.Fatal("the lake has no tile to be drawn from")
	}
	if got.Z >= lake.Z || got.Z < lake.Z-boundsReach || lake.Ancestor(got.Z) != got || !m.inBounds(got) {
		t.Errorf("the lake at %v is drawn from %v, want an ancestor inside the bounds within %d levels",
			lake, got, boundsReach)
	}

	// Hamburg is far beyond reach.
	if got, ok := m.dataTile(at(53.55, 10.0, 13)); ok {
		t.Errorf("Hamburg is drawn from %v; it is further than %d levels from the region", got, boundsReach)
	}
}
