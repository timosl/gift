package imageviewer

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

// The sizes the stages are requested at.
var (
	// PreviewSize is the longest edge of the first stage, in logical pixels:
	// the size a gallery's tiles are usually decoded at, so that a picture
	// opened from a gallery is in the pipeline's cache already and shows in
	// the first frame. A picture that is not is decoded at this size first,
	// which a [asset.ScaledDecoder] does from the DC coefficients of a JPEG
	// alone — one value per 8 by 8 block, the cheapest picture there is.
	PreviewSize = 256
	// MaxDetail caps the longest edge of the third stage, the one a zoomed
	// picture is decoded at, in device pixels. 4096 is the largest texture
	// every target guarantees; a backend refuses a larger one.
	MaxDetail = 4096
)

// stages are the pictures of one source at the sizes a zoomable view shows it
// at: a preview at once, the screen as soon as it has arrived, and more
// detail when the picture is zoomed into. Each is drawn as soon as it is
// there, sharpest first, so the view is never empty after its first frame
// and never blurrier than what has arrived.
type stages struct {
	preview, screen, detail ui.Picture
}

// requestStages issues the requests for src in a viewport of w by h logical
// pixels at density, zoomed by z. The detail stage is requested for a zoom of
// more than one and a quarter only, and at a power of two of the screen size,
// so that a zoom that changes by a little does not ask for a new decode.
// natural is the longest edge of the picture itself, or zero when unknown.
// A neighbour that is not on screen yet is prefetched instead.
func requestStages(ctx *gift.LayoutContext, src asset.Source, w, h, density float32, z float64, natural int, neighbour bool) stages {
	if src == nil {
		return stages{}
	}
	screen := int(math.Ceil(float64(max(w, h) * density)))
	if natural > 0 {
		screen = min(screen, natural)
	}
	preview := int(math.Ceil(float64(PreviewSize) * float64(density)))
	get := ui.RequestPicture
	if neighbour {
		get = ui.PrefetchPicture
	}
	var s stages
	s.preview = get(ctx, src, min(preview, max(1, screen)))
	if screen > preview {
		s.screen = get(ctx, src, screen)
	}
	if !neighbour && z > 1.25 {
		f := math.Pow(2, math.Ceil(math.Log2(z)))
		d := min(int(float64(screen)*f), MaxDetail)
		if natural > 0 {
			d = min(d, natural)
		}
		if d > screen {
			s.detail = get(ctx, src, d)
		}
	}
	return s
}

// aspect is the width over height of the picture, from whatever stage knows
// it, or zero.
func (s stages) aspect() float64 {
	for _, p := range [...]ui.Picture{s.detail, s.screen, s.preview} {
		if w, h := p.Dimensions(); w > 0 && h > 0 {
			return float64(w) / float64(h)
		}
	}
	return 0
}

// ready reports whether any stage has pixels.
func (s stages) ready() bool { return s.preview.Ready() || s.screen.Ready() || s.detail.Ready() }

// draw draws the sharpest stage that can be drawn this frame into dst.
func (s stages) draw(ctx *gift.PaintContext, dst geom.Rect, tint ui.Color, radius float32) bool {
	return s.detail.Draw(ctx, dst, tint, radius) ||
		s.screen.Draw(ctx, dst, tint, radius) ||
		s.preview.Draw(ctx, dst, tint, radius)
}

// prepare makes the preview and the screen stage resident without drawing
// them; see [ui.Picture.Prepare].
func (s stages) prepare(ctx *gift.PaintContext) {
	s.preview.Prepare(ctx)
	s.screen.Prepare(ctx)
}

// longestEdge is the longest edge a source declares about itself, or zero.
func longestEdge(src asset.Source) int {
	if src == nil {
		return 0
	}
	m := src.Metadata()
	return int(max(m.Width, m.Height))
}
