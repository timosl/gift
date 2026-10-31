package weather

import (
	"context"
	"math"
	"time"

	"github.com/worldiety/gift/geo"
)

// Combine returns a provider that takes primary where it covers the region
// and fallback everywhere else: the Deutscher Wetterdienst's 2.2 km over
// Germany and the GFS's 28 km around it, say. The fields are put together
// on one grid over the region, at the primary's spacing where it is fine
// enough to afford, and across a band at the edge of the primary's
// coverage one blends into the other, so that a map panned past the edge
// sees no jump.
//
// A failure of the primary is no failure of the whole: the fallback then
// covers all of it.
func Combine(primary, fallback Provider) Provider {
	return &combined{a: primary, b: fallback}
}

type combined struct{ a, b Provider }

// blendDegrees is the width of the band at the primary's edge over which
// the two are blended.
const blendDegrees = 0.5

// maxCombined bounds the grid of a combined field along each axis.
const maxCombined = 768

func (c *combined) Info() ProviderInfo {
	ia, ib := c.a.Info(), c.b.Info()
	vars := ia.Variables
	for _, v := range ib.Variables {
		found := false
		for _, w := range vars {
			found = found || w == v
		}
		if !found {
			vars = append(vars, v)
		}
	}
	return ProviderInfo{
		Name: ia.Name + "+" + ib.Name, Coverage: ib.Coverage, Resolution: ia.Resolution,
		Step: min(ia.Step, ib.Step), Horizon: min(ia.Horizon, ib.Horizon), Cycle: min(ia.Cycle, ib.Cycle),
		Variables: vars, Attribution: ia.Attribution + ", " + ib.Attribution,
	}
}

func (c *combined) Fetch(ctx context.Context, q Query) (*Run, error) {
	ia := c.a.Info()
	inside := q.Region.Min.Lat >= ia.Coverage.Min.Lat && q.Region.Max.Lat <= ia.Coverage.Max.Lat &&
		q.Region.Min.Lon >= ia.Coverage.Min.Lon && q.Region.Max.Lon <= ia.Coverage.Max.Lon
	if inside {
		// Where the primary covers everything there is nothing to combine;
		// the band at its edge is outside the region.
		if run, err := c.a.Fetch(ctx, q); err == nil {
			return run, nil
		}
	}
	rb, err := c.b.Fetch(ctx, q)
	if err != nil {
		return nil, err
	}
	var ra *Run
	if q.Region.Intersects(ia.Coverage) {
		qa := q
		qa.Region = q.Region.Intersect(ia.Coverage)
		ra, _ = c.a.Fetch(ctx, qa)
	}
	if ra == nil {
		return rb, nil
	}
	out := &Run{Provider: c.Info().Name, Issued: ra.Issued, Fields: make(map[Variable][]*Field)}
	for v, fbs := range rb.Fields {
		fas := ra.Fields[v]
		times := fbs
		if len(fas) > 0 {
			times = fas
		}
		for _, tf := range times {
			out.Fields[v] = append(out.Fields[v], merge(ra.At(v, tf.Valid), rb.At(v, tf.Valid), q.Region, ia.Coverage, tf.Valid))
		}
	}
	return out, nil
}

// merge resamples a and b onto one grid over r: a inside cov, blended into
// b across the band at cov's edge, b outside.
func merge(a, b *Field, r, cov geo.Rect, t time.Time) *Field {
	step := b.Grid.DLat
	if a != nil {
		step = a.Grid.DLat
	}
	step = math.Max(step, math.Max(r.Max.Lat-r.Min.Lat, r.Max.Lon-r.Min.Lon)/maxCombined)
	g := Grid{Lat0: r.Max.Lat, Lon0: r.Min.Lon, DLat: step, DLon: step,
		W: int(math.Ceil((r.Max.Lon-r.Min.Lon)/step)) + 1, H: int(math.Ceil((r.Max.Lat-r.Min.Lat)/step)) + 1}
	out := &Field{Var: b.Var, Valid: t, Grid: g, Data: make([]float32, g.W*g.H)}
	vec := b.V != nil
	if vec {
		out.V = make([]float32, g.W*g.H)
	}
	nan := float32(math.NaN())
	for y := range g.H {
		for x := range g.W {
			c := geo.LatLon(g.Lat0-float64(y)*step, g.Lon0+float64(x)*step)
			i := y*g.W + x
			// How far inside the primary's coverage, as a weight.
			edge := math.Min(math.Min(c.Lat-cov.Min.Lat, cov.Max.Lat-c.Lat), math.Min(c.Lon-cov.Min.Lon, cov.Max.Lon-c.Lon))
			w := float32(math.Max(0, math.Min(1, edge/blendDegrees)))
			var av, aw, bv, bw float32
			okA, okB := false, false
			if w > 0 && a != nil {
				if vec {
					av, aw, okA = a.SampleVector(c)
				} else {
					av, okA = a.Sample(c)
				}
			}
			if w < 1 || !okA {
				if vec {
					bv, bw, okB = b.SampleVector(c)
				} else {
					bv, okB = b.Sample(c)
				}
			}
			switch {
			case okA && okB:
				out.Data[i] = av*w + bv*(1-w)
				if vec {
					out.V[i] = aw*w + bw*(1-w)
				}
			case okA:
				out.Data[i] = av
				if vec {
					out.V[i] = aw
				}
			case okB:
				out.Data[i] = bv
				if vec {
					out.V[i] = bw
				}
			default:
				out.Data[i] = nan
				if vec {
					out.V[i] = nan
				}
			}
		}
	}
	return out
}
