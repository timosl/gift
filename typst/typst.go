package typst

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

// ErrUnavailable reports that libgifttypst could not be loaded.
var ErrUnavailable = errors.New("typst: libgifttypst is not available")

// ErrClosed reports a call on an [Engine] after [Engine.Close].
var ErrClosed = errors.New("typst: engine is closed")

// errBadBuffer is a buffer from the library that does not decode: a library
// built from another revision of the encoding.
var errBadBuffer = errors.New("typst: the library returned a malformed buffer")

// Open loads the library from path before anything else would look for it,
// or, with an empty path, from where the package looks by itself; see the
// package documentation. It must be called before the first use of the
// package to have an effect; afterwards it reports what that first use found.
func Open(path string) error {
	lib.path = path
	return load()
}

// bundled writes the library package typst/embedded carries to disk and
// returns its path; it is nil without that package.
var bundled func() (string, error)

// Provide is how package typst/embedded hands over the library it carries:
// f writes it to disk and returns its path. f is called at most once, and
// only if neither [Open] nor GIFT_TYPST_LIB names a library that loads. An
// application imports typst/embedded rather than calling Provide.
func Provide(f func() (path string, err error)) { bundled = f }

// Available reports whether the library was found and bound.
func Available() bool { return load() == nil }

// Version describes the loaded library, or is empty without one.
func Version() string {
	if load() != nil {
		return ""
	}
	return version()
}

// Engine is a Typst compiler with its fonts and one document.
//
// It keeps everything between compilations: the parsed source, the fonts
// and Typst's memoized intermediate results. A compile after a small edit
// therefore redoes only what the edit touched. An Engine is safe for
// concurrent use; calls are serialised.
type Engine struct {
	mu    sync.Mutex
	h     uintptr
	fonts []render.FontID
}

// New creates an engine without fonts. Typst draws nothing without a font,
// so add at least one with [Engine.AddFont].
func New() (*Engine, error) {
	if err := load(); err != nil {
		return nil, err
	}
	h := newEngine()
	if h == 0 {
		return nil, errors.New("typst: the library could not create an engine")
	}
	return &Engine{h: h}, nil
}

// Close releases the engine. Further calls return [ErrClosed].
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h != 0 {
		freeEngine(e.h)
		e.h = 0
	}
	return nil
}

// AddFont makes the faces of a TrueType or OpenType file available to
// documents, which select them by family, weight and style with Typst's own
// text function.
//
// The same bytes are loaded into gift's text stack, so that the glyphs Typst
// positions are drawn by gift's glyph atlas from the very same outlines: a
// glyph id means the same glyph on both sides only if both read one file.
// A collection (.ttc) is accepted by Typst but not drawn by gift.
func (e *Engine) AddFont(data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == 0 {
		return ErrClosed
	}
	n := addFont(e.h, data)
	if n == 0 {
		return fmt.Errorf("typst: not a font")
	}
	var id render.FontID
	if n == 1 {
		f, err := text.ParseFont(data)
		if err != nil {
			return err
		}
		id = render.FontID(f.ID())
	}
	for range n {
		e.fonts = append(e.fonts, id)
	}
	return nil
}

// SetSource replaces the text of the document. Typst compares it with the
// previous text and reparses only the part that changed.
func (e *Engine) SetSource(src string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == 0 {
		return ErrClosed
	}
	if !setSource(e.h, src) {
		return errors.New("typst: the source is not valid UTF-8")
	}
	return nil
}

// Compile lays the document out. On errors in the source it returns a
// *[CompileError].
func (e *Engine) Compile() (*Document, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == 0 {
		return nil, ErrClosed
	}
	code, buf := compile(e.h)
	switch code {
	case 0:
		return decodeDocument(buf, e.fonts)
	case 1:
		return nil, decodeErrors(buf)
	}
	return nil, errors.New("typst: the compiler failed")
}

// PDF exports the last document that compiled without errors.
func (e *Engine) PDF() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == 0 {
		return nil, ErrClosed
	}
	code, buf := pdf(e.h)
	if code != 0 {
		return nil, errors.New("typst: no document to export, or the export failed")
	}
	return buf, nil
}

// Diagnostic is one error in the source.
type Diagnostic struct {
	// Start and End are byte offsets into the source, -1 if Typst could
	// not attribute the error to it.
	Start, End int
	Message    string
}

// CompileError is the error of a document that did not compile.
type CompileError struct{ Diagnostics []Diagnostic }

func (e *CompileError) Error() string {
	var b strings.Builder
	for i, d := range e.Diagnostics {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d.Message)
	}
	return "typst: " + b.String()
}

// Document is a laid out document: its pages, in points.
type Document struct {
	Pages []Page
}

// Page is one page as a flat display list, in points from its top left
// corner, painted in the order of Items.
type Page struct {
	Width, Height float32
	Items         []Item
	Glyphs        []Glyph
	Vertices      []geom.Point
}

// ItemKind says what an [Item] draws.
type ItemKind uint8

const (
	// ItemText is a run of glyphs in one font, size and colour,
	// Page.Glyphs[First:First+Count].
	ItemText ItemKind = iota + 1
	// ItemRect is a filled rectangle: backgrounds, and every horizontal or
	// vertical rule, which is what a table's strokes are.
	ItemRect
	// ItemTriangles is a filled triangle list, Page.Vertices[First:First+Count],
	// three per triangle: everything else Typst draws, flattened.
	ItemTriangles
	// ItemImage is the place of a picture. The picture itself is not
	// transferred.
	ItemImage
)

// Item is one entry of a page's display list.
type Item struct {
	Kind ItemKind
	// Font and Size are those of a text run: a font of gift's text stack,
	// zero for a face it could not load, and the em size in points.
	Font render.FontID
	Size float32
	// Color is premultiplied, as everywhere in gift.
	Color render.Color
	// Bounds is the rectangle the item covers, roughly for text.
	Bounds       geom.Rect
	First, Count int
}

// Glyph is one positioned glyph of a text run.
type Glyph struct {
	ID render.GlyphID
	// X and Y are the origin on the baseline, Advance the width.
	X, Y, Advance float32
	// Source is the byte offset in the source of the text the glyph was
	// shaped from, or -1 for text not written there: numbering, text from a
	// show rule. Len is the length of that text in bytes.
	Source, Len int
}

type reader struct {
	b   []byte
	err bool
}

func (r *reader) u32() uint32 {
	if len(r.b) < 4 {
		r.err = true
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *reader) i32() int { return int(int32(r.u32())) }

func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }

func (r *reader) u8() byte {
	if len(r.b) < 1 {
		r.err = true
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) bytes(n int) []byte {
	if n < 0 || len(r.b) < n {
		r.err = true
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

// count reads a number of things of at least size bytes each, refusing one
// the rest of the buffer cannot hold.
func (r *reader) count(size int) int {
	n := int(r.u32())
	if n*size > len(r.b) {
		r.err = true
		return 0
	}
	return n
}

func rgba(v uint32) render.Color {
	return render.RGBA(uint8(v>>24), uint8(v>>16), uint8(v>>8), uint8(v))
}

func decodeErrors(buf []byte) error {
	r := &reader{b: buf}
	e := &CompileError{}
	for range r.count(12) {
		d := Diagnostic{Start: r.i32(), End: r.i32()}
		d.Message = string(r.bytes(int(r.u32())))
		e.Diagnostics = append(e.Diagnostics, d)
	}
	if r.err {
		return errBadBuffer
	}
	return e
}

func decodeDocument(buf []byte, fonts []render.FontID) (*Document, error) {
	r := &reader{b: buf}
	doc := &Document{Pages: make([]Page, r.count(12))}
	for i := range doc.Pages {
		p := &doc.Pages[i]
		p.Width, p.Height = r.f32(), r.f32()
		p.Items = make([]Item, 0, r.count(1))
		for range cap(p.Items) {
			p.Items = append(p.Items, decodeItem(r, p, fonts))
			if r.err {
				return nil, errBadBuffer
			}
		}
	}
	if r.err || len(r.b) != 0 {
		return nil, errBadBuffer
	}
	return doc, nil
}

func decodeItem(r *reader, p *Page, fonts []render.FontID) Item {
	switch ItemKind(r.u8()) {
	case ItemText:
		it := Item{Kind: ItemText}
		if f := int(r.u32()); f < len(fonts) {
			it.Font = fonts[f]
		}
		it.Size = r.f32()
		it.Color = rgba(r.u32())
		it.First = len(p.Glyphs)
		it.Count = r.count(24)
		minX, maxX := float32(math.Inf(1)), float32(math.Inf(-1))
		minY, maxY := minX, maxX
		for range it.Count {
			g := Glyph{ID: render.GlyphID(r.u32()), X: r.f32(), Y: r.f32(), Advance: r.f32(), Source: r.i32(), Len: int(r.u32())}
			p.Glyphs = append(p.Glyphs, g)
			minX, maxX = min(minX, g.X), max(maxX, g.X+g.Advance)
			minY, maxY = min(minY, g.Y-it.Size), max(maxY, g.Y+it.Size*0.3)
		}
		if it.Count > 0 {
			it.Bounds = geom.Rc(minX, minY, maxX, maxY)
		}
		return it
	case ItemRect:
		it := Item{Kind: ItemRect}
		it.Bounds = geom.RcXYWH(r.f32(), r.f32(), r.f32(), r.f32())
		it.Color = rgba(r.u32())
		return it
	case ItemTriangles:
		it := Item{Kind: ItemTriangles, Color: rgba(r.u32())}
		it.First = len(p.Vertices)
		it.Count = r.count(8)
		minX, maxX := float32(math.Inf(1)), float32(math.Inf(-1))
		minY, maxY := minX, maxX
		for range it.Count {
			v := geom.Pt(r.f32(), r.f32())
			p.Vertices = append(p.Vertices, v)
			minX, maxX = min(minX, v.X), max(maxX, v.X)
			minY, maxY = min(minY, v.Y), max(maxY, v.Y)
		}
		if it.Count > 0 {
			it.Bounds = geom.Rc(minX, minY, maxX, maxY)
		}
		return it
	case ItemImage:
		return Item{Kind: ItemImage, Bounds: geom.RcXYWH(r.f32(), r.f32(), r.f32(), r.f32())}
	}
	r.err = true
	return Item{}
}

// SourceAt returns the byte offset in the source that a click at (x, y) on
// the page means: before the glyph under the point if its left half was hit,
// after it otherwise. Off the text it takes the nearest glyph of the line the
// point is on, and off every line the nearest glyph at all. It reports false
// for a page without text that came from the source.
func (p *Page) SourceAt(x, y float32) (int, bool) {
	best, bestD := -1, float32(math.Inf(1))
	var bestG Glyph
	for _, it := range p.Items {
		if it.Kind != ItemText {
			continue
		}
		top, bottom := float32(0), float32(0)
		for i := it.First; i < it.First+it.Count; i++ {
			g := p.Glyphs[i]
			if g.Source < 0 {
				continue
			}
			top, bottom = g.Y-it.Size*0.8, g.Y+it.Size*0.25
			var dy float32
			switch {
			case y < top:
				dy = top - y
			case y > bottom:
				dy = y - bottom
			}
			var dx float32
			switch {
			case x < g.X:
				dx = g.X - x
			case x > g.X+g.Advance:
				dx = x - g.X - g.Advance
			}
			// A line is chosen before a glyph: any distance across lines
			// outweighs any distance along one.
			d := dy*1e4 + dx
			if d < bestD {
				best, bestD, bestG = i, d, g
			}
		}
	}
	if best < 0 {
		return 0, false
	}
	if x > bestG.X+bestG.Advance/2 {
		return bestG.Source + bestG.Len, true
	}
	return bestG.Source, true
}

// Caret is where a text cursor at a source offset is drawn.
type Caret struct {
	Page        int
	X, Top, Bot float32
}

// CaretAt returns the caret for a byte offset in the source: at the left edge
// of the glyph that starts there, or at the right edge of the glyph that ends
// there. It reports false for an offset no glyph starts or ends at, which is
// one inside markup, inside code or in text the document does not show.
func (d *Document) CaretAt(off int) (Caret, bool) {
	var end Caret
	found := false
	for pi := range d.Pages {
		p := &d.Pages[pi]
		for _, it := range p.Items {
			if it.Kind != ItemText {
				continue
			}
			for i := it.First; i < it.First+it.Count; i++ {
				g := p.Glyphs[i]
				if g.Source < 0 {
					continue
				}
				c := Caret{Page: pi, Top: g.Y - it.Size*0.8, Bot: g.Y + it.Size*0.25}
				if g.Source == off {
					c.X = g.X
					return c, true
				}
				if g.Source+g.Len == off && !found {
					c.X = g.X + g.Advance
					end, found = c, true
				}
			}
		}
	}
	return end, found
}
