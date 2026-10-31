// Package weather is the weather a map of package ui/maps can show: the
// provider seam of the project plan, section 37, the fields of a model run
// on a regular grid, and a [Sky] that keeps the latest run of a provider.
//
// The drawing knows no provider. It sees normalised quantities — cloud
// cover in three layers as a fraction, precipitation in millimetres an hour,
// wind in metres a second — on a regular latitude and longitude grid; each
// provider crops its own grid to the region asked for, converts its units
// and derives what it does not deliver directly, the rate of precipitation
// from an accumulated sum, say. Package weather/dwd is the Deutscher
// Wetterdienst's ICON-D2 and weather/gfs NOAA's GFS.
package weather

import (
	"context"
	"math"
	"time"

	"github.com/worldiety/gift/geo"
)

// Variable is a normalised quantity.
type Variable uint8

const (
	// CloudLow, CloudMid and CloudHigh are the cloud cover of the low, the
	// middle and the high layer, as a fraction from 0 to 1.
	CloudLow Variable = iota
	CloudMid
	CloudHigh
	// PrecipRate is the precipitation in millimetres an hour.
	PrecipRate
	// Wind10m is the wind ten metres above the ground in metres a second, a
	// vector: towards the east in Data, towards the north in V.
	Wind10m
	// WindAloft is the wind at about 500 hPa, the height of the middle
	// clouds, which it drives.
	WindAloft
	// Temp2m is the temperature two metres above the ground in kelvin. A
	// map shows snow rather than rain where it is below freezing.
	Temp2m
	numVariables
)

func (v Variable) String() string {
	switch v {
	case CloudLow:
		return "cloud-low"
	case CloudMid:
		return "cloud-mid"
	case CloudHigh:
		return "cloud-high"
	case PrecipRate:
		return "precip-rate"
	case Wind10m:
		return "wind-10m"
	case WindAloft:
		return "wind-aloft"
	case Temp2m:
		return "temp-2m"
	}
	return "unknown"
}

// IsVector reports whether the variable has two components.
func (v Variable) IsVector() bool { return v == Wind10m || v == WindAloft }

// Grid is a regular latitude and longitude grid: W columns from Lon0
// eastward in steps of DLon, H rows from Lat0, the northernmost, southward in
// steps of DLat.
type Grid struct {
	Lat0, Lon0 float64
	DLat, DLon float64
	W, H       int
}

// Bounds returns the rectangle the grid's points span.
func (g Grid) Bounds() geo.Rect {
	return geo.Rect{
		Min: geo.LatLon(g.Lat0-float64(g.H-1)*g.DLat, g.Lon0),
		Max: geo.LatLon(g.Lat0, g.Lon0+float64(g.W-1)*g.DLon),
	}
}

// index returns the fractional column and row of a coordinate.
func (g Grid) index(c geo.Coordinate) (x, y float64) {
	return (c.Lon - g.Lon0) / g.DLon, (g.Lat0 - c.Lat) / g.DLat
}

// Field is one quantity at one time on a grid. Data holds W×H values row by
// row from the north; a vector field holds its northward component in V.
// A value that is not known is NaN.
type Field struct {
	Var   Variable
	Valid time.Time
	Grid  Grid
	Data  []float32
	V     []float32
}

// Sample returns the value at c, interpolated between the four grid points
// around it, and false outside the grid.
func (f *Field) Sample(c geo.Coordinate) (float32, bool) {
	if f == nil {
		return 0, false
	}
	return bilinear(f.Grid, f.Data, c)
}

// SampleVector returns both components at c.
func (f *Field) SampleVector(c geo.Coordinate) (u, v float32, ok bool) {
	if f == nil || f.V == nil {
		return 0, 0, false
	}
	u, ok = bilinear(f.Grid, f.Data, c)
	if !ok {
		return 0, 0, false
	}
	v, ok = bilinear(f.Grid, f.V, c)
	return u, v, ok
}

func bilinear(g Grid, d []float32, c geo.Coordinate) (float32, bool) {
	x, y := g.index(c)
	if x < 0 || y < 0 || x > float64(g.W-1) || y > float64(g.H-1) || len(d) < g.W*g.H {
		return 0, false
	}
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, g.W-1), min(y0+1, g.H-1)
	fx, fy := float32(x-float64(x0)), float32(y-float64(y0))
	a, b := d[y0*g.W+x0], d[y0*g.W+x1]
	cc, dd := d[y1*g.W+x0], d[y1*g.W+x1]
	v := (a*(1-fx)+b*fx)*(1-fy) + (cc*(1-fx)+dd*fx)*fy
	if math.IsNaN(float64(v)) {
		return 0, false
	}
	return v, true
}

// Lerp returns the field between a and b at t, which must lie between their
// valid times, on a's grid. The two must share their grid.
func Lerp(a, b *Field, t time.Time) *Field {
	span := b.Valid.Sub(a.Valid)
	if span <= 0 || a.Grid != b.Grid {
		return a
	}
	f := float32(t.Sub(a.Valid)) / float32(span)
	f = max(0, min(1, f))
	out := &Field{Var: a.Var, Valid: t, Grid: a.Grid, Data: make([]float32, len(a.Data))}
	for i := range a.Data {
		out.Data[i] = a.Data[i] + (b.Data[i]-a.Data[i])*f
	}
	if a.V != nil && b.V != nil {
		out.V = make([]float32, len(a.V))
		for i := range a.V {
			out.V[i] = a.V[i] + (b.V[i]-a.V[i])*f
		}
	}
	return out
}

// Run is the fields of one model run.
type Run struct {
	// Provider names the provider and Issued is the time of the run.
	Provider string
	Issued   time.Time
	// Fields holds every variable's fields in the order of their valid
	// times.
	Fields map[Variable][]*Field
}

// At returns the field of v at t, interpolated between the two steps
// around it and held at the ends; nil when the run has no such field.
func (r *Run) At(v Variable, t time.Time) *Field {
	if r == nil {
		return nil
	}
	fs := r.Fields[v]
	switch {
	case len(fs) == 0:
		return nil
	case !t.After(fs[0].Valid):
		return fs[0]
	case !t.Before(fs[len(fs)-1].Valid):
		return fs[len(fs)-1]
	}
	for i := 1; i < len(fs); i++ {
		if t.Before(fs[i].Valid) {
			return Lerp(fs[i-1], fs[i], t)
		}
	}
	return fs[len(fs)-1]
}

// Query is what a provider is asked for: the variables over a region and a
// time window.
type Query struct {
	Region    geo.Rect
	From, To  time.Time
	Variables []Variable
}

// ProviderInfo is what makes a weather provider what it is.
type ProviderInfo struct {
	Name string
	// Coverage is the area the model covers.
	Coverage geo.Rect
	// Resolution is the nominal grid spacing in kilometres.
	Resolution float64
	// Step is the time between fields, Horizon how far the model looks
	// ahead and Cycle the time between runs.
	Step, Horizon, Cycle time.Duration
	// Variables is what the provider delivers.
	Variables []Variable
	// Attribution is the notice the provider's licence asks for.
	Attribution string
}

// Provider delivers the fields of a weather model. It is the weather seam of
// the project plan, section 37: the Deutscher Wetterdienst and NOAA's GFS
// implement it, and so can anybody else's model.
//
// Fetch returns the fields of the latest run that is complete, for the
// variables of the query it has, cropped to the region and normalised; a
// variable it does not have is left out, and the map leaves out what it
// would have drawn of it.
type Provider interface {
	Info() ProviderInfo
	Fetch(ctx context.Context, q Query) (*Run, error)
}
