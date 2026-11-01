// Package typstview shows and edits documents that Typst sets, with gift's
// own renderer: Typst's glyphs through gift's glyph atlas, its rules as
// rectangles, its other shapes as triangles.
//
// A [Document] is the source of a document, a caret in it and an engine
// that sets it on a goroutine of its own. [Editor] edits it on its pages,
// below a toolbar, the way a word processor does, and [Viewer] shows the
// pages; both follow the document as it changes. Every edit on a page is an
// edit of the source, so a program can edit the same document as code, in a
// ui.CodeEditor with syntax.Typst say, and the pages follow.
//
//	doc := typstview.Use(ctx, "letter", source)
//	typstview.Editor(doc).Flex(1)
//
// [Page] is the drawing alone: one page of a document a program set itself
// with package typst.
package typstview

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/typst"
)

var pageType = gift.RegisterType("typstview.Page")

// The colours of the paper and the caret. A page is paper in either theme.
var (
	paper = render.RGB(255, 255, 255)
	edge  = render.RGBA(0, 0, 0, 40)
	caret = render.RGB(0, 122, 255)
)

// PageView is one page of a document. It is created by [Page].
type PageView struct {
	doc     *typst.Document
	page    int
	zoom    float32
	caret   int
	key     string
	onCaret func(off int)
	onKey   func(e gift.Event) bool
}

// Page returns the view of page i of doc at one pixel per point.
func Page(doc *typst.Document, i int) PageView {
	return PageView{doc: doc, page: i, zoom: 1, caret: -1}
}

// Zoom sets the scale in logical pixels per point.
func (v PageView) Zoom(z float32) PageView { v.zoom = z; return v }

// Caret draws a caret at a byte offset of the source, if the offset is shown
// on this page. A negative offset draws none.
func (v PageView) Caret(off int) PageView { v.caret = off; return v }

// OnCaret is called with the source offset a click on the page means.
func (v PageView) OnCaret(f func(off int)) PageView { v.onCaret = f; return v }

// OnKey is called with the key and rune events the page receives while it has
// the focus, and reports whether it handled them.
func (v PageView) OnKey(f func(e gift.Event) bool) PageView { v.onKey = f; return v }

// Key sets the reconciliation key among siblings.
func (v PageView) Key(k string) PageView { v.key = k; return v }

// ViewType implements gift.View.
func (PageView) ViewType() gift.TypeID { return pageType }

// Build implements gift.View.
func (v PageView) Build(*gift.BuildContext) gift.Element {
	n := &pageNode{v: v}
	return gift.Element{Key: v.key, Layouter: n, Painter: n, Interactor: n, Focusable: true}
}

type pageNode struct {
	v PageView
}

func (n *pageNode) pg() *typst.Page {
	if n.v.doc == nil || n.v.page < 0 || n.v.page >= len(n.v.doc.Pages) {
		return nil
	}
	return &n.v.doc.Pages[n.v.page]
}

// Layout implements gift.Layouter: the page at its size times the zoom.
func (n *pageNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	p := n.pg()
	if p == nil {
		return c.Constrain(geom.Sz(0, 0))
	}
	return c.Constrain(geom.Sz(p.Width*n.v.zoom, p.Height*n.v.zoom))
}

// Paint implements gift.Painter.
func (n *pageNode) Paint(ctx *gift.PaintContext) {
	p := n.pg()
	if p == nil {
		return
	}
	paintPage(ctx, p, n.v.zoom, func(at func(geom.Rect) geom.Rect) {
		if n.v.caret < 0 {
			return
		}
		if c, ok := n.v.doc.CaretAt(n.v.caret); ok && c.Page == n.v.page {
			r := at(geom.Rc(c.X, c.Top, c.X, c.Bot))
			ctx.Add(render.Op{Kind: render.OpFillRect, Color: caret,
				Bounds: geom.Rc(r.Min.X-0.75, r.Min.Y, r.Min.X+0.75, r.Max.Y)})
		}
	})
}

// paintPage draws page p at zoom z into the bounds of ctx: the paper, then
// the items of the page, then whatever over draws on top, inside the clip
// of the page. over is given the mapping from points of the page to the
// bounds.
func paintPage(ctx *gift.PaintContext, p *typst.Page, z float32, over func(at func(geom.Rect) geom.Rect)) {
	b := ctx.Bounds()
	db := ctx.DeviceBounds()
	// d is device pixels per logical pixel, for snapping glyph origins to
	// whole device pixels: the atlas rasterises a glyph once per size, and
	// an origin between two pixels would be drawn half a pixel off.
	d := float32(1)
	if b.Width() > 0 {
		d = db.Width() / b.Width()
	}
	snap := func(local, min, devMin float32) float32 {
		return (float32(math.Round(float64((local-min)*d+devMin)))-devMin)/d + min
	}
	at := func(r geom.Rect) geom.Rect {
		return geom.Rc(b.Min.X+r.Min.X*z, b.Min.Y+r.Min.Y*z, b.Min.X+r.Max.X*z, b.Min.Y+r.Max.Y*z)
	}
	// A rule of a table is half a point, which at most zooms is a fraction
	// of a device pixel more than one. Its edges rounded one by one, it
	// would be one pixel thick on some rows and two on others, depending on
	// where the row falls. So a thin band keeps its rounded thickness, at
	// least one pixel, from its rounded start; a fill has its edges rounded,
	// so that two fills that touch still touch.
	span := func(lo, hi, min, devMin float32) (float32, float32) {
		s := snap(lo, min, devMin)
		if (hi-lo)*d >= 3 {
			return s, snap(hi, min, devMin)
		}
		return s, s + max(1, float32(math.Round(float64((hi-lo)*d))))/d
	}
	snapped := func(r geom.Rect) geom.Rect {
		r = at(r)
		x0, x1 := span(r.Min.X, r.Max.X, b.Min.X, db.Min.X)
		y0, y1 := span(r.Min.Y, r.Max.Y, b.Min.Y, db.Min.Y)
		return geom.Rc(x0, y0, x1, y1)
	}

	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: paper})
	ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: b, Color: edge, StrokeWidth: 1})
	ctx.PushClip(db)
	defer ctx.PopClip()
	for _, it := range p.Items {
		switch it.Kind {
		case typst.ItemRect:
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: snapped(it.Bounds), Color: it.Color})
		case typst.ItemImage:
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: at(it.Bounds), Color: render.RGB(220, 220, 220)})
		case typst.ItemTriangles:
			first := ctx.VerticesLen()
			for _, v := range p.Vertices[it.First : it.First+it.Count] {
				ctx.AppendVertex(render.Vertex{X: b.Min.X + v.X*z, Y: b.Min.Y + v.Y*z, Color: it.Color})
			}
			ctx.Add(render.Op{Kind: render.OpTriangles, Bounds: at(it.Bounds), Color: it.Color,
				Glyphs: first, GlyphCount: uint32(it.Count)})
		case typst.ItemText:
			if it.Font == 0 {
				continue
			}
			first := ctx.GlyphsLen()
			for _, g := range p.Glyphs[it.First : it.First+it.Count] {
				ctx.AppendGlyph(render.Glyph{
					Font: it.Font, ID: g.ID, Size: it.Size * z,
					X: snap(b.Min.X+g.X*z, b.Min.X, db.Min.X),
					Y: snap(b.Min.Y+g.Y*z, b.Min.Y, db.Min.Y),
				})
			}
			ctx.Add(render.Op{Kind: render.OpGlyphs, Bounds: at(it.Bounds), Color: it.Color,
				Glyphs: first, GlyphCount: uint32(it.Count)})
		}
	}
	if over != nil {
		over(at)
	}
}

// HandleEvent implements gift.Interactor.
func (n *pageNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		p := n.pg()
		db := ctx.DeviceBounds()
		if p == nil || n.v.onCaret == nil || !(db.Width() > 0) || !(db.Height() > 0) {
			return true
		}
		x := (e.Pos.X - db.Min.X) / db.Width() * p.Width
		y := (e.Pos.Y - db.Min.Y) / db.Height() * p.Height
		if off, ok := p.SourceAt(x, y); ok {
			n.v.onCaret(off)
		}
		return true
	case gift.EventKeyDown, gift.EventRune:
		if n.v.onKey != nil && ctx.Interaction().Focused {
			return n.v.onKey(e)
		}
	}
	return false
}
