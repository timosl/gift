package maps

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/weather"
)

// WeatherLayers chooses what a weather overlay draws.
type WeatherLayers uint8

const (
	// WeatherClouds is the three layers of clouds, low, middle and high,
	// each at its height above the map.
	WeatherClouds WeatherLayers = 1 << iota
	// WeatherShadows is the shadows the clouds cast on the ground, away
	// from the sun.
	WeatherShadows
	// WeatherRadar is the precipitation in the colours of a radar picture,
	// over the clouds, where it can be read.
	WeatherRadar
	// WeatherPrecipitation is falling rain, or snow where it freezes.
	WeatherPrecipitation
	// WeatherWind is the wind near the ground as streaks that follow it.
	WeatherWind
	// WeatherAll is everything.
	WeatherAll = WeatherClouds | WeatherShadows | WeatherRadar | WeatherPrecipitation | WeatherWind
)

// WeatherQuality is how much a weather overlay costs.
type WeatherQuality uint8

const (
	// WeatherFull draws the clouds from textures of 512 by 512 with five
	// octaves of noise, and a thousand streaks of wind on a full HD map.
	WeatherFull WeatherQuality = iota
	// WeatherReduced halves the textures and the noise and draws less than
	// half the streaks: the level of a Raspberry Pi.
	WeatherReduced
)

// WeatherOptions configures [Weather].
type WeatherOptions struct {
	// Layers is what to draw; zero draws all.
	Layers WeatherLayers
	// Quality is how much it may cost.
	Quality WeatherQuality
	// Time is the moment shown; nil is the sky's clock, now. A scrubber over
	// the forecast sets it.
	Time func() time.Time
}

// Weather returns an overlay that draws the weather of sky over a map, as
// the project plan, section 37, describes it: clouds in three layers at
// their heights, which move against the ground as the map is panned and
// cast shadows away from the sun, precipitation on the ground and in the
// air, and the wind near the ground.
//
// Nothing of it is computed per pixel in a frame. The clouds and the radar
// picture are drawn into textures on a goroutine of their own whenever the
// data, the zoom or the region change, and in a frame they are pictures,
// moved by the wind aloft and by the parallax of their height. The streaks
// of wind and rain are particles, moved on the processor and drawn as
// triangles in one operation each. The overlay is long lived: create it
// once with the map.
func Weather(sky *weather.Sky, o WeatherOptions) Overlay {
	if o.Layers == 0 {
		o.Layers = WeatherAll
	}
	return &weatherOverlay{sky: sky, o: o, rng: rand.New(rand.NewPCG(7, 31))}
}

// The heights of the cloud layers in kilometres: low clouds up to 2 km,
// middle ones around 4 km, high ones around 9 km.
var layerHeight = [3]float64{1.5, 4, 9}

// layerOpacity is how much of the ground each layer hides at full cover:
// low cloud is thick, high cloud thin.
var layerOpacity = [3]float64{0.72, 0.58, 0.36}

// layerScale is the wavelength of the largest cloud features of each layer,
// in kilometres.
var layerScale = [3]float64{6, 10, 18}

type weatherOverlay struct {
	sky *weather.Sky
	o   WeatherOptions
	rng *rand.Rand

	baking atomic.Bool
	baked  atomic.Pointer[weatherBake]
	cur    *weatherBake
	slots  [4]texSlot // the three cloud layers and the radar picture

	wind      []windParticle
	rain      []rainParticle
	lastT     time.Duration
	windVerts []render.Vertex
	rainVerts []render.Vertex
	sun       sunState
	view      Projection
	last      Projection
	now       time.Time
}

// weatherBake is what a bake produced: textures over a rectangle of the
// world, and the fields of the moment, for the particles.
type weatherBake struct {
	key     uint64
	version uint64
	rect    [4]float64 // minX, minY, maxX, maxY in world units
	zoom    float64
	at      time.Time
	clouds  [3]*image.RGBA
	radar   *image.RGBA
	wind    *weather.Field
	aloft   *weather.Field
	precip  *weather.Field
	temp    *weather.Field
	drift   [2]float64 // world units per second, the mean wind aloft
}

// texSlot is the texture of one layer and the bake it was made from.
type texSlot struct {
	bake *weatherBake
	h    render.ImageHandle
	ok   bool
}

type windParticle struct {
	trail      [4]geo.Point
	n          int
	age, life  float64
	sinceTrail float64
}

type rainParticle struct {
	x, y      float64 // map coordinates
	age, life float64
}

type sunState struct {
	elev, az float64 // radians; az clockwise from north
}

var bakeKeys atomic.Uint64

// LayoutOverlay keeps the bake current and moves the particles.
func (w *weatherOverlay) LayoutOverlay(ctx *gift.LayoutContext, p Projection) bool {
	w.view = p
	now := ctx.Now()
	if w.o.Time != nil {
		w.now = w.o.Time()
	} else {
		w.now = w.sky.Now()
	}
	run := w.sky.Run()
	// The region the sky keeps follows the map, generously.
	vis := p.Visible()
	w.sky.SetRegion(grow(vis, 0.5))
	if b := w.baked.Load(); b != nil && b != w.cur {
		w.cur = b
	}
	// A bake waits until the map rests, unless there is nothing to draw
	// at all or a pan is about to leave the textures: it competes for the
	// processor with the frame and with the rasterising of the tiles a zoom
	// brings, and the textures of the last one, moved and scaled, stand in
	// for a few frames.
	moved := p.v != w.last.v
	w.last = p
	if run != nil && !w.baking.Load() {
		if need, urgent := w.needsBake(run, p); need && (w.cur == nil || !moved || urgent) {
			w.startBake(run, p)
		}
	}
	if b := w.cur; b != nil {
		c := geo.Point{X: (b.rect[0] + b.rect[2]) / 2, Y: (b.rect[1] + b.rect[3]) / 2}.Coordinate()
		w.sun = sunAt(w.now, c)
	}
	dt := 0.0
	if w.lastT != 0 {
		dt = math.Max(0, math.Min(0.1, (now-w.lastT).Seconds()))
	}
	w.lastT = now
	w.moveWind(p, dt)
	w.moveRain(p, dt)
	return true
}

// needsBake reports whether the textures no longer suit the view: a new
// run, a zoom a level away, a view near the edge of the rectangle, or ten
// minutes of the forecast passed. Urgent is a view that will leave the
// textures soon, closer to their edge than a quarter of its size, or has
// already; that bake does not wait until the map rests.
func (w *weatherOverlay) needsBake(run *weather.Run, p Projection) (need, urgent bool) {
	b := w.cur
	if b == nil {
		return true, true
	}
	minX, minY, maxX, maxY := worldBox(p)
	dx := p.Center().X - (b.rect[0]+b.rect[2])/2
	dx -= math.Round(dx)
	cx := (b.rect[0] + b.rect[2]) / 2
	minX, maxX = minX-p.Center().X+cx+dx, maxX-p.Center().X+cx+dx
	mx, my := (maxX-minX)/4, (maxY-minY)/4
	urgent = minX-mx < b.rect[0] || maxX+mx > b.rect[2] ||
		(minY-my < b.rect[1] && b.rect[1] > 0) || (maxY+my > b.rect[3] && b.rect[3] < 1)
	need = urgent || b.version != w.sky.Version() || math.Abs(b.zoom-p.Zoom()) > 0.75 ||
		w.now.Sub(b.at).Abs() > 10*time.Minute
	return need, urgent
}

// worldBox is the world box of the turned viewport.
func worldBox(p Projection) (minX, minY, maxX, maxY float64) {
	w, h := p.Size()
	minX, minY, maxX, maxY = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{0, 0}, {w, 0}, {w, h}, {0, h}} {
		pt := p.ToWorld(c[0], c[1])
		minX, maxX = math.Min(minX, pt.X), math.Max(maxX, pt.X)
		minY, maxY = math.Min(minY, pt.Y), math.Max(maxY, pt.Y)
	}
	return
}

func grow(r geo.Rect, f float64) geo.Rect {
	dl, dn := (r.Max.Lat-r.Min.Lat)*f, (r.Max.Lon-r.Min.Lon)*f
	return geo.Rect{
		Min: geo.LatLon(math.Max(-85, r.Min.Lat-dl), math.Max(-180, r.Min.Lon-dn)),
		Max: geo.LatLon(math.Min(85, r.Max.Lat+dl), math.Min(180, r.Max.Lon+dn)),
	}
}

// startBake draws the textures for the view on a goroutine: a rectangle
// twice the view's extent around it, so that a pan does not need another.
func (w *weatherOverlay) startBake(run *weather.Run, p Projection) {
	minX, minY, maxX, maxY := worldBox(p)
	ex, ey := (maxX-minX)/2, (maxY-minY)/2
	rect := [4]float64{minX - ex, math.Max(0, minY-ey), maxX + ex, math.Min(1, maxY+ey)}
	res := 512
	octaves := 5
	if w.o.Quality == WeatherReduced {
		res, octaves = 256, 3
	}
	at, version, zoom, layers := w.now, w.sky.Version(), p.Zoom(), w.o.Layers
	w.baking.Store(true)
	go func() {
		defer w.baking.Store(false)
		w.baked.Store(bakeWeather(run, at, version, zoom, rect, res, octaves, layers))
	}()
}

// bakeWeather draws the cloud layers and the radar picture over rect.
func bakeWeather(run *weather.Run, at time.Time, version uint64, zoom float64, rect [4]float64, res, octaves int, layers WeatherLayers) *weatherBake {
	b := &weatherBake{key: bakeKeys.Add(1), version: version, rect: rect, zoom: zoom, at: at,
		wind: run.At(weather.Wind10m, at), aloft: run.At(weather.WindAloft, at),
		precip: run.At(weather.PrecipRate, at), temp: run.At(weather.Temp2m, at)}
	ww, wh := rect[2]-rect[0], rect[3]-rect[1]
	if ww <= 0 || wh <= 0 {
		return b
	}
	W := res
	H := max(16, min(2*res, int(float64(res)*wh/ww)))
	centre := geo.Point{X: (rect[0] + rect[2]) / 2, Y: (rect[1] + rect[3]) / 2}.Coordinate()
	if u, v, ok := b.aloft.SampleVector(centre); ok {
		// m/s to world units a second: a world unit is the circumference of
		// the parallel.
		m := geo.MetresPerUnit(centre.Lat)
		b.drift = [2]float64{float64(u) / m, -float64(v) / m}
	}
	// The noise moves with the wind aloft since the run began, so that a
	// bake later in the hour continues the clouds of the one before.
	since := at.Sub(run.Issued).Seconds()
	nx, ny := b.drift[0]*since, b.drift[1]*since
	texel := ww / float64(W)
	kmPerUnit := geo.MetresPerUnit(centre.Lat) / 1000
	coords := make([]geo.Coordinate, W*H)
	for y := range H {
		wy := rect[1] + (float64(y)+0.5)*wh/float64(H)
		for x := range W {
			wx := rect[0] + (float64(x)+0.5)*ww/float64(W)
			coords[y*W+x] = geo.Point{X: wx, Y: wy}.Coordinate()
		}
	}
	// The textures fade out at their edges, over a sixteenth of them, so
	// that a pan which outruns the next bake sees the clouds thin out
	// rather than end in a line. The poles of the world are no edge.
	fadeX, fadeY := make([]float64, W), make([]float64, H)
	for x := range W {
		fadeX[x] = smoothstep(0, float64(W)/16, math.Min(float64(x)+0.5, float64(W-x)-0.5))
	}
	for y := range H {
		top, bottom := float64(y)+0.5, float64(H-y)-0.5
		if rect[1] <= 0 {
			top = math.Inf(1)
		}
		if rect[3] >= 1 {
			bottom = math.Inf(1)
		}
		fadeY[y] = smoothstep(0, float64(H)/16, math.Min(top, bottom))
	}
	fade := func(x, y int) float64 { return math.Min(fadeX[x], fadeY[y]) }
	if layers&(WeatherClouds|WeatherShadows) != 0 {
		for li, v := range [3]weather.Variable{weather.CloudLow, weather.CloudMid, weather.CloudHigh} {
			f := run.At(v, at)
			if f == nil {
				continue
			}
			img := image.NewRGBA(image.Rect(0, 0, W, H))
			// The largest features at the layer's scale, but never finer
			// than eight texels, and octaves down to two texels.
			wave := math.Max(layerScale[li]/kmPerUnit, 8*texel)
			freq := 1 / wave
			oct := 1
			for o := 1; o < octaves && wave/math.Exp2(float64(o)) > 2*texel; o++ {
				oct = o + 1
			}
			seed := float64(li) * 41.7
			any := false
			for y := range H {
				wy := rect[1] + (float64(y)+0.5)*wh/float64(H)
				for x := range W {
					cover, ok := f.Sample(coords[y*W+x])
					if !ok || cover <= 0.02 {
						continue
					}
					wx := rect[0] + (float64(x)+0.5)*ww/float64(W)
					n := fbm((wx-nx)*freq+seed, (wy-ny)*freq-seed, oct)
					d := cloudDensity(float64(cover), n)
					if d <= 0 {
						continue
					}
					a := d * layerOpacity[li] * fade(x, y)
					// A little grey where the cloud is thin, as its underside
					// shows through.
					shade := 0.86 + 0.14*d
					img.SetRGBA(x, y, color.RGBA{uint8(255 * shade * a), uint8(255 * shade * a), uint8(255 * (shade + 0.02) * a), uint8(255 * a)})
					any = true
				}
			}
			// A clear layer is no layer: a transparent picture the size of
			// the screen still costs its fill on every frame, and on a
			// Raspberry Pi three of them were a third of the frame.
			if any {
				b.clouds[li] = img
			}
		}
	}
	if layers&WeatherRadar != 0 && b.precip != nil {
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		any := false
		for i, c := range coords {
			r, ok := b.precip.Sample(c)
			if !ok || r < 0.1 {
				continue
			}
			c := radarColour(float64(r))
			if k := fade(i%W, i/W); k < 1 {
				c = color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), uint8(float64(c.A) * k)}
			}
			img.SetRGBA(i%W, i/W, c)
			any = true
		}
		if any {
			b.radar = img
		}
	}
	return b
}

// cloudDensity turns cover and noise into the density of a texel: the
// noise is cut at a level that leaves the fraction cover of the sky
// clouded, with a soft edge.
func cloudDensity(cover, n float64) float64 {
	cut := 1 - cover
	return smoothstep(cut-0.08, cut+0.22, n)
}

func smoothstep(a, b, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// fbm is fractal value noise, normalised to about 0 to 1.
func fbm(x, y float64, octaves int) float64 {
	v, a, sum := 0.0, 0.5, 0.0
	for range octaves {
		v += a * vnoise(x, y)
		sum += a
		x, y = x*2.03+17.1, y*2.03+9.2
		a *= 0.5
	}
	return v / sum
}

func vnoise(x, y float64) float64 {
	ix, iy := math.Floor(x), math.Floor(y)
	fx, fy := x-ix, y-iy
	ux, uy := fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a, b := hash21(ix, iy), hash21(ix+1, iy)
	c, d := hash21(ix, iy+1), hash21(ix+1, iy+1)
	return (a+(b-a)*ux)*(1-uy) + (c+(d-c)*ux)*uy
}

// hash21 is the hash without sine of the glass grain, in float64: a
// lattice point to a value in [0, 1).
func hash21(x, y float64) float64 {
	frac := func(v float64) float64 { return v - math.Floor(v) }
	p3x, p3y, p3z := frac(x*0.1031), frac(y*0.1031), frac(x*0.1031)
	d := p3x*(p3y+33.33) + p3y*(p3z+33.33) + p3z*(p3x+33.33)
	p3x, p3y, p3z = p3x+d, p3y+d, p3z+d
	return frac((p3x + p3y) * p3z)
}

// radarColour is the colour of a rate of precipitation, the scale of a
// radar picture: light blue drizzle, blue rain, green, yellow, orange and
// red downpours.
func radarColour(r float64) color.RGBA {
	type stop struct {
		rate       float64
		r, g, b, a float64
	}
	stops := [...]stop{
		{0.1, 158, 201, 255, 0.35}, {0.5, 79, 143, 247, 0.5}, {2, 46, 196, 92, 0.6},
		{5, 245, 229, 59, 0.66}, {10, 245, 155, 46, 0.72}, {20, 232, 67, 46, 0.78},
	}
	i := 1
	for i < len(stops)-1 && r > stops[i].rate {
		i++
	}
	a, b := stops[i-1], stops[i]
	t := math.Max(0, math.Min(1, math.Log(r/a.rate)/math.Log(b.rate/a.rate)))
	lerp := func(x, y float64) float64 { return x + (y-x)*t }
	al := lerp(a.a, b.a)
	return color.RGBA{uint8(lerp(a.r, b.r) * al), uint8(lerp(a.g, b.g) * al), uint8(lerp(a.b, b.b) * al), uint8(255 * al)}
}

// sunAt is the position of the sun at t over c, by the low precision
// formulas of the US Naval Observatory's almanac: good to a fraction of a
// degree, which a shadow needs.
func sunAt(t time.Time, c geo.Coordinate) sunState {
	const rad = math.Pi / 180
	d := t.UTC().Sub(time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)).Hours() / 24
	g := (357.529 + 0.98560028*d) * rad
	q := 280.459 + 0.98564736*d
	l := (q + 1.915*math.Sin(g) + 0.020*math.Sin(2*g)) * rad
	e := (23.439 - 0.00000036*d) * rad
	ra := math.Atan2(math.Cos(e)*math.Sin(l), math.Cos(l))
	dec := math.Asin(math.Sin(e) * math.Sin(l))
	gmst := math.Mod(18.697374558+24.06570982441908*d, 24)
	ha := (gmst*15+c.Lon)*rad - ra
	lat := c.Lat * rad
	elev := math.Asin(math.Sin(lat)*math.Sin(dec) + math.Cos(lat)*math.Cos(dec)*math.Cos(ha))
	az := math.Atan2(-math.Sin(ha), math.Tan(dec)*math.Cos(lat)-math.Sin(lat)*math.Cos(ha))
	return sunState{elev: elev, az: az}
}

// parallax is how much larger a layer at height h kilometres appears than
// the ground, seen from the height a camera would have to show the map as
// wide as it is: H/(H−h), at most three times.
func parallax(p Projection, h float64) float64 {
	w, _ := p.Size()
	lat := p.Center().Coordinate().Lat
	widthKm := w / p.Scale() * geo.MetresPerUnit(lat) / 1000
	cam := widthKm * 0.87
	return cam / math.Max(cam-h, cam/3)
}

// PaintOverlay draws the weather: the shadows on the ground, the wind, the
// rain, the clouds above them, and the radar picture over everything.
func (w *weatherOverlay) PaintOverlay(ctx *gift.PaintContext, p Projection, bounds geom.Rect) {
	b := w.cur
	im := ctx.Images()
	if b != nil && im != nil {
		ws, hs := p.Size()
		mid := geom.Pt(bounds.Min.X+float32(ws)/2, bounds.Min.Y+float32(hs)/2)
		turned := p.Heading() != 0
		if turned {
			ctx.PushTransform(geom.Translate(geom.Pt(-mid.X, -mid.Y)).Mul(geom.Rotate(float32(-p.Heading()))).Mul(geom.Translate(mid)))
		}
		// How far the clouds of a bake have moved since it was made.
		drift := func(b *weatherBake) [2]float64 {
			since := w.now.Sub(b.at).Seconds()
			return [2]float64{b.drift[0] * since, b.drift[1] * since}
		}
		day := math.Max(0, math.Min(1, w.sun.elev/(10*math.Pi/180)))
		if w.o.Layers&WeatherShadows != 0 && day > 0 && w.sun.elev > 3*math.Pi/180 {
			lat := p.Center().Coordinate().Lat
			kmPerUnit := geo.MetresPerUnit(lat) / 1000
			for li := range 3 {
				id, lb, ok := w.texture(im, li, b, b.clouds[li])
				if !ok {
					continue
				}
				// Away from the sun, the further the higher the layer and
				// the lower the sun.
				dist := layerHeight[li] / math.Tan(w.sun.elev) / kmPerUnit
				d := drift(lb)
				off := [2]float64{d[0] - math.Sin(w.sun.az)*dist, d[1] + math.Cos(w.sun.az)*dist}
				w.drawLayer(ctx, p, bounds, id, lb, off, 1, ui.RGBA(0, 0, 0, uint8(70*day)))
			}
		}
		if turned {
			ctx.PopTransform()
		}
		ctx.PushTransform(geom.Translate(bounds.Min))
		emitTriangles(ctx, w.windVerts, bounds)
		emitTriangles(ctx, w.rainVerts, bounds)
		ctx.PopTransform()
		if turned {
			ctx.PushTransform(geom.Translate(geom.Pt(-mid.X, -mid.Y)).Mul(geom.Rotate(float32(-p.Heading()))).Mul(geom.Translate(mid)))
		}
		if w.o.Layers&WeatherClouds != 0 {
			// At night the clouds are grey, but not so dark that the map
			// under an overcast sky disappears.
			light := 0.72 + 0.28*day
			for li := range 3 {
				id, lb, ok := w.texture(im, li, b, b.clouds[li])
				if !ok {
					continue
				}
				k := parallax(p, layerHeight[li])
				g := uint8(255 * light)
				w.drawLayer(ctx, p, bounds, id, lb, drift(lb), k, ui.RGB(g, g, g))
			}
		}
		// The radar picture over the clouds, as weather apps draw it: under
		// an overcast sky it would be hidden, and it is what one wants to
		// read.
		if w.o.Layers&WeatherRadar != 0 {
			if id, lb, ok := w.texture(im, 3, b, b.radar); ok {
				w.drawLayer(ctx, p, bounds, id, lb, [2]float64{}, 1, ui.OpaqueWhite)
			}
		}
		if turned {
			ctx.PopTransform()
		}
	}
	// The source the weather comes from, as its licence asks, above the
	// map's own attribution.
	if attr := w.sky.Info().Attribution; attr != "" {
		ts := ui.TextStyle{Size: 10, Color: ui.RGB(40, 44, 52), Halo: ui.RGBA(255, 255, 255, 200)}
		sz := ui.MeasureText(attr, ts)
		ui.DrawText(ctx, attr, geom.Pt(bounds.Max.X-sz.W-8, bounds.Max.Y-sz.H-26), ts)
	}
}

// texture returns the texture of a slot and the bake it was made from: the
// current bake's once it is uploaded, and until then the one before it, so
// that a new bake replaces the old on the screen without a frame between
// them in which neither is there. A layer the current bake left clear has
// no texture.
func (w *weatherOverlay) texture(im render.Images, slot int, b *weatherBake, img *image.RGBA) (render.ImageID, *weatherBake, bool) {
	s := &w.slots[slot]
	if img == nil {
		if s.ok {
			im.Deallocate(s.h)
		}
		*s = texSlot{}
		return 0, nil, false
	}
	if s.bake != b || !s.ok {
		h, ok := im.Acquire(render.Pixels{Pix: img.Pix, W: img.Rect.Dx(), H: img.Rect.Dy(), Stride: img.Stride,
			Key: b.key*8 + uint64(slot) + 1})
		if ok {
			if s.ok {
				im.Deallocate(s.h)
			}
			*s = texSlot{bake: b, h: h, ok: true}
		}
		// Deferred by the upload budget, the old texture stays until a
		// later frame has uploaded the new one.
	}
	if !s.ok {
		return 0, nil, false
	}
	id, ok := im.Resolve(s.h)
	if !ok {
		// Evicted; the next frame uploads it again.
		*s = texSlot{}
		return 0, nil, false
	}
	return id, s.bake, true
}

// drawLayer draws a texture of the bake b over its rectangle, moved by off
// in world units and enlarged k times about the middle of the map, in the
// unrotated space of the map.
func (w *weatherOverlay) drawLayer(ctx *gift.PaintContext, p Projection, bounds geom.Rect,
	id render.ImageID, b *weatherBake, off [2]float64, k float64, tint ui.Color) {
	v := p.v
	cx := (b.rect[0]+b.rect[2])/2 + off[0]
	dx := cx - v.c.X
	dx -= math.Round(dx)
	x0 := v.w/2 + (dx-(b.rect[2]-b.rect[0])/2)*v.scale
	x1 := v.w/2 + (dx+(b.rect[2]-b.rect[0])/2)*v.scale
	y0 := v.h/2 + (b.rect[1]+off[1]-v.c.Y)*v.scale
	y1 := v.h/2 + (b.rect[3]+off[1]-v.c.Y)*v.scale
	// Parallax: about the middle of the map.
	x0, x1 = v.w/2+(x0-v.w/2)*k, v.w/2+(x1-v.w/2)*k
	y0, y1 = v.h/2+(y0-v.h/2)*k, v.h/2+(y1-v.h/2)*k
	r := geom.Rect{
		Min: geom.Pt(bounds.Min.X+float32(x0), bounds.Min.Y+float32(y0)),
		Max: geom.Pt(bounds.Min.X+float32(x1), bounds.Min.Y+float32(y1)),
	}
	ctx.Add(render.Op{Kind: render.OpImage, Bounds: r, Color: tint, Image: id})
}

// emitTriangles adds one triangle operation for the vertices, in map
// coordinates.
func emitTriangles(ctx *gift.PaintContext, vs []render.Vertex, bounds geom.Rect) {
	if len(vs) < 3 {
		return
	}
	first := ctx.VerticesLen()
	for _, v := range vs {
		ctx.AppendVertex(v)
	}
	ctx.Add(render.Op{Kind: render.OpTriangles, Bounds: geom.Rect{Max: geom.Pt(bounds.Width(), bounds.Height())},
		Glyphs: first, GlyphCount: uint32(len(vs))})
}

// windSpeed is how fast a streak moves on the screen per metre a second of
// wind: a breeze of 5 m/s crosses a map of 1000 points in about 25 s.
const windSpeed = 8.0

// moveWind moves the streaks of wind through the field and builds their
// triangles.
func (w *weatherOverlay) moveWind(p Projection, dt float64) {
	w.windVerts = w.windVerts[:0]
	b := w.cur
	if b == nil || b.wind == nil || w.o.Layers&WeatherWind == 0 {
		w.wind = w.wind[:0]
		return
	}
	ws, hs := p.Size()
	target := 1000
	if w.o.Quality == WeatherReduced {
		target = 400
	}
	target = int(float64(target) * ws * hs / (1920 * 1080))
	for len(w.wind) < target {
		w.wind = append(w.wind, windParticle{})
	}
	w.wind = w.wind[:target]
	for i := range w.wind {
		q := &w.wind[i]
		q.age += dt
		if q.n == 0 || q.age > q.life {
			w.spawnWind(q, p)
			continue
		}
		head := q.trail[q.n-1]
		u, v, ok := b.wind.SampleVector(head.Coordinate())
		if !ok {
			w.spawnWind(q, p)
			continue
		}
		// Screen points a second to world units a second.
		vx := float64(u) * windSpeed / p.Scale()
		vy := -float64(v) * windSpeed / p.Scale()
		next := geo.Point{X: head.X + vx*dt, Y: head.Y + vy*dt}
		q.sinceTrail += dt
		if q.sinceTrail >= 0.08 || q.n == 1 {
			q.sinceTrail = 0
			if q.n < len(q.trail) {
				q.n++
			} else {
				copy(q.trail[:], q.trail[1:])
			}
		}
		q.trail[q.n-1] = next
		x, y := p.ToScreen(next)
		if x < -20 || y < -20 || x > ws+20 || y > hs+20 {
			w.spawnWind(q, p)
			continue
		}
		speed := math.Hypot(float64(u), float64(v))
		fade := math.Min(1, math.Min(q.age/0.5, (q.life-q.age)/0.5))
		alpha := float32(math.Min(0.7, 0.18+speed*0.05) * fade)
		w.windVerts = streak(w.windVerts, p, q.trail[:q.n], 1.4, alpha)
	}
}

func (w *weatherOverlay) spawnWind(q *windParticle, p Projection) {
	ws, hs := p.Size()
	x, y := w.rng.Float64()*ws, w.rng.Float64()*hs
	q.trail[0] = p.ToWorld(x, y)
	q.n, q.age, q.life, q.sinceTrail = 1, 0, 2+3*w.rng.Float64(), 0
}

// streak appends the triangles of a polyline through the trail, width
// points wide, fading towards its tail.
func streak(vs []render.Vertex, p Projection, trail []geo.Point, width float64, alpha float32) []render.Vertex {
	if len(trail) < 2 {
		return vs
	}
	n := len(trail)
	for i := 0; i+1 < n; i++ {
		x0, y0 := p.ToScreen(trail[i])
		x1, y1 := p.ToScreen(trail[i+1])
		dx, dy := x1-x0, y1-y0
		l := math.Hypot(dx, dy)
		if l < 0.01 {
			continue
		}
		nx, ny := -dy/l*width/2, dx/l*width/2
		a0 := alpha * float32(i) / float32(n-1)
		a1 := alpha * float32(i+1) / float32(n-1)
		c0 := render.Color{R: a0, G: a0, B: a0, A: a0}
		c1 := render.Color{R: a1, G: a1, B: a1, A: a1}
		p0 := render.Vertex{X: float32(x0 + nx), Y: float32(y0 + ny), Color: c0}
		p1 := render.Vertex{X: float32(x0 - nx), Y: float32(y0 - ny), Color: c0}
		p2 := render.Vertex{X: float32(x1 + nx), Y: float32(y1 + ny), Color: c1}
		p3 := render.Vertex{X: float32(x1 - nx), Y: float32(y1 - ny), Color: c1}
		vs = append(vs, p0, p1, p2, p1, p3, p2)
	}
	return vs
}

// moveRain moves the drops and flakes, in the map's own coordinates: rain
// falls out of the sky and not along the ground.
func (w *weatherOverlay) moveRain(p Projection, dt float64) {
	w.rainVerts = w.rainVerts[:0]
	b := w.cur
	if b == nil || b.precip == nil || w.o.Layers&WeatherPrecipitation == 0 {
		w.rain = w.rain[:0]
		return
	}
	ws, hs := p.Size()
	target := 700
	if w.o.Quality == WeatherReduced {
		target = 280
	}
	target = int(float64(target) * ws * hs / (1920 * 1080))
	for len(w.rain) < target {
		w.rain = append(w.rain, rainParticle{})
	}
	w.rain = w.rain[:target]
	// The direction of the fall: the wind of the middle of the map, turned
	// with the map, and a little towards the bottom of the screen.
	fx, fy := 0.25, 1.0
	if b.wind != nil {
		if u, v, ok := b.wind.SampleVector(p.Center().Coordinate()); ok {
			s, c := math.Sincos(-p.Heading())
			ux, uy := float64(u), -float64(v)
			fx, fy = fx+0.08*(c*ux-s*uy), fy+0.08*(s*ux+c*uy)
		}
	}
	fl := math.Hypot(fx, fy)
	fx, fy = fx/fl, fy/fl
	for i := range w.rain {
		q := &w.rain[i]
		q.age += dt
		if q.life == 0 || q.age > q.life || q.y > hs+20 || q.x < -20 || q.x > ws+20 {
			q.x, q.y = w.rng.Float64()*ws, w.rng.Float64()*hs
			q.age, q.life = 0, 0.6+0.8*w.rng.Float64()
		}
		c := p.ToWorld(q.x, q.y).Coordinate()
		r, ok := b.precip.Sample(c)
		if !ok || r < 0.1 {
			continue
		}
		snow := false
		if b.temp != nil {
			if t, ok := b.temp.Sample(c); ok && t < 274.2 {
				snow = true
			}
		}
		inten := float32(math.Min(1, 0.3+math.Log1p(float64(r))/3))
		fade := float32(math.Min(1, math.Min(q.age/0.15, (q.life-q.age)/0.15)))
		if snow {
			q.x += (fx*40 + 12*math.Sin(q.age*5+float64(i))) * dt
			q.y += fy * 40 * dt
			a := 0.85 * inten * fade
			col := render.Color{R: a, G: a, B: a, A: a}
			s := 1.6
			w.rainVerts = append(w.rainVerts,
				render.Vertex{X: float32(q.x - s), Y: float32(q.y - s), Color: col},
				render.Vertex{X: float32(q.x + s), Y: float32(q.y - s), Color: col},
				render.Vertex{X: float32(q.x - s), Y: float32(q.y + s), Color: col},
				render.Vertex{X: float32(q.x + s), Y: float32(q.y - s), Color: col},
				render.Vertex{X: float32(q.x + s), Y: float32(q.y + s), Color: col},
				render.Vertex{X: float32(q.x - s), Y: float32(q.y + s), Color: col})
			continue
		}
		q.x += fx * 520 * dt
		q.y += fy * 520 * dt
		a := 0.5 * inten * fade
		head := render.Color{R: a * 0.78, G: a * 0.84, B: a * 0.95, A: a}
		tail := render.Color{}
		l, hw := 13.0, 0.7
		nx, ny := -fy*hw, fx*hw
		tx, ty := q.x-fx*l, q.y-fy*l
		w.rainVerts = append(w.rainVerts,
			render.Vertex{X: float32(tx + nx), Y: float32(ty + ny), Color: tail},
			render.Vertex{X: float32(tx - nx), Y: float32(ty - ny), Color: tail},
			render.Vertex{X: float32(q.x + nx), Y: float32(q.y + ny), Color: head},
			render.Vertex{X: float32(tx - nx), Y: float32(ty - ny), Color: tail},
			render.Vertex{X: float32(q.x - nx), Y: float32(q.y - ny), Color: head},
			render.Vertex{X: float32(q.x + nx), Y: float32(q.y + ny), Color: head})
	}
}
