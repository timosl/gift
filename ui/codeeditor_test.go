package ui_test

import (
	"strings"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// keywords is a highlighter that knows one word.
var keywords = ui.HighlighterFunc(func(src string, dst []ui.Token) []ui.Token {
	for off := 0; ; {
		i := strings.Index(src[off:], "let")
		if i < 0 {
			return dst
		}
		dst = append(dst, ui.Token{Start: off + i, End: off + i + 3, Kind: ui.TokenKeyword})
		off += i + 3
	}
})

// keywordColor is the colour of a keyword in the light theme.
var keywordColor = ui.RGB(0xAD, 0x3D, 0xA4)

// codeFixture is one code editor alone in the window, focused, with a box
// after it that takes the focus on tab.
type codeFixture struct {
	h  *gifttest.Harness
	ed *ui.TextEditor
}

func newCode(t *testing.T, initial string, decorate func(ui.CodeEditorView) ui.CodeEditorView) *codeFixture {
	t.Helper()
	f := &codeFixture{ed: ui.NewTextEditor(initial)}
	v := ui.CodeEditor(f.ed).Font(loadTestFont(t)).FontSize(16).Highlighter(keywords).
		Frame(300, geom.Unbounded()).Key("code")
	if decorate != nil {
		v = decorate(v)
	}
	f.h = gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.Box(), ui.VStack(v, ui.TextField(ui.NewTextEditor("")).Key("next")).Padding(20)),
		Size: geom.Sz(400, 400),
		Font: loadTestFont(t),
	})
	f.h.Find(gifttest.ByKey("code")).Focus()
	return f
}

// glyphOps are the glyph runs of the frame, by colour.
func glyphOps(h *gifttest.Harness) []render.Op {
	var out []render.Op
	for _, op := range h.Ops() {
		if op.Kind == render.OpGlyphs {
			out = append(out, op)
		}
	}
	return out
}

func TestACodeEditorNumbersItsLinesBesideTheTextAndColoursItsTokens(t *testing.T) {
	f := newCode(t, "let x = 1\nprint(x)\n\nlet y", nil)
	edge := ui.ResolveColor(ui.ColorSeparator)
	var gutter geom.Rect
	for _, op := range f.h.Ops() {
		if op.Kind == render.OpFillRect && op.Color == edge && op.Bounds.Width() == 1 {
			gutter = op.Bounds
		}
	}
	if gutter.IsEmpty() {
		t.Fatal("no edge of the gutter drawn")
	}
	var keywordGlyphs, numbers int
	textLeft := float32(1e9)
	for _, op := range glyphOps(f.h) {
		if op.Color == keywordColor {
			keywordGlyphs += int(op.GlyphCount)
		}
		if op.GlyphCount == 1 && op.Bounds.Width() > 0 {
			g := f.h.List().Glyphs(op.Glyphs, 1)[0]
			if g.X < gutter.Min.X {
				numbers++
				continue
			}
		}
		textLeft = min(textLeft, f.h.List().Glyphs(op.Glyphs, 1)[0].X)
	}
	if keywordGlyphs != 6 {
		t.Errorf("%d glyphs in the colour of a keyword, want the six of two lets", keywordGlyphs)
	}
	if numbers != 4 {
		t.Errorf("%d line numbers left of the gutter, want 4", numbers)
	}
	if textLeft <= gutter.Max.X {
		t.Errorf("the text starts at %v, inside the gutter that ends at %v", textLeft, gutter.Max.X)
	}
}

func TestTabIndentsAndShiftTabTakesItBackAndEscapeTabLeaves(t *testing.T) {
	f := newCode(t, "a\nb\nc", func(v ui.CodeEditorView) ui.CodeEditorView { return v.Indent("  ") })
	f.ed.SetCaret(0)
	f.h.Key(gift.KeyTab)
	if got := f.ed.Text(); got != "  a\nb\nc" {
		t.Fatalf("after tab %q", got)
	}
	f.h.AssertFocus(gifttest.ByKey("code"))

	// A selection over the first two lines indents both and stays over
	// them.
	f.ed.SetSelection(0, 6)
	f.h.Key(gift.KeyTab)
	if got := f.ed.Text(); got != "    a\n  b\nc" {
		t.Fatalf("after tab over two lines %q", got)
	}
	if lo, hi := f.ed.Selection(); lo != 0 || hi != 10 {
		t.Fatalf("the selection is %d..%d, want 0..10", lo, hi)
	}
	f.h.Key(gift.KeyTab, gift.ModShift)
	f.h.Key(gift.KeyTab, gift.ModShift)
	if got := f.ed.Text(); got != "a\nb\nc" {
		t.Fatalf("after two shift-tabs %q", got)
	}

	f.h.Key(gift.KeyEscape)
	f.h.Key(gift.KeyTab)
	f.h.AssertFocus(gifttest.ByKey("next"))
	if got := f.ed.Text(); got != "a\nb\nc" {
		t.Fatalf("escape and tab edited the text: %q", got)
	}
}

func TestEnterStartsTheNewLineAtTheIndentationOfTheOld(t *testing.T) {
	f := newCode(t, "func f() {\n\tif x {", nil)
	f.ed.SetCaret(f.ed.Len())
	f.h.Key(gift.KeyEnter)
	f.h.TypeText("y")
	if got := f.ed.Text(); got != "func f() {\n\tif x {\n\ty" {
		t.Fatalf("%q", got)
	}
}

func TestAMarkTintsItsLineAndUnderlinesItsTextAndItsNumber(t *testing.T) {
	src := "first\nsecond line\nthird"
	at := strings.Index(src, "line")
	f := newCode(t, src, func(v ui.CodeEditorView) ui.CodeEditorView {
		return v.Marks(ui.CodeMark{Start: at, End: at + 4})
	})
	danger := ui.ResolveColor(ui.ColorDanger)
	tint := ui.Fade(danger, 0.14)
	var tints, underlines []geom.Rect
	for _, op := range f.h.Ops() {
		switch {
		case op.Kind == render.OpFillRect && op.Color == tint:
			tints = append(tints, op.Bounds)
		case op.Kind == render.OpFillRect && op.Color == danger:
			underlines = append(underlines, op.Bounds)
		}
	}
	if len(tints) != 1 || len(underlines) != 1 {
		t.Fatalf("%d tinted rows and %d underlines, want one of each", len(tints), len(underlines))
	}
	if u, r := underlines[0], tints[0]; u.Min.Y < r.Min.Y || u.Max.Y > r.Max.Y || u.Width() < 20 {
		t.Fatalf("the underline %v is not under the word in the tinted row %v", u, r)
	}
	var red int
	for _, op := range glyphOps(f.h) {
		if op.Color == danger {
			red++
		}
	}
	if red != 1 {
		t.Fatalf("%d glyph runs in the colour of the mark, want the number of its line", red)
	}
}
