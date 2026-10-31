// Package gfs is NOAA's Global Forecast System behind the seam of package
// weather: the whole earth at 0.25 degrees, about 28 km, every hour to five
// days ahead, in the public domain.
//
// The fields come from NOMADS's GRIB filter, which cuts a region and the
// variables asked for out of a forecast hour and repacks them in simple
// packing, one request per hour for all of them: a few kilobytes for a
// country, where the whole file would be half a gigabyte. Runs are every
// six hours and are complete about five hours after their time.
package gfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/weather"
	"github.com/worldiety/gift/weather/grib2"
)

// The servers of NOMADS: the filter and the directory the files are listed
// in.
const (
	DefaultFilter = "https://nomads.ncep.noaa.gov/cgi-bin/filter_gfs_0p25.pl"
	DefaultFiles  = "https://nomads.ncep.noaa.gov/pub/data/nccf/com/gfs/prod"
)

// Options configures [Global].
type Options struct {
	// Filter and Files are the servers; empty selects the defaults.
	Filter, Files string
	// CacheDir keeps the fetched hours; empty keeps nothing.
	CacheDir string
	// Client is the HTTP client; nil selects http.DefaultClient.
	Client *http.Client
	// UserAgent names the application; empty selects
	// [asset.DefaultUserAgent].
	UserAgent string
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Global returns the provider of the GFS.
func Global(o Options) weather.Provider {
	if o.Filter == "" {
		o.Filter = DefaultFilter
	}
	if o.Files == "" {
		o.Files = DefaultFiles
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
	return &gfs{o: o}
}

type gfs struct{ o Options }

func (p *gfs) Info() weather.ProviderInfo {
	return weather.ProviderInfo{
		Name: "noaa-gfs", Coverage: geo.World, Resolution: 28,
		Step: time.Hour, Horizon: 120 * time.Hour, Cycle: 6 * time.Hour,
		Variables: []weather.Variable{
			weather.CloudLow, weather.CloudMid, weather.CloudHigh, weather.PrecipRate,
			weather.Wind10m, weather.WindAloft, weather.Temp2m,
		},
		Attribution: "NOAA GFS",
	}
}

// selector names a field in a forecast hour: the parameter, the surface
// and its level.
type selector struct {
	cat, num uint8
	surface  uint8
	level    float64
}

var (
	lcdc  = selector{6, 3, 214, 0}
	mcdc  = selector{6, 4, 224, 0}
	hcdc  = selector{6, 5, 234, 0}
	prate = selector{1, 7, 1, 0}
	u10   = selector{2, 2, 103, 10}
	v10   = selector{2, 3, 103, 10}
	u500  = selector{2, 2, 100, 50000}
	v500  = selector{2, 3, 100, 50000}
	t2m   = selector{0, 0, 103, 2}
)

// query is the variables and levels of the filter for the variables asked.
func query(vars []weather.Variable) url.Values {
	q := url.Values{}
	on := func(keys ...string) {
		for _, k := range keys {
			q.Set(k, "on")
		}
	}
	for _, v := range vars {
		switch v {
		case weather.CloudLow:
			on("var_LCDC", "lev_low_cloud_layer")
		case weather.CloudMid:
			on("var_MCDC", "lev_middle_cloud_layer")
		case weather.CloudHigh:
			on("var_HCDC", "lev_high_cloud_layer")
		case weather.PrecipRate:
			on("var_PRATE", "lev_surface")
		case weather.Wind10m:
			on("var_UGRD", "var_VGRD", "lev_10_m_above_ground")
		case weather.WindAloft:
			on("var_UGRD", "var_VGRD", "lev_500_mb")
		case weather.Temp2m:
			on("var_TMP", "lev_2_m_above_ground")
		}
	}
	return q
}

func (p *gfs) Fetch(ctx context.Context, q weather.Query) (*weather.Run, error) {
	if q.Region.IsEmpty() {
		return nil, errors.New("gfs: a query needs a region; the whole earth is half a gigabyte an hour")
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
	run, err := p.latestRun(ctx, q.To)
	if err != nil {
		return nil, err
	}
	first := max(0, int(math.Floor(q.From.Sub(run).Hours())))
	last := min(120, int(math.Ceil(q.To.Sub(run).Hours())))
	out := &weather.Run{Provider: "noaa-gfs", Issued: run, Fields: make(map[weather.Variable][]*weather.Field)}
	for step := max(first, 1); step <= max(last, 1); step++ {
		ms, err := p.hour(ctx, run, step, vars, q.Region)
		if err != nil {
			return nil, fmt.Errorf("gfs: +%dh: %w", step, err)
		}
		valid := run.Add(time.Duration(step) * time.Hour)
		for _, v := range vars {
			if f := field(ms, v, valid); f != nil {
				out.Fields[v] = append(out.Fields[v], f)
			}
		}
	}
	return out, nil
}

// latestRun finds the newest run that has the last hour asked for.
func (p *gfs) latestRun(ctx context.Context, to time.Time) (time.Time, error) {
	t := p.o.Now().UTC().Truncate(6 * time.Hour)
	for range 4 {
		step := min(120, max(1, int(math.Ceil(to.Sub(t).Hours()))))
		u := fmt.Sprintf("%s/gfs.%s/%02d/atmos/gfs.t%02dz.pgrb2.0p25.f%03d",
			strings.TrimRight(p.o.Files, "/"), t.Format("20060102"), t.Hour(), t.Hour(), step)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			return time.Time{}, err
		}
		req.Header.Set("User-Agent", p.o.UserAgent)
		resp, err := p.o.Client.Do(req)
		if err != nil {
			return time.Time{}, err
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return t, nil
		}
		t = t.Add(-6 * time.Hour)
	}
	return time.Time{}, errors.New("gfs: no run of the last day is complete")
}

// hour fetches the fields of one forecast hour through the filter, or
// reads them from the cache.
func (p *gfs) hour(ctx context.Context, run time.Time, step int, vars []weather.Variable, r geo.Rect) ([]grib2.Message, error) {
	q := query(vars)
	q.Set("dir", fmt.Sprintf("/gfs.%s/%02d/atmos", run.Format("20060102"), run.Hour()))
	q.Set("file", fmt.Sprintf("gfs.t%02dz.pgrb2.0p25.f%03d", run.Hour(), step))
	// The region on the grid's quarter degrees, so that the same view asks
	// the same question and the cache answers it.
	snap := func(v float64, up bool) float64 {
		if up {
			return math.Ceil(v*4) / 4
		}
		return math.Floor(v*4) / 4
	}
	q.Set("subregion", "")
	q.Set("toplat", fmt.Sprint(snap(r.Max.Lat, true)))
	q.Set("bottomlat", fmt.Sprint(snap(r.Min.Lat, false)))
	q.Set("leftlon", fmt.Sprint(snap(r.Min.Lon, false)))
	q.Set("rightlon", fmt.Sprint(snap(r.Max.Lon, true)))
	key := q.Encode()
	var raw []byte
	cached := ""
	if p.o.CacheDir != "" {
		cached = filepath.Join(p.o.CacheDir, fmt.Sprintf("%x.grib2", hash(key)))
		raw, _ = os.ReadFile(cached)
	}
	if raw == nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.o.Filter+"?"+key, nil)
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
		if raw, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20)); err != nil {
			return nil, err
		}
		if cached != "" && strings.HasPrefix(string(raw[:min(4, len(raw))]), "GRIB") {
			if os.MkdirAll(p.o.CacheDir, 0o755) == nil {
				os.WriteFile(cached+".part", raw, 0o644)
				os.Rename(cached+".part", cached)
			}
		}
	}
	return grib2.Decode(raw)
}

// field picks a variable out of the messages of an hour and normalises it:
// an instant over an interval's mean where the hour has both.
func field(ms []grib2.Message, v weather.Variable, valid time.Time) *weather.Field {
	pick := func(s selector) *grib2.Message {
		var found *grib2.Message
		for i := range ms {
			m := &ms[i]
			if m.Category == s.cat && m.Number == s.num && m.Surface == s.surface && m.Level == s.level {
				if found == nil || m.ProductTemplate == 0 {
					found = m
				}
			}
		}
		return found
	}
	conv := func(m *grib2.Message, scale float32) []float32 {
		out := make([]float32, len(m.Values))
		for i, x := range m.Values {
			out[i] = x * scale
		}
		return out
	}
	var a, b *grib2.Message
	var scale float32 = 1
	switch v {
	case weather.CloudLow:
		a, scale = pick(lcdc), 0.01
	case weather.CloudMid:
		a, scale = pick(mcdc), 0.01
	case weather.CloudHigh:
		a, scale = pick(hcdc), 0.01
	case weather.PrecipRate:
		// kg/m²/s is mm/s.
		a, scale = pick(prate), 3600
	case weather.Wind10m:
		a, b = pick(u10), pick(v10)
	case weather.WindAloft:
		a, b = pick(u500), pick(v500)
	case weather.Temp2m:
		a = pick(t2m)
	}
	if a == nil || (v.IsVector() && b == nil) {
		return nil
	}
	f := &weather.Field{Var: v, Valid: valid, Grid: weather.Grid{
		Lat0: a.Grid.Lat0, Lon0: a.Grid.Lon0, DLat: a.Grid.DLat, DLon: a.Grid.DLon, W: a.Grid.Ni, H: a.Grid.Nj,
	}, Data: conv(a, scale)}
	if b != nil {
		f.V = conv(b, 1)
	}
	return f
}

// hash is FNV-1a, for the names of the cached hours.
func hash(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
