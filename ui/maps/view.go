package maps

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/gesture"
)

var viewType = gift.RegisterType("maps.View")

// MapView is the declaration of a [Map] in a build. It is created by [View].
type MapView struct {
	m          *Map
	key, label string
	flex       float32
	w, h       float32
	onTap      func(geo.Coordinate)
	onSelect   func(Point)
	onCluster  func(Cluster)
	points     []pointLayer
	overlays   []Overlay
}

// View returns the view of m.
//
// The map fills what it is given. It pans under one finger or the mouse,
// glides on when flicked, zooms under two fingers about their centre and
// turns under them when rotation is on; a double tap zooms in about the tap.
// The mouse wheel zooms about the cursor and a trackpad pans; with control
// or command held a trackpad zooms. Arrow keys pan, plus and minus zoom. The
// provider's attribution is always shown in the corner, and a compass when
// the map is turned; a tap on it turns the map back to north.
func View(m *Map) MapView { return MapView{m: m} }

// Frame fixes the size of the map.
func (v MapView) Frame(w, h float32) MapView { v.w, v.h = w, h; return v }

// Flex sets the flex factor in a stack.
func (v MapView) Flex(f float32) MapView { v.flex = f; return v }

// Key sets the reconciliation key.
func (v MapView) Key(s string) MapView { v.key = s; return v }

// Label sets the accessible name.
func (v MapView) Label(s string) MapView { v.label = s; return v }

// OnTap sets the function called with the coordinate of a single tap on the
// map itself, not on a marker.
func (v MapView) OnTap(fn func(geo.Coordinate)) MapView { v.onTap = fn; return v }

// Overlay adds o above the tiles and below the markers, in the order the
// overlays were added.
func (v MapView) Overlay(o Overlay) MapView {
	v.overlays = append(v.overlays[:len(v.overlays):len(v.overlays)], o)
	return v
}

// ViewType implements gift.View.
func (MapView) ViewType() gift.TypeID { return viewType }

// Build implements gift.View.
func (v MapView) Build(bc *gift.BuildContext) gift.Element {
	m := v.m
	n := &node{m: m, v: v, f: &m.frame}
	n.gs = []gesture.Gesture{
		gesture.Magnify().OnChanged(n.onMagnify).OnEnded(n.onMagnify).OnCancelled(n.onCancel),
		gesture.Rotate().OnChanged(n.onRotate).OnEnded(n.onRotate).OnCancelled(n.onCancel),
		gesture.Drag().OnChanged(n.onDrag).OnEnded(n.onDragEnd).OnCancelled(n.onCancel),
		gesture.Tap().Count(2).OnEnded(n.onDoubleTap),
		gesture.Tap().OnEnded(n.onTap),
		gesture.Wheel().OnChanged(n.onWheel),
	}
	label := v.label
	if label == "" {
		label = "Map"
	}
	// The gestures and the keys are handled by a child that covers the map,
	// and the map keeps its own node to itself: a node has one timer, and
	// the map's — the next frame of a motion, the retry of a tile — would
	// replace the one a tap waits on to rule out a double tap.
	kids := []gift.View{surface{n: n, label: label}, attribution(m.info.Attribution)}
	return gift.Element{
		Key:        v.key,
		Label:      label,
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Clip:       true,
		Flex:       v.flex,
		Children:   kids,
	}
}

var surfaceType = gift.RegisterType("maps.surface")

// surface is the input of a map: a child as large as the map that takes
// its gestures and its keys, with "input" for its key.
type surface struct {
	n     *node
	label string
}

func (surface) ViewType() gift.TypeID { return surfaceType }

func (s surface) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: "input", Label: s.label, Layouter: s, Interactor: s, Focusable: true, MultiTouch: true}
}

func (s surface) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size { return c.Max }

func (s surface) HandleEvent(ctx *gift.EventContext, e gift.Event) bool { return s.n.input(ctx, e) }

// attribution is the notice of the provider, small, in the bottom right
// corner, on a translucent plate so it stays legible over any tile.
func attribution(s string) gift.View {
	return ui.Text(s).FontSize(10).Foreground(ui.RGB(40, 44, 52)).
		PaddingInsets(geom.Insets{Left: 5, Right: 5, Top: 2, Bottom: 2}).
		Background(ui.RGBA(255, 255, 255, 190)).CornerRadius(4).Key("attribution")
}

// frame is what one frame of a map computed in layout for its paint. It lives
// in the [Map], not the node, so that its slices are reused across builds.
type frame struct {
	proj   projection
	refs   []tileRef
	coarse []srcKey
	moving bool
	rested bool
	// labels
	labels []placed
	cands  []labelCand
	ltiles []TileID
	boxes  []geom.Rect
	seen   map[string]bool
	fading bool
	// more is set when labels were left unmeasured for a later frame.
	more bool
	// markers are the markers of the frame, hits the scratch of their
	// queries, and pointsBusy is set while an index is being built or a
	// level change is animated.
	markers    []marker
	hits       []int32
	pointsBusy bool
	// loading is set while a visible tile or picture has not arrived.
	loading bool
	// markerLayers is how many drawing layers the markers need.
	markerLayers int32
	// chrome is what the map draws over its labels.
	chrome []geom.Rect
	// grid is the collision grid of the boxes, cols by rows cells.
	grid       [][]int32
	cols, rows int
	// candKey and candTiles are what the sorted candidates were collected
	// for.
	candKey   candKey
	candTiles []TileID
	// gesture bookkeeping: the totals of the last update of each gesture.
	mag, rot           float64
	anchor             geom.Point
	trans              geom.Point
	pinching, rotating bool
	panning            bool
}

type node struct {
	m  *Map
	v  MapView
	f  *frame
	gs []gesture.Gesture
	// now and ctx are the time and context of the event being handled, for
	// the gesture callbacks.
	now time.Duration
	ctx *gift.EventContext
}

func (n *node) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	m := n.m
	size := c.Max
	if n.v.w > 0 && n.v.h > 0 {
		size = geom.Sz(n.v.w, n.v.h)
	}
	if !c.HasBoundedWidth() && n.v.w <= 0 {
		size.W = 320
	}
	if !c.HasBoundedHeight() && n.v.h <= 0 {
		size.H = 240
	}
	size = c.Constrain(size)
	m.now = ctx.Now()
	m.invalidate = ctx.Invalidator()
	m.mv.setViewport(float64(size.W), float64(size.H), m.minZ, m.maxZ)
	if p := m.pending; p != nil && size.W > 0 && size.H > 0 {
		m.pending = nil
		cam := p.cam
		if p.region != nil {
			cam = m.fit(*p.region)
		}
		if !m.rotation {
			cam.Heading = 0
		}
		m.mv.flyTo(m.now, geo.Project(cam.Center), cam.Zoom, radians(cam.Heading), p.animated)
	}
	cc, z, hd, moving := m.mv.at(m.now)
	v := newProjection(cc, z, hd, float64(size.W), float64(size.H))
	n.f.proj = v
	n.layoutTiles(ctx, v, ctx.Pass())
	// The surface over all of it, the attribution bottom right.
	ctx.Measure(0, geom.Tight(size))
	ctx.Place(0, geom.Point{})
	as := ctx.Measure(1, geom.Loose(size))
	at := geom.Pt(size.W-as.W-4, size.H-as.H-4)
	ctx.Place(1, at)
	n.f.chrome = append(n.f.chrome[:0], geom.Rect{Min: at, Max: at.Add(geom.Pt(as.W, as.H))})
	if hd != 0 {
		n.f.chrome = append(n.f.chrome, compassRect(geom.Rect{Max: geom.Pt(size.W, size.H)}))
	}

	n.layoutOverlays(ctx, v)
	n.layoutPoints(ctx, v)
	n.layoutLabels(ctx, v, ctx.Pass())

	// A camera in motion is laid out again in the next frame, and the
	// trigger is a timer rather than a request from here: a timer fires only
	// when the clock has moved, so a frame at the same time, which would
	// compute the same camera, lays out nothing and the application can
	// settle. A camera that comes to rest is reported from the same timer,
	// because a layout may not run code that writes state.
	switch {
	case moving || m.mv.gesturing:
		n.f.moving, n.f.rested = true, false
	case n.f.moving:
		n.f.moving, n.f.rested = false, true
	}
	switch {
	case moving || n.f.rested || n.f.fading || n.f.pointsBusy:
		ctx.After(time.Millisecond)
	case n.f.loading:
		// A picture the pipeline turned away is asked for again only by
		// a layout; while anything visible is missing, there is one ten
		// times a second.
		ctx.After(100 * time.Millisecond)
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

func (n *node) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: n.m.background()})
	v := n.f.proj
	ctx.PushClip(ctx.DeviceBounds())
	turned := v.heading != 0
	if turned {
		mid := geom.Pt(b.Min.X+b.Width()/2, b.Min.Y+b.Height()/2)
		ctx.PushTransform(geom.Translate(geom.Pt(-mid.X, -mid.Y)).
			Mul(geom.Rotate(float32(-v.heading))).Mul(geom.Translate(mid)))
	}
	n.paintTiles(ctx, b.Min)
	if turned {
		ctx.PopTransform()
	}
	n.paintOverlays(ctx, b)
	n.paintLabels(ctx, b.Min)
	n.paintMarkers(ctx, b.Min)
	ctx.PaintChildren()
	if turned {
		n.paintCompass(ctx, b, v.heading)
	}
	ctx.PopClip()
}

// The compass, top right while the map is turned.
const (
	compassSize   = 36
	compassMargin = 10
)

func compassRect(b geom.Rect) geom.Rect {
	return geom.Rect{
		Min: geom.Pt(b.Max.X-compassMargin-compassSize, b.Min.Y+compassMargin),
		Max: geom.Pt(b.Max.X-compassMargin, b.Min.Y+compassMargin+compassSize),
	}
}

// paintCompass draws a disc with a needle that points north: red to the
// north, grey to the south.
func (n *node) paintCompass(ctx *gift.PaintContext, b geom.Rect, heading float64) {
	r := compassRect(b)
	disc, south := ui.RGBA(255, 255, 255, 235), ui.RGB(150, 155, 165)
	if n.m.styleIndex() == 1 {
		disc, south = ui.RGBA(48, 50, 56, 235), ui.RGB(120, 124, 134)
	}
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: r, CornerRadius: compassSize / 2,
		Color: ui.RGBA(0, 0, 0, 60), Blur: 6})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: r, CornerRadius: compassSize / 2,
		Color: disc})
	mid := geom.Pt(r.Min.X+compassSize/2, r.Min.Y+compassSize/2)
	ctx.PushTransform(geom.Translate(geom.Pt(-mid.X, -mid.Y)).
		Mul(geom.Rotate(float32(-heading))).Mul(geom.Translate(mid)))
	const nw, nl = 5, 12
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, CornerRadius: nw / 2, Color: ui.RGB(230, 60, 50),
		Bounds: geom.Rect{Min: geom.Pt(mid.X-nw/2, mid.Y-nl), Max: geom.Pt(mid.X+nw/2, mid.Y)}})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, CornerRadius: nw / 2, Color: south,
		Bounds: geom.Rect{Min: geom.Pt(mid.X-nw/2, mid.Y), Max: geom.Pt(mid.X+nw/2, mid.Y+nl)}})
	ctx.PopTransform()
}

// HandleEvent takes the map's own timer: the next frame of a motion, of a
// transition or of a fade, the retry of what has not arrived, and the camera
// that came to rest.
func (n *node) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventTimer {
		return false
	}
	n.now = ctx.Now()
	used := false
	if n.f.moving || n.f.fading || n.f.pointsBusy || n.f.loading {
		ctx.RequestLayout()
		used = true
	}
	if n.f.rested && !n.f.moving {
		n.f.rested = false
		used = true
		if n.m.onCamera != nil {
			n.m.onCamera(n.m.Camera())
		}
	}
	return used
}

// input handles the events of the surface: the keys and the gestures.
func (n *node) input(ctx *gift.EventContext, e gift.Event) bool {
	n.now, n.ctx = ctx.Now(), ctx
	defer func() { n.ctx = nil }()
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
	case gift.EventKeyDown:
		if n.key(e) {
			return true
		}
	case gift.EventRune:
		if n.rune(e.Rune) {
			return true
		}
	}
	return gesture.Handle(ctx, e, n.gs)
}

// moved asks for the frames a camera change needs: a layout of the map,
// not of the surface the event arrived at.
func (n *node) moved() { n.m.request() }

func (n *node) begin() {
	if !n.m.mv.gesturing {
		n.m.mv.begin(n.now)
	}
}

func (n *node) onMagnify(v gesture.MagnifyValue) {
	f := n.f
	at := v.Common.Local(v.Location)
	switch v.Phase {
	case gesture.Began:
		n.begin()
		f.pinching, f.mag, f.anchor = true, 1, v.Common.Local(v.StartAnchor)
		fallthrough
	case gesture.Changed:
		n.m.mv.pinch(float64(f.anchor.X), float64(f.anchor.Y), float64(at.X), float64(at.Y), v.Magnification/f.mag)
		f.mag, f.anchor = v.Magnification, at
		n.moved()
	case gesture.Ended:
		f.pinching = false
		n.release(0, 0, at)
	}
}

func (n *node) onRotate(v gesture.RotateValue) {
	if !n.m.rotation {
		return
	}
	f := n.f
	at := v.Common.Local(v.Location)
	switch v.Phase {
	case gesture.Began:
		n.begin()
		f.rotating, f.rot = true, 0
		fallthrough
	case gesture.Changed:
		n.m.mv.rotate(float64(at.X), float64(at.Y), v.Rotation-f.rot)
		f.rot = v.Rotation
		n.moved()
	case gesture.Ended:
		f.rotating = false
		n.release(0, 0, at)
	}
}

func (n *node) onDrag(v gesture.DragValue) {
	f := n.f
	if v.Phase == gesture.Began {
		n.begin()
		f.panning, f.trans = true, v.Translation
		return
	}
	d := v.Translation.Sub(f.trans)
	f.trans = v.Translation
	if f.pinching || f.rotating {
		// The pinch and the rotation move with the centroid already.
		return
	}
	n.m.mv.pan(float64(d.X), float64(d.Y))
	n.moved()
}

func (n *node) onDragEnd(v gesture.DragValue) {
	f := n.f
	f.panning = false
	vx, vy := float64(v.Velocity.X), float64(v.Velocity.Y)
	if f.pinching || f.rotating {
		return
	}
	n.release(vx, vy, v.Common.Local(v.Location))
}

func (n *node) onCancel() {
	f := n.f
	f.pinching, f.rotating, f.panning = false, false, false
	if n.m.mv.gesturing {
		n.m.mv.end(n.now, 0, 0, n.m.mv.w/2, n.m.mv.h/2)
		n.moved()
	}
}

// release ends the gesture once the last of its recognisers has.
func (n *node) release(vx, vy float64, at geom.Point) {
	f := n.f
	if f.pinching || f.rotating || f.panning || !n.m.mv.gesturing {
		return
	}
	n.m.mv.end(n.now, vx, vy, float64(at.X), float64(at.Y))
	n.moved()
}

func (n *node) onDoubleTap(v gesture.TapValue) {
	at := v.Common.Local(v.Location)
	mv := &n.m.mv
	_, z, hd, _ := mv.at(n.now)
	mv.zoomAbout(n.now, math.Round(z)+1, hd, float64(at.X), float64(at.Y), 350*time.Millisecond)
	n.moved()
}

func (n *node) onTap(v gesture.TapValue) {
	at := v.Common.Local(v.Location)
	if k, ok := n.hitMarker(at); ok {
		n.tapMarker(k)
		return
	}
	b := geom.Rect{Max: geom.Pt(float32(n.m.mv.w), float32(n.m.mv.h))}
	if n.f.proj.heading != 0 && compassRect(b).Contains(at) {
		mv := &n.m.mv
		_, z, _, _ := mv.at(n.now)
		mv.zoomAbout(n.now, z, 0, mv.w/2, mv.h/2, 400*time.Millisecond)
		n.moved()
		return
	}
	if n.v.onTap != nil {
		n.v.onTap(n.f.proj.toWorld(float64(at.X), float64(at.Y)).Coordinate())
	}
}

// wheelNotch is the zoom of one notch of a mouse wheel, half a level.
const wheelNotch = 0.5

func (n *node) onWheel(v gesture.WheelValue) bool {
	mv := &n.m.mv
	at := v.Common.Local(v.Location)
	zoomMod := v.Mods.Has(gift.ModControl) || v.Mods.Has(gift.ModMeta)
	switch {
	case v.Precise && !zoomMod:
		// A trackpad pans, the content following the fingers; one unit
		// is ten points, as for the scroll containers.
		mv.settle(n.now)
		mv.pan(float64(v.Delta.X*gift.DefaultScrollPreciseWheelStep), float64(v.Delta.Y*gift.DefaultScrollPreciseWheelStep))
	case v.Precise:
		mv.settle(n.now)
		mv.begin(n.now)
		mv.pinch(float64(at.X), float64(at.Y), float64(at.X), float64(at.Y), math.Exp2(float64(v.Delta.Y)*0.05))
		mv.end(n.now, 0, 0, float64(at.X), float64(at.Y))
	default:
		if v.Delta.Y == 0 {
			return false
		}
		_, z, hd, _ := mv.at(n.now)
		target := z + wheelNotch*float64(v.Delta.Y)
		if mv.mode == zoomingAbout {
			target = mv.toZ + wheelNotch*float64(v.Delta.Y)
		}
		mv.zoomAbout(n.now, target, hd, float64(at.X), float64(at.Y), 200*time.Millisecond)
	}
	n.moved()
	return true
}

// keyStep is how far an arrow key pans, in logical pixels.
const keyStep = 80

func (n *node) key(e gift.Event) bool {
	var dx, dy float64
	switch e.Key {
	case gift.KeyLeft:
		dx = keyStep
	case gift.KeyRight:
		dx = -keyStep
	case gift.KeyUp:
		dy = keyStep
	case gift.KeyDown:
		dy = -keyStep
	default:
		return false
	}
	mv := &n.m.mv
	c, z, hd, _ := mv.at(n.now)
	v := newProjection(c, z, hd, mv.w, mv.h)
	to := c.Sub(v.worldVector(dx, dy))
	mv.flyTo(n.now, to, z, hd, true)
	n.moved()
	return true
}

// rune handles the zoom keys: plus and minus zoom by a level about the
// middle.
func (n *node) rune(r rune) bool {
	mv := &n.m.mv
	_, z, hd, _ := mv.at(n.now)
	switch r {
	case '+', '=':
		z = math.Round(z) + 1
	case '-', '_':
		z = math.Round(z) - 1
	default:
		return false
	}
	mv.zoomAbout(n.now, z, hd, mv.w/2, mv.h/2, 300*time.Millisecond)
	n.moved()
	return true
}
