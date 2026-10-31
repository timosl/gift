package maps_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/maps"
)

// images is a texture table that accepts everything, for headless tests.
type images struct{ n int }

func (f *images) Resolve(h render.ImageHandle) (render.ImageID, bool) { return h.ID, h.ID > 0 }
func (f *images) Acquire(px render.Pixels) (render.ImageHandle, bool) {
	if px.IsEmpty() {
		return render.ImageHandle{}, false
	}
	f.n++
	return render.ImageHandle{ID: render.ImageID(f.n), Gen: 1}, true
}
func (f *images) Deallocate(render.ImageHandle) {}

// scene is a map in a harness with an image pipeline whose results are
// delivered when the test drains them.
type scene struct {
	*gifttest.Harness
	m  *maps.Map
	ch chan func()
}

func newScene(t *testing.T, cfg maps.Config, size geom.Size, view func(maps.MapView) maps.MapView) *scene {
	t.Helper()
	ch := make(chan func(), 4096)
	pipe := asset.NewPipeline(asset.Config{Deliver: func(fn func()) { ch <- fn }, Sizes: []int{256, 512}, Workers: 2})
	t.Cleanup(func() { pipe.Close() })
	ui.SetImagePipeline(pipe)
	t.Cleanup(func() { ui.SetImagePipeline(nil) })
	if cfg.Tiles == nil {
		cfg.Tiles = maps.DebugTiles()
	}
	m, err := maps.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	v := maps.View(m).Key("map")
	if view != nil {
		v = view(v)
	}
	h := gifttest.New(t, gifttest.Options{View: v, Size: size,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.App().SetImages(&images{})
	h.Frame()
	return &scene{Harness: h, m: m, ch: ch}
}

// load runs frames and delivers results until every visible tile is drawn.
func (s *scene) load(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for drained := true; drained; {
			select {
			case fn := <-s.ch:
				fn()
			default:
				drained = false
			}
		}
		s.Frame()
		st := s.m.Stats()
		if st.Tiles > 0 && st.Drawn == st.Tiles {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the tiles did not arrive: %+v", s.m.Stats())
}

// assertNothingOffscreen checks the rule of the project plan, section 37:
// every operation the map emitted touches the viewport.
func (s *scene) assertNothingOffscreen(t *testing.T) {
	t.Helper()
	view := geom.Rect{Max: geom.Pt(s.Size().W, s.Size().H)}
	l := s.List()
	for i, op := range s.Ops() {
		if op.Kind != render.OpImage {
			continue
		}
		r := l.Xform(op.Xform).TransformRect(op.Bounds)
		if r.Max.X <= view.Min.X || r.Min.X >= view.Max.X || r.Max.Y <= view.Min.Y || r.Min.Y >= view.Max.Y {
			t.Errorf("op %d draws a tile at %v, outside the map %v", i, r, view)
		}
	}
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestTheMapStartsWhereItWasToldAndDrawsItsTiles(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.1435, 8.2146), Zoom: 12}},
		geom.Sz(512, 384), nil)
	s.load(t)
	st := s.m.Stats()
	if st.Level != 12 {
		t.Errorf("level %d, want 12", st.Level)
	}
	// 512 by 384 at a whole zoom: three by two tiles, or four by three when
	// the centre does not fall on a tile corner.
	if st.Tiles < 6 || st.Tiles > 12 {
		t.Errorf("%d tiles visible, want between 6 and 12", st.Tiles)
	}
	c := s.m.Camera()
	if !near(c.Center.Lat, 53.1435, 1e-6) || !near(c.Center.Lon, 8.2146, 1e-6) || c.Zoom != 12 {
		t.Errorf("camera %+v", c)
	}
	s.assertNothingOffscreen(t)
	s.AssertExists(gifttest.ByKey("attribution"))
}

// TestATurnedMapDrawsOnlyTheTilesUnderIt: at a heading of 45 degrees the
// bounding box of the viewport holds far more tiles than the viewport
// touches, and only the touched ones are drawn.
func TestATurnedMapDrawsOnlyTheTilesUnderIt(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(20, 30), Zoom: 6, Heading: 45},
		Rotation: maps.RotationOn},
		geom.Sz(800, 200), nil)
	s.load(t)
	st := s.m.Stats()
	// The turned 800 by 200 strip spans a box of about 707 by 707 points,
	// eight tiles across that box would be 16 to 20; the strip itself
	// touches far fewer.
	if st.Tiles > 14 {
		t.Errorf("%d tiles for a turned strip, want only those it touches", st.Tiles)
	}
	s.assertNothingOffscreen(t)
	if h := s.m.Camera().Heading; !near(h, 45, 1e-9) {
		t.Errorf("heading %v, want 45", h)
	}
}

func TestAPinchKeepsThePlaceUnderTheFingers(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}},
		geom.Sz(600, 400), nil)
	s.load(t)
	// Off centre, so that the anchor matters.
	at := geom.Pt(420, 150)
	before := worldAt(s, at)
	s.Pinch(at, 60, 180, 300*time.Millisecond)
	s.AdvanceTicks(10, gifttest.FrameInterval)
	z := s.m.Camera().Zoom
	if !near(z, 10+math.Log2(3), 0.05) {
		t.Errorf("zoom %v after a pinch of three, want %v", z, 10+math.Log2(3))
	}
	after := worldAt(s, at)
	if d := geo.Distance(before, after); d > 15 {
		t.Errorf("the place under the fingers moved by %.1f m", d)
	}
}

// worldAt is the coordinate under a point of the map, by the camera.
func worldAt(s *scene, at geom.Point) geo.Coordinate {
	c := s.m.Camera()
	w, h := s.Size().W, s.Size().H
	scale := maps.TileSize * math.Exp2(c.Zoom)
	ctr := geo.Project(c.Center)
	hd := c.Heading * math.Pi / 180
	dx, dy := float64(at.X-w/2), float64(at.Y-h/2)
	sin, cos := math.Sincos(hd)
	return geo.Point{X: ctr.X + (cos*dx-sin*dy)/scale, Y: ctr.Y + (sin*dx+cos*dy)/scale}.Coordinate()
}

func TestAPinchPastTheLimitSpringsBack(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 15}, MaxZoom: 16},
		geom.Sz(600, 400), nil)
	s.Pinch(geom.Pt(300, 200), 40, 300, 300*time.Millisecond)
	s.AdvanceTicks(2, gifttest.FrameInterval)
	s.AdvanceTicks(120, gifttest.FrameInterval)
	if z := s.m.Camera().Zoom; !near(z, 16, 1e-3) {
		t.Fatalf("zoom %v after letting go past the limit, want 16", z)
	}
}

func TestAFlickGlidesAndStops(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(0, 0), Zoom: 5}},
		geom.Sz(600, 400), nil)
	start := s.m.Camera().Center
	s.Fling(geom.Pt(400, 200), geom.Pt(200, 200), 150*time.Millisecond)
	s.AdvanceTicks(1, gifttest.FrameInterval)
	early := s.m.Camera().Center
	s.AdvanceTicks(240, gifttest.FrameInterval)
	end := s.m.Camera().Center
	if !(early.Lon > start.Lon && end.Lon > early.Lon) {
		t.Fatalf("a flick to the left did not glide east: %v, %v, %v", start, early, end)
	}
	s.AdvanceTicks(10, gifttest.FrameInterval)
	if again := s.m.Camera().Center; again != end {
		t.Fatalf("the glide did not stop: %v then %v", end, again)
	}
}

func TestAFlightEndsExactlyWhereItWasSent(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 12}},
		geom.Sz(600, 400), nil)
	to := maps.Camera{Center: geo.LatLon(48.137, 11.575), Zoom: 13}
	s.m.SetCamera(to, true)
	s.AdvanceTicks(2, gifttest.FrameInterval)
	mid := s.m.Camera()
	if mid.Zoom >= 12 {
		t.Errorf("a long flight did not zoom out on the way: %v", mid.Zoom)
	}
	s.AdvanceTicks(240, gifttest.FrameInterval)
	got := s.m.Camera()
	if !near(got.Center.Lat, to.Center.Lat, 1e-9) || !near(got.Center.Lon, to.Center.Lon, 1e-9) || got.Zoom != 13 {
		t.Fatalf("landed at %+v, want %+v", got, to)
	}
}

// TestStandInsCoverWhatHasNotArrived: zoomed in by one level, the tiles of
// the new level are still being drawn and the level before stands in.
func TestStandInsCoverWhatHasNotArrived(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 9}},
		geom.Sz(512, 384), nil)
	s.load(t)
	s.m.SetCamera(maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}, false)
	s.Frame()
	st := s.m.Stats()
	if st.Level != 10 || st.Drawn == st.Tiles {
		t.Skipf("the new level arrived before the frame: %+v", st)
	}
	if st.Fallbacks == 0 {
		t.Fatalf("nothing stands in for the missing tiles: %+v", st)
	}
	images := 0
	for _, op := range s.Ops() {
		if op.Kind == render.OpImage {
			images++
		}
	}
	if images == 0 {
		t.Fatal("the map is blank while the level loads")
	}
	s.load(t)
}

func TestTheCameraChangeIsReportedWhenItComesToRest(t *testing.T) {
	var got []maps.Camera
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 9}},
		geom.Sz(512, 384), nil)
	s.m.OnCameraChange(func(c maps.Camera) { got = append(got, c) })
	s.m.SetCamera(maps.Camera{Center: geo.LatLon(53.2, 8.3), Zoom: 11}, true)
	s.AdvanceTicks(5, gifttest.FrameInterval)
	if len(got) != 0 {
		t.Fatalf("reported during the motion: %v", got)
	}
	s.AdvanceTicks(120, gifttest.FrameInterval)
	if len(got) != 1 || got[0].Zoom != 11 {
		t.Fatalf("reported %v, want one change at zoom 11", got)
	}
}

func TestTheKeysPanAndZoom(t *testing.T) {
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(0, 0), Zoom: 5}},
		geom.Sz(512, 384), nil)
	s.Find(gifttest.ByKey("map")).Focus()
	s.TypeRune('+')
	s.AdvanceTicks(60, gifttest.FrameInterval)
	if z := s.m.Camera().Zoom; z != 6 {
		t.Errorf("zoom %v after plus, want 6", z)
	}
	s.Key(gift.KeyRight)
	s.AdvanceTicks(120, gifttest.FrameInterval)
	if lon := s.m.Camera().Center.Lon; lon <= 0 {
		t.Errorf("longitude %v after the right arrow, want east of 0", lon)
	}
}

func TestAVectorProviderNeedsAStyleForItsSchema(t *testing.T) {
	p := fakeVector{}
	if _, err := maps.New(maps.Config{Tiles: p}); err == nil {
		t.Error("a vector provider without a style was accepted")
	}
	if _, err := maps.New(maps.Config{Tiles: p, Style: &maps.Style{Schema: "other"}}); err == nil {
		t.Error("a style for another schema was accepted")
	}
	if _, err := maps.New(maps.Config{Tiles: p, Style: &maps.Style{Schema: "test"}}); err != nil {
		t.Errorf("the matching style was refused: %v", err)
	}
}

type fakeVector struct{}

func (fakeVector) Info() maps.ProviderInfo {
	return maps.ProviderInfo{Name: "fake", Kind: maps.Vector, Schema: "test", MaxZoom: 14}
}

func (fakeVector) Tile(context.Context, maps.TileID) (maps.Tile, error) {
	return maps.Tile{}, maps.ErrNoTile
}

// extract opens the Oldenburg extract of the project plan, section 37, or
// skips.
func extract(t *testing.T) maps.Provider {
	t.Helper()
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	p, err := maps.PMTiles(filepath.Join(dir, "gift", "maps", "oldenburg.pmtiles"))
	if err != nil {
		t.Skipf("no Oldenburg extract: %v", err)
	}
	return p
}

// TestOldenburgIsDrawnWithItsLabels: the vector map of the extract, at a
// street level zoom and turned, has its tiles and its labels, and draws
// nothing outside itself.
func TestOldenburgIsDrawnWithItsLabels(t *testing.T) {
	s := newScene(t, maps.Config{
		Tiles: extract(t), Style: maps.ProtomapsLight(),
		Camera:   maps.Camera{Center: geo.LatLon(53.1405, 8.2135), Zoom: 15.3, Heading: 20},
		Rotation: maps.RotationOn,
	}, geom.Sz(800, 600), nil)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s.load(t)
		if s.m.Stats().Labels >= 10 {
			break
		}
		s.Advance(gifttest.FrameInterval)
	}
	st := s.m.Stats()
	if st.Labels < 10 {
		t.Fatalf("only %d labels in the centre of Oldenburg: %+v", st.Labels, st)
	}
	// Faded in.
	s.AdvanceTicks(20, gifttest.FrameInterval)
	glyphs := 0
	for _, op := range s.Ops() {
		if op.Kind == render.OpGlyphs && op.Color.A > 0.9 {
			glyphs++
		}
	}
	if glyphs < st.Labels {
		t.Errorf("%d opaque glyph runs for %d labels", glyphs, st.Labels)
	}
	s.assertNothingOffscreen(t)
}
