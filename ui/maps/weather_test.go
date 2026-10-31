package maps_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/maps"
	"github.com/worldiety/gift/weather"
)

// fakeWeather is a provider of made up weather over Oldenburg: a band of
// low cloud and rain in the west, high cloud in the east, wind from the
// south west everywhere, warm.
type fakeWeather struct{ now time.Time }

func (f fakeWeather) Info() weather.ProviderInfo {
	return weather.ProviderInfo{Name: "fake", Coverage: geo.World, Step: time.Hour, Cycle: time.Hour, Attribution: "Fake weather"}
}

func (f fakeWeather) Fetch(_ context.Context, q weather.Query) (*weather.Run, error) {
	g := weather.Grid{Lat0: 54, Lon0: 7, DLat: 0.05, DLon: 0.05, W: 61, H: 41}
	run := &weather.Run{Provider: "fake", Issued: f.now.Add(-time.Hour), Fields: map[weather.Variable][]*weather.Field{}}
	field := func(v weather.Variable, fn func(lat, lon float64) (float32, float32)) {
		d := make([]float32, g.W*g.H)
		var vv []float32
		if v.IsVector() {
			vv = make([]float32, g.W*g.H)
		}
		for y := range g.H {
			for x := range g.W {
				a, b := fn(g.Lat0-float64(y)*g.DLat, g.Lon0+float64(x)*g.DLon)
				d[y*g.W+x] = a
				if vv != nil {
					vv[y*g.W+x] = b
				}
			}
		}
		run.Fields[v] = []*weather.Field{{Var: v, Valid: f.now, Grid: g, Data: d, V: vv}}
	}
	field(weather.CloudLow, func(lat, lon float64) (float32, float32) {
		return float32(math.Max(0, math.Min(1, (8.2-lon)*3))), 0
	})
	field(weather.CloudHigh, func(lat, lon float64) (float32, float32) {
		return float32(math.Max(0, math.Min(1, (lon-8.2)*2))), 0
	})
	field(weather.PrecipRate, func(lat, lon float64) (float32, float32) {
		return float32(math.Max(0, (8.15-lon)*20)), 0
	})
	field(weather.Wind10m, func(lat, lon float64) (float32, float32) { return 6, 4 })
	field(weather.WindAloft, func(lat, lon float64) (float32, float32) { return 15, 8 })
	field(weather.Temp2m, func(lat, lon float64) (float32, float32) { return 285, 0 })
	return run, nil
}

// TestTheWeatherIsDrawnOverTheMap: clouds, their shadows, the radar
// picture, rain and wind, all of it inside the map.
func TestTheWeatherIsDrawnOverTheMap(t *testing.T) {
	noon := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	sky := weather.NewSky(fakeWeather{now: noon}, weather.SkyConfig{Now: func() time.Time { return noon },
		Region: geo.RectOf(geo.LatLon(52.9, 7.8), geo.LatLon(53.4, 8.6))})
	if err := sky.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	wx := maps.Weather(sky, maps.WeatherOptions{})
	s := newScene(t, maps.Config{Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}},
		geom.Sz(800, 600), func(v maps.MapView) maps.MapView { return v.Overlay(wx) })
	s.load(t)
	deadline := time.Now().Add(10 * time.Second)
	images, triangles := 0, 0
	for time.Now().Before(deadline) {
		s.AdvanceTicks(2, gifttest.FrameInterval)
		images, triangles = 0, 0
		for _, op := range s.Ops() {
			switch op.Kind {
			case render.OpImage:
				images++
			case render.OpTriangles:
				triangles++
			}
		}
		if triangles == 2 && images > s.m.Stats().Tiles+3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if triangles != 2 {
		t.Errorf("%d triangle operations, want the wind and the rain", triangles)
	}
	if images <= s.m.Stats().Tiles+3 {
		t.Errorf("%d pictures for %d tiles: the clouds, their shadows and the radar are missing", images, s.m.Stats().Tiles)
	}
	s.assertNothingOffscreen(t)
}
