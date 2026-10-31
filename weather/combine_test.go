package weather_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/weather"
)

// flat is a provider with one value everywhere in its coverage.
type flat struct {
	cov   geo.Rect
	value float32
	step  float64
}

func (f flat) Info() weather.ProviderInfo {
	return weather.ProviderInfo{Name: "flat", Coverage: f.cov, Variables: []weather.Variable{weather.CloudLow}}
}

func (f flat) Fetch(_ context.Context, q weather.Query) (*weather.Run, error) {
	r := q.Region.Intersect(f.cov)
	g := weather.Grid{Lat0: r.Max.Lat, Lon0: r.Min.Lon, DLat: f.step, DLon: f.step,
		W: int((r.Max.Lon-r.Min.Lon)/f.step) + 1, H: int((r.Max.Lat-r.Min.Lat)/f.step) + 1}
	d := make([]float32, g.W*g.H)
	for i := range d {
		d[i] = f.value
	}
	t := time.Date(2026, 10, 31, 18, 0, 0, 0, time.UTC)
	return &weather.Run{Fields: map[weather.Variable][]*weather.Field{weather.CloudLow: {{Var: weather.CloudLow, Valid: t, Grid: g, Data: d}}}}, nil
}

func TestTheFallbackTakesOverPastThePrimarysEdgeWithoutAJump(t *testing.T) {
	primary := flat{cov: geo.RectOf(geo.LatLon(47, 6), geo.LatLon(55, 15)), value: 1, step: 0.02}
	fallback := flat{cov: geo.World, value: 0, step: 0.25}
	c := weather.Combine(primary, fallback)
	run, err := c.Fetch(context.Background(), weather.Query{Region: geo.RectOf(geo.LatLon(52, 13), geo.LatLon(54, 18))})
	if err != nil {
		t.Fatal(err)
	}
	f := run.Fields[weather.CloudLow][0]
	at := func(lon float64) float32 { v, _ := f.Sample(geo.LatLon(53, lon)); return v }
	if v := at(13.2); math.Abs(float64(v)-1) > 1e-3 {
		t.Errorf("well inside the primary %v, want 1", v)
	}
	if v := at(17); v != 0 {
		t.Errorf("well outside it %v, want 0", v)
	}
	// Across the edge at 15 the value falls over the band, never by much
	// from one sample to the next.
	prev := at(14)
	for lon := 14.0; lon <= 16; lon += 0.02 {
		v := at(lon)
		if math.Abs(float64(v-prev)) > 0.06 {
			t.Fatalf("a jump of %v at %v", v-prev, lon)
		}
		prev = v
	}
}
