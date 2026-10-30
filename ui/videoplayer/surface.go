package videoplayer

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/video"
	"github.com/worldiety/gift/video/ffmpeg"
)

var surfaceType = gift.RegisterType("videoplayer.Surface")

// Gravity is how a picture fills a view whose shape is not the picture's:
// AVLayerVideoGravity.
type Gravity uint8

const (
	// ResizeAspect shows the whole picture at its aspect ratio, with bars
	// where the view is wider or taller. The default.
	ResizeAspect Gravity = iota
	// ResizeAspectFill fills the view at the picture's aspect ratio and cuts
	// off what does not fit.
	ResizeAspectFill
	// Resize stretches the picture to the view.
	Resize
)

// SurfaceView shows the picture of a [video.Player] and nothing else: the
// SwiftUI VideoPlayer without its controls. It is created by [Surface].
//
// It is also what plays the sound. A surface attaches the player to the
// backend's audio device and asks it for the picture of the moment once per
// drawn frame, so a player that no surface shows does not advance; see
// [video.Player.Frame].
//
// Without a Frame, it fills what it is offered and takes the picture's aspect
// ratio along an unbounded axis, so a surface in a [ui.VStack] is as tall as
// its width asks for. Anamorphic pictures are shown at their display aspect
// ratio, 16:9 PAL as 16:9 and not as 5:4.
type SurfaceView struct {
	p          *video.Player
	gravity    Gravity
	radius     float32
	background ui.Color
	onTap      func()
	key        string
	label      string
}

// Surface returns a view of p's picture. p must not be nil.
func Surface(p *video.Player) SurfaceView {
	if p == nil {
		panic("gift/ui/videoplayer: Surface with a nil player")
	}
	return SurfaceView{p: p, background: ui.RGB(0, 0, 0)}
}

// Gravity sets how the picture fills the view; see [Gravity].
func (v SurfaceView) Gravity(g Gravity) SurfaceView { v.gravity = g; return v }

// CornerRadius rounds the view and the picture in it.
func (v SurfaceView) CornerRadius(r float32) SurfaceView { v.radius = r; return v }

// Background sets the colour of the bars around the picture. The default is
// black; [ui.ColorClear] shows what is behind.
func (v SurfaceView) Background(c ui.Color) SurfaceView { v.background = c; return v }

// OnTap calls fn when the picture is tapped or clicked without moving.
func (v SurfaceView) OnTap(fn func()) SurfaceView { v.onTap = fn; return v }

// Label sets the accessibility label.
func (v SurfaceView) Label(s string) SurfaceView { v.label = s; return v }

// Key sets the identity among siblings.
func (v SurfaceView) Key(s string) SurfaceView { v.key = s; return v }

// ViewType implements gift.View.
func (v SurfaceView) ViewType() gift.TypeID { return surfaceType }

// Build implements gift.View.
func (v SurfaceView) Build(*gift.BuildContext) gift.Element {
	n := &surfaceNode{p: v.p, gravity: v.gravity, radius: v.radius,
		background: ui.ResolveColor(v.background), onTap: v.onTap}
	e := gift.Element{Key: v.key, Label: v.label, Layouter: n, Painter: n}
	if v.onTap != nil {
		e.Interactor = n
	}
	return e
}

// surfaceNode is the retained half of a [SurfaceView].
type surfaceNode struct {
	p          *video.Player
	gravity    Gravity
	radius     float32
	background ui.Color
	onTap      func()
}

// Layout fills a bounded box and derives an unbounded axis from the display
// aspect ratio of the picture.
func (n *surfaceNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	a := float32(n.p.Info().DisplayAspect())
	if !(a > 0) {
		a = 16.0 / 9
	}
	w, h := c.Max.W, c.Max.H
	switch fw, fh := c.HasBoundedWidth(), c.HasBoundedHeight(); {
	case fw && !fh:
		h = w / a
	case fh && !fw:
		w = h * a
	case !fw && !fh:
		w, h = 640, 640/a
	}
	ctx.ReportOverflow(geom.Size{})
	return c.Constrain(geom.Sz(w, h))
}

// Paint draws the bars, advances the player and draws its picture.
func (n *surfaceNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	p := n.p
	if out := ctx.Audio(); out != nil {
		p.AttachAudio(out)
	}
	f, _ := p.Frame(time.Now())
	if vids := ctx.Videos(); vids != nil && f != nil {
		st := surfaces.state(p, vids)
		field := fieldOf(p, f)
		if f != st.frame || f.PTS != st.pts || field != st.field || !st.live(vids) {
			vf := render.VideoFrame{
				Pix: f.Pix, Stride: f.Stride, Width: f.Width, Height: f.Height,
				Layout: render.VideoLayout(f.Layout), Matrix: render.VideoMatrix(f.Matrix),
				FullRange: f.FullRange, Field: field,
			}
			// Only the field changed: the pixels are on the GPU already.
			if f == st.frame && f.PTS == st.pts && st.live(vids) {
				vf.Pix = nil
			}
			st.handle, _ = vids.Present(st.handle, vf)
			st.frame, st.pts, st.field = f, f.PTS, field
		}
		if id, ok := vids.Resolve(st.handle); ok {
			n.paintPicture(ctx, b, f, id)
		}
	} else {
		n.paintBars(ctx, b, geom.Rect{})
	}
	if p.NeedsFrames() {
		ctx.Animate(250 * time.Millisecond)
	}
}

// paintPicture emits the video operation for the picture placed in b.
func (n *surfaceNode) paintPicture(ctx *gift.PaintContext, b geom.Rect, f *ffmpeg.Frame, id render.ImageID) {
	op := render.Op{Kind: render.OpVideo, Color: ui.OpaqueWhite, CornerRadius: n.radius, Image: id}
	info := n.p.Info()
	a := float32(info.DisplayAspect())
	if !(a > 0) {
		a = float32(f.Width) / float32(f.Height)
	}
	square := info.SARNum == info.SARDen || info.SARNum <= 0 || info.SARDen <= 0
	switch n.gravity {
	case Resize:
		op.Bounds = b
	case ResizeAspectFill:
		if square {
			// The backend crops symmetrically and keeps the corners.
			op.Bounds, op.Fit = b, render.ImageCover
			break
		}
		// Not square pixels: the crop of ImageCover would follow the
		// texture's shape, so the picture is drawn larger and clipped,
		// which loses the rounded corners.
		op.Bounds, op.CornerRadius = fill(b, a), 0
		ctx.PushClip(b)
		ctx.Add(op)
		ctx.PopClip()
		return
	default:
		op.Bounds = fit(b, a)
		if op.Bounds != b {
			// Letterboxed: the bars carry the corners, the picture is
			// square inside them unless it touches them.
			op.CornerRadius = 0
		}
	}
	n.paintBars(ctx, b, op.Bounds)
	ctx.Add(op)
}

// paintBars fills the part of b that the picture pic does not cover.
//
// Only that part, and not all of b under the picture: on a Raspberry Pi every
// full screen fill is a measurable share of a frame, and a fill the picture
// covers completely is one nobody sees. With rounded corners the bars are one
// rounded fill under the picture, because four rectangles cannot carry the
// corners; that costs the pass, for a view that chose to look like a card.
func (n *surfaceNode) paintBars(ctx *gift.PaintContext, b, pic geom.Rect) {
	if n.background.IsTransparent() {
		return
	}
	if n.radius > 0 && pic != b {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: n.background, CornerRadius: n.radius})
		return
	}
	pic = pic.Intersect(b)
	if pic.IsEmpty() {
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: n.background})
		return
	}
	for _, r := range [4]geom.Rect{
		geom.Rc(b.Min.X, b.Min.Y, b.Max.X, pic.Min.Y),
		geom.Rc(b.Min.X, pic.Max.Y, b.Max.X, b.Max.Y),
		geom.Rc(b.Min.X, pic.Min.Y, pic.Min.X, pic.Max.Y),
		geom.Rc(pic.Max.X, pic.Min.Y, b.Max.X, pic.Max.Y),
	} {
		if !r.IsEmpty() {
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: r, Color: n.background})
		}
	}
}

// fit is the largest rectangle of aspect a centred in b, snapped to whole
// logical pixels so that a picture shown at its own size stays one texel per
// pixel.
func fit(b geom.Rect, a float32) geom.Rect {
	w, h := b.Width(), b.Height()
	if w/h > a {
		w = h * a
	} else {
		h = w / a
	}
	x := b.Min.X + float32(math.Round(float64((b.Width()-w)/2)))
	y := b.Min.Y + float32(math.Round(float64((b.Height()-h)/2)))
	return geom.Rc(x, y, x+w, y+h)
}

// fill is the smallest rectangle of aspect a centred on b that covers it.
func fill(b geom.Rect, a float32) geom.Rect {
	w, h := b.Width(), b.Height()
	if w/h > a {
		h = w / a
	} else {
		w = h * a
	}
	cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	return geom.Rc(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}

// fieldOf is the field of an interlaced picture to show now: the first in its
// first half, the second after.
func fieldOf(p *video.Player, f *ffmpeg.Frame) render.VideoField {
	if !f.Interlaced {
		return render.FieldProgressive
	}
	second := p.CurrentTime() >= f.PTS+f.Duration/2
	if f.TopFieldFirst != second {
		return render.FieldTop
	}
	return render.FieldBottom
}

// HandleEvent implements gift.Interactor: a press and a release without a
// drag in between is a tap.
func (n *surfaceNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.ControlState()
	switch e.Kind {
	case gift.EventPointerDown:
		st.Grabbed, st.At = true, e.Pos
		ctx.SetControlState(st)
		return true
	case gift.EventPointerMove:
		if !st.Grabbed {
			return false
		}
		// Ten logical pixels of slack, as a finger wobbles.
		slop := 10 * deviceScale(ctx)
		if d := e.Pos.Sub(st.At); d.X*d.X+d.Y*d.Y > slop*slop {
			st.Grabbed = false
			ctx.SetControlState(st)
		}
		return true
	case gift.EventPointerUp:
		if !st.Grabbed {
			return false
		}
		st.Grabbed = false
		ctx.SetControlState(st)
		n.onTap()
		return true
	case gift.EventPointerCancel:
		st.Grabbed = false
		ctx.SetControlState(st)
		return true
	}
	return false
}

// deviceScale is the number of device pixels per logical pixel of the node.
func deviceScale(ctx *gift.EventContext) float32 {
	if w := ctx.Bounds().Width(); w > 0 {
		return ctx.DeviceBounds().Width() / w
	}
	return 1
}

// surfaceState is what a surface keeps of a player across builds: the video
// resource and which picture it holds. It lives here and not in the node,
// which is rebuilt with every build of the tree around it.
type surfaceState struct {
	handle render.ImageHandle
	frame  *ffmpeg.Frame
	pts    time.Duration
	field  render.VideoField
}

func (s *surfaceState) live(v render.Videos) bool {
	_, ok := v.Resolve(s.handle)
	return ok
}

// registry maps players to their state. It is touched by painters only, on
// the UI goroutine.
type registry struct {
	m map[*video.Player]*surfaceState
}

var surfaces = registry{m: map[*video.Player]*surfaceState{}}

// state returns the state of p, and releases the resources of players that
// were closed meanwhile.
func (r *registry) state(p *video.Player, v render.Videos) *surfaceState {
	st := r.m[p]
	if st == nil {
		for q, s := range r.m {
			if q.Closed() {
				v.Release(s.handle)
				delete(r.m, q)
			}
		}
		st = &surfaceState{}
		r.m[p] = st
	}
	return st
}
