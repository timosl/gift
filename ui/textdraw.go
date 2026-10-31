package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

// TextStyle is how [DrawText] draws a line of text.
type TextStyle struct {
	// Font is the typeface; the zero value selects the default font.
	Font Font
	// Size is the font size in logical pixels.
	Size float32
	// Color is the colour of the glyphs. A semantic colour is resolved
	// against the current theme.
	Color Color
	// Halo, when not transparent, is drawn around the glyphs one pixel wide,
	// so that the text stays legible over any background: the outline of the
	// labels of a map.
	Halo Color
}

// MeasureText returns the size of s set in one line in st: its advance
// width and the height of its line box.
//
// It is the measuring half of [DrawText], for a view that places text itself
// — the labels of a map, which collide with each other and are placed in
// layout. The layout is cached by the text stack, so measuring the same
// string again is a lookup.
func MeasureText(s string, st TextStyle) geom.Size {
	if s == "" {
		return geom.Size{}
	}
	return text.Default().Layout(textRequest(s, st)).Size
}

// DrawText draws s in one line with the top left corner of its line box at
// at, in the local space of the node being painted. It is the low level
// half of [Text], as [Picture] is of [Image]: for a view that draws text at
// positions it computes, under a transform of its own, without a node per
// string. Glyph positions are rounded to whole pixels, as for every text in
// gift.
func DrawText(ctx *gift.PaintContext, s string, at geom.Point, st TextStyle) {
	if s == "" {
		return
	}
	p := text.Default().Layout(textRequest(s, st))
	if halo := ResolveColor(st.Halo); !halo.IsTransparent() {
		for _, d := range [...][2]float32{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}} {
			emitGlyphs(ctx, p, geom.Pt(at.X+d[0], at.Y+d[1]), halo)
		}
	}
	emitGlyphs(ctx, p, at, ResolveColor(st.Color))
}

func textRequest(s string, st TextStyle) text.Request {
	size := st.Size
	if size <= 0 {
		size = 17
	}
	return text.Request{Text: s, Font: resolveFont(st.Font), Size: size, MaxWidth: geom.Unbounded()}
}

// emitGlyphs copies the glyphs of p to the display list as one operation,
// the way a TextView paints its paragraph.
func emitGlyphs(ctx *gift.PaintContext, p *text.Paragraph, at geom.Point, c Color) {
	if c.IsTransparent() {
		return
	}
	first := ctx.GlyphsLen()
	for li := range p.Lines {
		ln := &p.Lines[li]
		lx, by := roundf(at.X), roundf(at.Y+ln.Baseline)
		for ri := range ln.Runs {
			run := &ln.Runs[ri]
			id := render.FontID(run.Font.ID())
			for gi := range run.Glyphs {
				g := &run.Glyphs[gi]
				ctx.AppendGlyph(render.Glyph{Font: id, ID: render.GlyphID(g.ID), Size: run.Size, X: lx + g.X, Y: by + g.Y})
			}
		}
	}
	if n := ctx.GlyphsLen() - first; n > 0 {
		ctx.Add(render.Op{
			Kind:       render.OpGlyphs,
			Bounds:     geom.Rect{Min: at, Max: geom.Pt(at.X+p.Size.W, at.Y+p.Size.H)},
			Color:      c,
			Glyphs:     first,
			GlyphCount: n,
		})
	}
}
