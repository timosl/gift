package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// Picture is a handle on one picture at one size in the shared image service
// of [SetImagePipeline]: the request, the decoded pixels and the texture.
//
// It is the low level half of [Image], for a view that draws pictures
// itself — at a rectangle it computes, at several sizes at once, under a
// transform of its own — and still wants the one pipeline, the one disk cache
// and the one table of textures every [Image] and [Gallery] shares. A zoomable
// picture is the first such view: it shows a small rung at once, the rung that
// fills the screen when it has arrived, and a larger one when it is zoomed
// into, all from the same source; see package ui/imageviewer.
//
// A Picture is a value and cheap to copy. It is valid for the frame it was
// obtained in, like the layout it was obtained from.
type Picture struct {
	r *imageRequest
}

// RequestPicture asks the shared image service for src at size pixels along
// its longest edge, and returns the handle. It is called from a layouter, for
// the reason [Image] requests from Layout: scheduling is update work.
//
// The size is in device pixels and goes through the pipeline's ladder; see
// [asset.Config.Sizes]. A picture that is already in the pipeline's CPU cache
// at a suitable rung is ready immediately, in this call, which is what lets a
// view opened from a gallery show the gallery's thumbnail in its very first
// frame. Anything else is requested, at [asset.Visible] priority unless the
// node is off screen, and the node is laid out again when the answer arrives.
// A retryable refusal is asked again on a later pass, at most once per pass;
// a permanent failure settles, as for [Image].
func RequestPicture(ctx *gift.LayoutContext, src asset.Source, size int) Picture {
	prio := asset.Visible
	if ctx.OffScreen() {
		prio = asset.Prefetch
	}
	return requestPicture(src, size, ctx.Pass(), prio, ctx.Invalidator())
}

// PrefetchPicture is [RequestPicture] at [asset.Prefetch] priority and
// without a layout when the answer arrives: for the neighbours of what is on
// screen, which should be ready when they are needed and must not delay what
// is needed now.
func PrefetchPicture(ctx *gift.LayoutContext, src asset.Source, size int) Picture {
	return requestPicture(src, size, ctx.Pass(), asset.Prefetch, nil)
}

func requestPicture(src asset.Source, size int, pass uint64, prio asset.Priority, notify func()) Picture {
	if src == nil || size <= 0 {
		return Picture{}
	}
	id := src.Metadata().ID
	r := images.state(id, size)
	if !r.ok && !r.inFlight && r.failure == nil && images.pipe != nil {
		// The warm path of [Gallery.requestImage]: already decoded, so no
		// request, no closure and no frame of placeholder.
		if t, ok := images.pipe.Lookup(id, size); ok {
			r.key = imageKey{id: id, rung: t.LadderSize(), rev: t.Revision()}
			if r.w == 0 || r.h == 0 {
				r.w, r.h = uint32(t.Width()), uint32(t.Height())
			}
			r.ok = true
			t.Release()
			return Picture{r: r}
		}
	}
	return Picture{r: images.request(src, size, pass, prio, notify)}
}

// Ready reports whether the pixels have arrived. A ready picture may still
// draw nothing in a frame whose upload budget is spent; see [Picture.Draw].
// It is not ready again once a frame found its pixels gone, evicted from the
// CPU cache before they could be uploaded or after their texture was
// evicted, until a layout has requested it again and they have arrived
// again.
func (p Picture) Ready() bool { return p.r != nil && p.r.ok }

// Err is the permanent failure of the picture, or nil.
func (p Picture) Err() error {
	if p.r == nil {
		return nil
	}
	return p.r.failure
}

// Dimensions are the oriented pixel dimensions the pipeline reported for the
// picture, or zero before it has. They are the picture's own size, not the
// rung's, when the pipeline probed it, and the rung's size when the picture
// was found in the cache before anything was probed; the aspect ratio is
// right in both cases.
func (p Picture) Dimensions() (w, h uint32) {
	if p.r == nil {
		return 0, 0
	}
	return p.r.w, p.r.h
}

// Rung is the ladder rung the pixels were produced at, or zero before they
// have arrived.
func (p Picture) Rung() int {
	if p.r == nil || !p.r.ok {
		return 0
	}
	return p.r.key.rung
}

// Draw draws the whole picture stretched into dst, in the local space of the
// node being painted, with tint and with its corners rounded by radius. It
// reports whether it drew: a picture that is not ready, or whose texture the
// frame's upload budget refused, draws nothing and is tried again next frame.
func (p Picture) Draw(ctx *gift.PaintContext, dst geom.Rect, tint Color, radius float32) bool {
	id, iw, ih, ok := images.drawable(ctx, p.r)
	if !ok {
		return false
	}
	paintImage(ctx, dst, id, iw, ih, FitStretch, tint, radius)
	return true
}

// DrawFit draws the picture into dst as fit says, with tint and with its
// corners rounded by radius: [FitCover] fills dst and crops the overhang, as
// the thumbnails of a gallery or the markers of a map do. It reports whether
// it drew, as [Picture.Draw].
func (p Picture) DrawFit(ctx *gift.PaintContext, dst geom.Rect, fit ImageFit, tint Color, radius float32) bool {
	id, iw, ih, ok := images.drawable(ctx, p.r)
	if !ok {
		return false
	}
	paintImage(ctx, dst, id, iw, ih, fit, tint, radius)
	return true
}

// DrawTile draws the whole picture stretched into dst like [Picture.Draw],
// sampling only between the centres of its edge texels, so that pictures
// drawn edge to edge show no seam between them under linear filtering; see
// [render.ImageTile]. It is how the tiles of a map are drawn.
func (p Picture) DrawTile(ctx *gift.PaintContext, dst geom.Rect, tint Color) bool {
	id, _, _, ok := images.drawable(ctx, p.r)
	if !ok {
		return false
	}
	ctx.Add(render.Op{Kind: render.OpImage, Bounds: dst, Color: tint, Image: id, Fit: render.ImageTile})
	return true
}

// Prepare makes the picture resident without drawing it, and reports whether
// it is. A view calls it for what is about to come on screen — the next page
// of a viewer — so that the upload, which a backend may spread over several
// frames for a large picture, is done before the picture is needed rather
// than in the frame it first appears in.
func (p Picture) Prepare(ctx *gift.PaintContext) bool {
	_, _, _, ok := images.drawable(ctx, p.r)
	return ok
}

// DrawCover draws the picture into dst like [FitCover]: the centred part of
// it with the aspect ratio of dst.
func (p Picture) DrawCover(ctx *gift.PaintContext, dst geom.Rect, tint Color, radius float32) bool {
	id, iw, ih, ok := images.drawable(ctx, p.r)
	if !ok {
		return false
	}
	paintImage(ctx, dst, id, iw, ih, FitCover, tint, radius)
	return true
}

// --- where a gallery shows a picture -----------------------------------------

// ThumbnailFrame returns where the tile of the entry id was drawn in the last
// frame, in device space, with the tile's corner radius, and whether it was
// drawn at all.
//
// It is what a full screen viewer opens from and closes back into: the hero
// transition of iOS's Photos, in which the picture grows out of its tile and
// shrinks back into it. It reports the rectangle as drawn, so a tile that is
// partly scrolled out of the viewport reports all of it, and a tile that was
// not drawn — scrolled away, or never bound — reports false.
func (g *Gallery) ThumbnailFrame(id asset.ID) (geom.Rect, float32, bool) {
	for i := range g.slots {
		s := &g.slots[i]
		if s.bound && s.id == id && s.paintSeq == g.paintSeq && g.paintSeq != 0 {
			return s.frame, g.style.CornerRadius, true
		}
	}
	return geom.Rect{}, 0, false
}

// Conceal hides the picture of the entry id in its tile until another call
// conceals a different one; the empty ID shows every tile again.
//
// A viewer conceals the tile it opened from, for as long as it shows that
// picture, because the picture is then on screen twice: once where it has
// moved to and once where it came from, which a swipe that drags the picture
// away uncovers.
func (g *Gallery) Conceal(id asset.ID) {
	if g.concealed == id {
		return
	}
	g.concealed = id
	g.Invalidate()
}
