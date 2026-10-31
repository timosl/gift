// Package dwd is the weather of the Deutscher Wetterdienst behind the seam
// of package weather: the ICON-D2 model, 2.2 km over Germany and its
// neighbours, from the open data server, free for any use with the source
// named.
//
// A run is complete about two hours after its time; runs are every three
// hours. Every field of a step is its own file of 0.7 MB, compressed, so a
// provider fetches only the hours it is asked for, and keeps the files in a
// cache directory, where a run is immutable.
package dwd

import (
	"bytes"
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/weather"
	"github.com/worldiety/gift/weather/grib2"
)

// DefaultBase is the ICON-D2 directory of the open data server.
const DefaultBase = "https://opendata.dwd.de/weather/nwp/icon-d2/grib"

// Options configures [ICOND2].
type Options struct {
	// Base is the directory of the runs; empty selects [DefaultBase].
	Base string
	// CacheDir keeps the downloaded files; empty keeps nothing.
	CacheDir string
	// Client is the HTTP client; nil selects http.DefaultClient.
	Client *http.Client
	// UserAgent names the application; empty selects
	// [asset.DefaultUserAgent].
	UserAgent string
	// Now is the clock the latest run is found by; nil is time.Now.
	Now func() time.Time
}

// ICOND2 returns the provider of the ICON-D2 model.
func ICOND2(o Options) weather.Provider {
	if o.Base == "" {
		o.Base = DefaultBase
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	if o.UserAgent == "" {
		o.UserAgent = asset.DefaultUserAgent
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &icon{o: o}
}

type icon struct {
	o Options
}

func (p *icon) Info() weather.ProviderInfo {
	return weather.ProviderInfo{
		Name:       "dwd-icon-d2",
		Coverage:   geo.Rect{Min: geo.LatLon(43.18, -3.94), Max: geo.LatLon(58.08, 20.34)},
		Resolution: 2.2,
		Step:       time.Hour, Horizon: 48 * time.Hour, Cycle: 3 * time.Hour,
		Variables: []weather.Variable{
			weather.CloudLow, weather.CloudMid, weather.CloudHigh, weather.PrecipRate,
			weather.Wind10m, weather.WindAloft, weather.Temp2m,
		},
		Attribution: "Datenbasis: Deutscher Wetterdienst",
	}
}

// file is one field of one step: the directory and the name on the
// server.
type file struct{ dir, name string }

// files returns the files of a variable at a run and a step: two for a
// vector, and for the precipitation rate the accumulation of the step
// before as well.
func files(v weather.Variable, run time.Time, step int) []file {
	stamp := run.Format("2006010215")
	single := func(name string, s int) file {
		return file{name, fmt.Sprintf("icon-d2_germany_regular-lat-lon_single-level_%s_%03d_2d_%s.grib2.bz2", stamp, s, name)}
	}
	pressure := func(name string) file {
		return file{name, fmt.Sprintf("icon-d2_germany_regular-lat-lon_pressure-level_%s_%03d_500_%s.grib2.bz2", stamp, step, name)}
	}
	switch v {
	case weather.CloudLow:
		return []file{single("clcl", step)}
	case weather.CloudMid:
		return []file{single("clcm", step)}
	case weather.CloudHigh:
		return []file{single("clch", step)}
	case weather.PrecipRate:
		if step == 0 {
			return []file{single("tot_prec", 0), single("tot_prec", 1)}
		}
		return []file{single("tot_prec", step-1), single("tot_prec", step)}
	case weather.Wind10m:
		return []file{single("u_10m", step), single("v_10m", step)}
	case weather.WindAloft:
		return []file{pressure("u"), pressure("v")}
	case weather.Temp2m:
		return []file{single("t_2m", step)}
	}
	return nil
}

// Fetch returns the latest complete run's fields over the query.
func (p *icon) Fetch(ctx context.Context, q weather.Query) (*weather.Run, error) {
	if q.Region.IsEmpty() {
		q.Region = p.Info().Coverage
	}
	if q.From.IsZero() {
		q.From = p.o.Now()
	}
	if q.To.Before(q.From) || q.To.IsZero() {
		q.To = q.From.Add(3 * time.Hour)
	}
	vars := q.Variables
	if len(vars) == 0 {
		vars = p.Info().Variables
	}
	run, err := p.latestRun(ctx, vars[0], q.To)
	if err != nil {
		return nil, err
	}
	first := max(0, int(math.Floor(q.From.Sub(run).Hours())))
	last := min(48, int(math.Ceil(q.To.Sub(run).Hours())))
	out := &weather.Run{Provider: "dwd-icon-d2", Issued: run, Fields: make(map[weather.Variable][]*weather.Field)}
	for _, v := range vars {
		for step := first; step <= last; step++ {
			f, err := p.field(ctx, v, run, step, q.Region)
			if err != nil {
				return nil, fmt.Errorf("dwd: %v at +%dh: %w", v, step, err)
			}
			out.Fields[v] = append(out.Fields[v], f)
		}
	}
	return out, nil
}

// latestRun finds the newest run that has the last step the query needs on
// the server: runs are every three hours, and their steps appear one after
// the other over the two hours after, so a run whose first step is there may
// not have the one asked for yet.
func (p *icon) latestRun(ctx context.Context, v weather.Variable, to time.Time) (time.Time, error) {
	now := p.o.Now().UTC()
	t := now.Truncate(3 * time.Hour)
	for range 4 {
		last := min(48, int(math.Ceil(to.Sub(t).Hours())))
		f := files(v, t, last)[0]
		ok, err := p.exists(ctx, f, t)
		if err != nil {
			return time.Time{}, err
		}
		if ok {
			return t, nil
		}
		t = t.Add(-3 * time.Hour)
	}
	return time.Time{}, errors.New("dwd: no run of the last twelve hours is on the server")
}

func (p *icon) url(f file, run time.Time) string {
	return fmt.Sprintf("%s/%02d/%s/%s", strings.TrimRight(p.o.Base, "/"), run.Hour(), f.dir, f.name)
}

func (p *icon) exists(ctx context.Context, f file, run time.Time) (bool, error) {
	if p.o.CacheDir != "" {
		if _, err := os.Stat(filepath.Join(p.o.CacheDir, f.name)); err == nil {
			return true, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.url(f, run), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", p.o.UserAgent)
	resp, err := p.o.Client.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// load returns the decoded messages of a file, from the cache or the
// server.
func (p *icon) load(ctx context.Context, f file, run time.Time) ([]grib2.Message, error) {
	var raw []byte
	cached := ""
	if p.o.CacheDir != "" {
		cached = filepath.Join(p.o.CacheDir, f.name)
		raw, _ = os.ReadFile(cached)
	}
	if raw == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url(f, run), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", p.o.UserAgent)
		resp, err := p.o.Client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, &asset.StatusError{Status: resp.StatusCode}
		}
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, err
		}
		if cached != "" {
			if err := os.MkdirAll(p.o.CacheDir, 0o755); err == nil {
				tmp := cached + ".part"
				if os.WriteFile(tmp, raw, 0o644) == nil {
					os.Rename(tmp, cached)
				}
			}
		}
	}
	data := raw
	if bytes.HasPrefix(raw, []byte("BZh")) {
		var err error
		if data, err = io.ReadAll(bzip2.NewReader(bytes.NewReader(raw))); err != nil {
			return nil, fmt.Errorf("%s: %w", f.name, err)
		}
	}
	return grib2.Decode(data)
}

// field loads, crops and normalises one variable at one step.
func (p *icon) field(ctx context.Context, v weather.Variable, run time.Time, step int, region geo.Rect) (*weather.Field, error) {
	fs := files(v, run, step)
	var ms [][]grib2.Message
	for _, f := range fs {
		m, err := p.load(ctx, f, run)
		if err != nil {
			return nil, err
		}
		if len(m) == 0 {
			return nil, fmt.Errorf("%s holds no field", f.name)
		}
		ms = append(ms, m)
	}
	out := &weather.Field{Var: v, Valid: run.Add(time.Duration(step) * time.Hour)}
	g, d := crop(ms[0][0], region)
	out.Grid = g
	switch v {
	case weather.CloudLow, weather.CloudMid, weather.CloudHigh:
		for i := range d {
			d[i] /= 100
		}
		out.Data = d
	case weather.PrecipRate:
		// The accumulation since the run began, in kg/m², which is mm: the
		// rate of an hour is the difference to the hour before.
		_, after := crop(ms[1][0], region)
		for i := range d {
			d[i] = max(0, after[i]-d[i])
		}
		out.Data = d
	case weather.Wind10m, weather.WindAloft:
		_, vv := crop(ms[1][0], region)
		out.Data, out.V = d, vv
	default:
		out.Data = d
	}
	return out, nil
}

// crop cuts the grid of a message to the region, with one point to spare
// on every side for the interpolation at its edge.
func crop(m grib2.Message, r geo.Rect) (weather.Grid, []float32) {
	g := m.Grid
	x0 := int(math.Floor((r.Min.Lon-g.Lon0)/g.DLon)) - 1
	x1 := int(math.Ceil((r.Max.Lon-g.Lon0)/g.DLon)) + 1
	y0 := int(math.Floor((g.Lat0-r.Max.Lat)/g.DLat)) - 1
	y1 := int(math.Ceil((g.Lat0-r.Min.Lat)/g.DLat)) + 1
	x0, y0 = max(0, x0), max(0, y0)
	x1, y1 = min(g.Ni-1, x1), min(g.Nj-1, y1)
	if x1 < x0 || y1 < y0 {
		return weather.Grid{}, nil
	}
	w, h := x1-x0+1, y1-y0+1
	out := make([]float32, w*h)
	for y := range h {
		copy(out[y*w:(y+1)*w], m.Values[(y0+y)*g.Ni+x0:(y0+y)*g.Ni+x0+w])
	}
	return weather.Grid{
		Lat0: g.Lat0 - float64(y0)*g.DLat, Lon0: g.Lon0 + float64(x0)*g.DLon,
		DLat: g.DLat, DLon: g.DLon, W: w, H: h,
	}, out
}
