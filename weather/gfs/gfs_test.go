package gfs_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/internal/gribwrite"
	"github.com/worldiety/gift/weather"
	"github.com/worldiety/gift/weather/gfs"
)

func TestAnHourIsFetchedThroughTheFilterAndNormalised(t *testing.T) {
	run := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	gets := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "gfs.20261031/12/") {
			http.Error(w, "not yet", http.StatusForbidden)
		}
	})
	mux.HandleFunc("/filter", func(w http.ResponseWriter, r *http.Request) {
		gets++
		q := r.URL.Query()
		if q.Get("dir") != "/gfs.20261031/12/atmos" || q.Get("var_LCDC") != "on" || q.Get("leftlon") != "7.75" {
			http.Error(w, "bad query "+r.URL.RawQuery, http.StatusBadRequest)
			return
		}
		step := 0
		for _, c := range q.Get("file")[len(q.Get("file"))-3:] {
			step = step*10 + int(c-'0')
		}
		msg := func(cat, num, surface uint8, level int32, value float32) []byte {
			f := gribwrite.Field{Reference: run, Forecast: time.Duration(step) * time.Hour, Category: cat, Number: num,
				Surface: surface, Level: level, Ni: 9, Nj: 7, Lat0: 54, Lon0: 7.75, DLat: 0.25, DLon: 0.25, DecimalScale: 5}
			for range f.Ni * f.Nj {
				f.Values = append(f.Values, value)
			}
			return gribwrite.Encode(f)
		}
		var out []byte
		out = append(out, msg(6, 3, 214, 0, 50)...)
		out = append(out, msg(1, 7, 1, 0, 0.001)...)
		out = append(out, msg(2, 2, 103, 10, 4)...)
		out = append(out, msg(2, 3, 103, 10, -2)...)
		out = append(out, msg(0, 0, 103, 2, 280)...)
		w.Write(out)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	now := time.Date(2026, 10, 31, 19, 10, 0, 0, time.UTC)
	p := gfs.Global(gfs.Options{Filter: srv.URL + "/filter", Files: srv.URL + "/files", Now: func() time.Time { return now },
		CacheDir: t.TempDir()})
	q := weather.Query{Region: geo.RectOf(geo.LatLon(52.6, 7.9), geo.LatLon(53.7, 9.4)), From: now, To: now.Add(time.Hour),
		Variables: []weather.Variable{weather.CloudLow, weather.PrecipRate, weather.Wind10m, weather.Temp2m}}
	r, err := p.Fetch(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Issued.Equal(run) {
		t.Fatalf("run of %v; the 18 UTC one is not complete", r.Issued)
	}
	c := geo.LatLon(53.14, 8.21)
	at := now.Add(20 * time.Minute)
	if v, _ := r.At(weather.CloudLow, at).Sample(c); math.Abs(float64(v)-0.5) > 1e-3 {
		t.Errorf("low cloud %v, want 0.5", v)
	}
	if v, _ := r.At(weather.PrecipRate, at).Sample(c); math.Abs(float64(v)-3.6) > 0.01 {
		t.Errorf("precipitation %v mm/h, want 3.6", v)
	}
	if u, v, _ := r.At(weather.Wind10m, at).SampleVector(c); math.Abs(float64(u)-4) > 1e-3 || math.Abs(float64(v)+2) > 1e-3 {
		t.Errorf("wind %v, %v", u, v)
	}
	before := gets
	if _, err := p.Fetch(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if gets != before {
		t.Errorf("%d hours fetched again instead of from the cache", gets-before)
	}
}
