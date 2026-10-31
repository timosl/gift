package dwd_test

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/internal/gribwrite"
	"github.com/worldiety/gift/weather"
	"github.com/worldiety/gift/weather/dwd"
)

// server is an open data server with one run, 15 UTC, whose steps up to
// 8 are published, of a small grid around Oldenburg; the files are not
// compressed, which the provider takes as well.
type server struct {
	mu   sync.Mutex
	gets int
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	name := parts[len(parts)-1]
	if !strings.Contains(name, "_2026103115_") {
		http.NotFound(w, r)
		return
	}
	var step int
	var v string
	if strings.Contains(name, "pressure-level") {
		// ..._2026103115_FFF_500_u.grib2.bz2
		f := strings.Split(strings.TrimSuffix(name, ".grib2.bz2"), "_")
		step = atoi(f[len(f)-3])
		v = f[len(f)-1]
	} else {
		f := strings.Split(strings.TrimSuffix(name, ".grib2.bz2"), "_")
		step = atoi(f[len(f)-3])
		v = strings.Join(f[len(f)-1:], "")
		if strings.HasSuffix(name, "tot_prec.grib2.bz2") {
			step = atoi(f[len(f)-4])
			v = "tot_prec"
		} else if strings.HasSuffix(name, "_10m.grib2.bz2") || strings.HasSuffix(name, "_2m.grib2.bz2") {
			step = atoi(f[len(f)-4])
			v = f[len(f)-2] + "_" + f[len(f)-1]
		}
	}
	if step > 8 {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	s.mu.Lock()
	s.gets++
	s.mu.Unlock()
	value := map[string]float32{"clcl": 80, "clcm": 20, "clch": 0, "u_10m": 3, "v_10m": -4, "u": 12, "v": 5, "t_2m": 283}[v]
	if v == "tot_prec" {
		value = float32(step) * 1.5 // 1.5 mm an hour since the run began
	}
	f := gribwrite.Field{
		Reference: time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC), Forecast: time.Duration(step) * time.Hour,
		Category: 6, Number: 22, Surface: 1, Ni: 41, Nj: 31, Lat0: 53.8, Lon0: 7.4, DLat: 0.04, DLon: 0.04,
		SouthFirst: true, DecimalScale: 2,
	}
	for range f.Ni * f.Nj {
		f.Values = append(f.Values, value)
	}
	w.Write(gribwrite.Encode(f))
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func TestTheLatestCompleteRunIsFetchedAndNormalised(t *testing.T) {
	srv := &server{}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	now := time.Date(2026, 10, 31, 18, 40, 0, 0, time.UTC)
	cache := t.TempDir()
	p := dwd.ICOND2(dwd.Options{Base: ts.URL, CacheDir: cache, Now: func() time.Time { return now }})
	q := weather.Query{
		Region: geo.RectOf(geo.LatLon(52.9, 7.9), geo.LatLon(53.4, 8.5)),
		From:   now, To: now.Add(2 * time.Hour),
	}
	run, err := p.Fetch(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	// The 18 UTC run is not on the server; the 15 UTC one is, to step 8.
	if run.Issued != time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC) {
		t.Fatalf("run of %v", run.Issued)
	}
	c := geo.LatLon(53.14, 8.21)
	at := now.Add(30 * time.Minute)
	check := func(v weather.Variable, want float64) {
		t.Helper()
		f := run.At(v, at)
		got, ok := f.Sample(c)
		if !ok || math.Abs(float64(got)-want) > 0.02 {
			t.Errorf("%v = %v (%v), want %v", v, got, ok, want)
		}
	}
	check(weather.CloudLow, 0.8)
	check(weather.CloudMid, 0.2)
	check(weather.PrecipRate, 1.5)
	check(weather.Temp2m, 283)
	f := run.At(weather.WindAloft, at)
	if u, v, ok := f.SampleVector(c); !ok || math.Abs(float64(u)-12) > 0.05 || math.Abs(float64(v)-5) > 0.05 {
		t.Errorf("wind aloft %v, %v", u, v)
	}
	if g := run.Fields[weather.CloudLow][0].Grid; g.W > 20 || g.H > 20 {
		t.Errorf("the grid was not cropped to the region: %d by %d", g.W, g.H)
	}
	// Everything again from the cache.
	srv.mu.Lock()
	before := srv.gets
	srv.mu.Unlock()
	if _, err := p.Fetch(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.gets != before {
		t.Errorf("%d files fetched again", srv.gets-before)
	}
}
