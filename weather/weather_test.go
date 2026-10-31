package weather_test

import (
	"context"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/weather"
)

func field(v weather.Variable, t time.Time, value float32) *weather.Field {
	g := weather.Grid{Lat0: 54, Lon0: 8, DLat: 1, DLon: 1, W: 2, H: 2}
	return &weather.Field{Var: v, Valid: t, Grid: g, Data: []float32{value, value, value, value}}
}

func TestARunInterpolatesBetweenItsSteps(t *testing.T) {
	t0 := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	run := &weather.Run{Fields: map[weather.Variable][]*weather.Field{
		weather.CloudLow: {field(weather.CloudLow, t0, 0.2), field(weather.CloudLow, t0.Add(time.Hour), 0.8)},
	}}
	c := geo.LatLon(53.5, 8.5)
	for _, tc := range []struct {
		at   time.Time
		want float32
	}{
		{t0.Add(-time.Hour), 0.2}, {t0, 0.2}, {t0.Add(15 * time.Minute), 0.35}, {t0.Add(2 * time.Hour), 0.8},
	} {
		got, ok := run.At(weather.CloudLow, tc.at).Sample(c)
		if !ok || got < tc.want-1e-5 || got > tc.want+1e-5 {
			t.Errorf("at %v: %v, want %v", tc.at.Format("15:04"), got, tc.want)
		}
	}
	if run.At(weather.PrecipRate, t0) != nil {
		t.Error("a variable the run lacks has a field")
	}
	if _, ok := field(weather.CloudLow, t0, 1).Sample(geo.LatLon(60, 8)); ok {
		t.Error("a point outside the grid has a value")
	}
}

type oneRun struct{ n int }

func (o *oneRun) Info() weather.ProviderInfo {
	return weather.ProviderInfo{Name: "one", Cycle: time.Hour}
}
func (o *oneRun) Fetch(context.Context, weather.Query) (*weather.Run, error) {
	o.n++
	return &weather.Run{Provider: "one"}, nil
}

func TestASkyPublishesWhatItFetched(t *testing.T) {
	p := &oneRun{}
	s := weather.NewSky(p, weather.SkyConfig{Region: geo.RectOf(geo.LatLon(53, 8), geo.LatLon(54, 9))})
	if s.Run() != nil || s.Version() != 0 {
		t.Fatal("a run before any fetch")
	}
	if err := s.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Run() == nil || s.Version() != 1 || p.n != 1 {
		t.Fatalf("run %v, version %d, fetches %d", s.Run(), s.Version(), p.n)
	}
}
