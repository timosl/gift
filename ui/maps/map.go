package maps

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/ui"
)

// Config configures a [Map].
type Config struct {
	// Tiles is the provider of the map's tiles; see [Provider]. Required.
	Tiles Provider
	// Style draws vector tiles. It is required for a vector provider and must
	// be written for its schema; a raster provider ignores it.
	Style *Style
	// Camera is where the map starts. The zero value shows the world.
	Camera Camera
	// MinZoom and MaxZoom limit the zoom. Zero selects the provider's
	// minimum and, for the maximum, three levels past a raster provider's
	// last, or 20 for vector tiles, which stay sharp at any zoom.
	MinZoom, MaxZoom float64
	// Rotation allows turning the map with two fingers. A vector map keeps
	// its labels upright at any heading; a raster map turns the lettering
	// baked into its tiles, and for it the default is off.
	Rotation RotationMode
}

// RotationMode says whether a map can be turned.
type RotationMode uint8

const (
	// RotationAuto allows rotation for vector tiles and not for raster
	// tiles, whose lettering would turn upside down.
	RotationAuto RotationMode = iota
	RotationOn
	RotationOff
)

// Map is one map: its provider, its style and its camera. It is long lived
// and owned by the application, and [View] is the short lived declaration of
// it in a build, the way a [ui.Gallery] and [ui.ImageGallery] divide the
// work. The camera survives rebuilds because it lives here.
//
// A Map belongs to the UI goroutine. Its methods are called from builds and
// event handlers, never from other goroutines.
type Map struct {
	cfg   Config
	info  ProviderInfo
	style *Style
	mv    mover

	minZ, maxZ float64
	rotation   bool

	// now is the time of the last layout, for a camera change made outside
	// an event; invalidate asks for the next one.
	now        time.Duration
	invalidate func()
	pending    *pendingCamera

	tiles tileCache
	frame frame
	stats Stats

	onCamera func(Camera)
}

// pendingCamera is a camera set before the map knew its size.
type pendingCamera struct {
	cam      Camera
	region   *geo.Rect
	animated bool
}

// New returns a map over cfg.Tiles. It fails for a vector provider without a
// style or with a style for another schema: a mismatch would show an empty
// map, and an error now is easier to find than a blank screen later.
func New(cfg Config) (*Map, error) {
	if cfg.Tiles == nil {
		return nil, errors.New("maps: Config.Tiles is nil")
	}
	info := cfg.Tiles.Info()
	if info.Bounds.IsEmpty() {
		info.Bounds = geo.World
	}
	if info.Kind == Vector {
		if cfg.Style == nil {
			return nil, fmt.Errorf("maps: the vector provider %q needs a Style", info.Name)
		}
		if cfg.Style.Schema != info.Schema {
			return nil, fmt.Errorf("maps: the style is for the schema %q and the provider %q has %q",
				cfg.Style.Schema, info.Name, info.Schema)
		}
	}
	m := &Map{cfg: cfg, info: info, style: cfg.Style, mv: newMover()}
	m.minZ = math.Max(cfg.MinZoom, float64(info.MinZoom))
	m.maxZ = cfg.MaxZoom
	if m.maxZ <= 0 {
		if info.Kind == Vector {
			m.maxZ = 20
		} else {
			m.maxZ = float64(info.MaxZoom) + 3
		}
	}
	switch cfg.Rotation {
	case RotationOn:
		m.rotation = true
	case RotationAuto:
		m.rotation = info.Kind == Vector
	}
	m.tiles.init()
	cam := cfg.Camera
	if cam == (Camera{}) {
		cam = Camera{Center: info.Bounds.Center(), Zoom: m.minZ}
	}
	m.pending = &pendingCamera{cam: cam}
	return m, nil
}

// Info returns what the provider says about itself.
func (m *Map) Info() ProviderInfo { return m.info }

// Camera returns the camera as of the last frame, in motion or not.
func (m *Map) Camera() Camera {
	if m.pending != nil && m.pending.region == nil {
		return m.pending.cam
	}
	c, z, hd, _ := m.mv.at(m.now)
	return Camera{Center: c.Coordinate(), Zoom: z, Heading: degrees(hd)}
}

// SetCamera moves the map to c, along the flight path of van Wijk and Nuij
// when animated: out, across and in again, as MapKit and Mapbox fly.
func (m *Map) SetCamera(c Camera, animated bool) {
	if m.mv.w <= 0 {
		m.pending = &pendingCamera{cam: c, animated: animated}
		m.request()
		return
	}
	if !m.rotation {
		c.Heading = 0
	}
	m.mv.flyTo(m.now, geo.Project(c.Center), c.Zoom, radians(c.Heading), animated)
	m.request()
}

// ShowRect moves the map so that r fills it, at the current heading.
func (m *Map) ShowRect(r geo.Rect, animated bool) {
	if m.mv.w <= 0 {
		m.pending = &pendingCamera{region: &r, animated: animated}
		m.request()
		return
	}
	m.SetCamera(m.fit(r), animated)
}

// fit is the camera that shows r whole, with a margin of a tenth.
func (m *Map) fit(r geo.Rect) Camera {
	nw, se := r.Projected()
	ww, wh := math.Abs(se.X-nw.X), math.Abs(se.Y-nw.Y)
	z := m.maxZ
	if ww > 0 || wh > 0 {
		zx := math.Log2(m.mv.w * 0.9 / (math.Max(ww, 1e-12) * TileSize))
		zy := math.Log2(m.mv.h * 0.9 / (math.Max(wh, 1e-12) * TileSize))
		z = math.Min(zx, zy)
	}
	c := geo.Point{X: (nw.X + se.X) / 2, Y: (nw.Y + se.Y) / 2}
	_, _, hd, _ := m.mv.at(m.now)
	return Camera{Center: c.Coordinate(), Zoom: z, Heading: degrees(hd)}
}

// OnCameraChange sets the function called when the camera comes to rest
// after a gesture or an animation, as MapKit's regionDidChangeAnimated: not
// on every frame of the motion.
func (m *Map) OnCameraChange(fn func(Camera)) { m.onCamera = fn }

// Stats is what the last frame drew, for tests and metrics.
type Stats struct {
	// Level is the tile zoom level shown.
	Level int
	// Tiles is how many tiles of that level were visible, Drawn how many of
	// them were drawn and Fallbacks how many coarser or finer tiles stood in
	// for the rest.
	Tiles, Drawn, Fallbacks int
	// Points and Labels are the markers and labels drawn.
	Points, Labels int
}

// Stats returns what the last frame drew.
func (m *Map) Stats() Stats { return m.stats }

func (m *Map) request() {
	if m.invalidate != nil {
		m.invalidate()
	}
}

// background is the colour behind the tiles, where none has arrived yet.
func (m *Map) background() ui.Color {
	if m.style != nil {
		return m.style.background()
	}
	return ui.RGB(232, 234, 237)
}

func degrees(r float64) float64 { return r * 180 / math.Pi }
func radians(d float64) float64 { return d * math.Pi / 180 }
