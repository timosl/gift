//go:build giftgpu

package maps_test

import (
	"context"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/maps"
	"github.com/worldiety/gift/weather"
)

// TestTheWeatherOnScreen draws the made up weather at noon in June over a
// dark ground and checks that the low clouds in the west whiten the map
// more than the thin high cloud of the east.
func TestTheWeatherOnScreen(t *testing.T) {
	noon := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	sky := weather.NewSky(fakeWeather{now: noon}, weather.SkyConfig{Now: func() time.Time { return noon },
		Region: geo.RectOf(geo.LatLon(52.9, 7.8), geo.LatLon(53.4, 8.6))})
	if err := sky.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	ch := make(chan func(), 4096)
	pipe := asset.NewPipeline(asset.Config{Deliver: func(fn func()) { ch <- fn }, Sizes: []int{256, 512}, Workers: 2})
	t.Cleanup(func() { pipe.Close() })
	ui.SetImagePipeline(pipe)
	t.Cleanup(func() { ui.SetImagePipeline(nil) })
	// Dark ground, on which white clouds show.
	m, err := maps.New(maps.Config{Tiles: solidTiles{color.RGBA{60, 80, 70, 255}}, Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}})
	if err != nil {
		t.Fatal(err)
	}
	h := gifttest.New(t, gifttest.Options{
		View: maps.View(m).Overlay(maps.Weather(sky, maps.WeatherOptions{
			Layers: maps.WeatherAll &^ maps.WeatherRadar,
		})), Size: geom.Sz(800, 600),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme(),
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for drained := true; drained; {
			select {
			case fn := <-ch:
				fn()
			default:
				drained = false
			}
		}
		h.AdvanceTicks(1, gifttest.FrameInterval)
		h.Image()
		time.Sleep(2 * time.Millisecond)
	}
	img := h.Image()
	if out := os.Getenv("GIFT_MAPS_WEATHER_OUT"); out != "" {
		f, _ := os.Create(out)
		png.Encode(f, img)
		f.Close()
	}
	lum := func(x0, x1 int) float64 {
		sum, n := 0.0, 0
		for y := 100; y < 500; y += 4 {
			for x := x0; x < x1; x += 4 {
				r, g, b, _ := img.At(x, y).RGBA()
				sum += float64(r+g+b) / 3 / 65535
				n++
			}
		}
		return sum / float64(n)
	}
	west, east := lum(20, 200), lum(600, 780)
	t.Logf("brightness west %.3f, east %.3f", west, east)
	if west <= east {
		t.Errorf("the clouded west (%.3f) is not lighter than the east (%.3f)", west, east)
	}
}
