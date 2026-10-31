package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// newSecureField is newField with a SecureField, focused.
func newSecureField(t *testing.T, initial string, decorate func(ui.TextFieldView) ui.TextFieldView) *fieldFixture {
	t.Helper()
	f := &fieldFixture{ed: ui.NewTextEditor(initial)}
	v := ui.SecureField(f.ed).
		Font(loadTestFont(t)).
		FontSize(16).
		Frame(200, geom.Unbounded()).
		OnChange(func(s string) { f.change = append(f.change, s) }).
		Key("field")
	if decorate != nil {
		v = decorate(v)
	}
	f.h = gifttest.New(t, gifttest.Options{
		View: ui.ZStack(ui.Box(), ui.VStack(v).Padding(20)),
		Size: geom.Sz(400, 200),
		Font: loadTestFont(t),
	})
	f.node().Focus()
	return f
}

// fieldGlyphs are the glyph ids the frame draws, in order.
func fieldGlyphs(h *gifttest.Harness) []render.GlyphID {
	var ids []render.GlyphID
	l := h.List()
	for _, op := range h.Ops() {
		if op.Kind != render.OpGlyphs {
			continue
		}
		for _, g := range l.Glyphs(op.Glyphs, op.GlyphCount) {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

// fieldDots counts the dots of a secure field in the frame: the small
// circles it draws.
func fieldDots(h *gifttest.Harness) int {
	n := 0
	for _, op := range h.Ops() {
		if op.Kind == render.OpFillRoundRect && op.Bounds.Width() < 12 && op.Bounds.Width() == op.Bounds.Height() {
			n++
		}
	}
	return n
}

func allSame(ids []render.GlyphID) bool {
	for _, id := range ids {
		if id != ids[0] {
			return false
		}
	}
	return true
}

func TestASecureFieldDrawsAndReportsADotPerCharacter(t *testing.T) {
	f := newSecureField(t, "", func(v ui.TextFieldView) ui.TextFieldView { return v.ShowsLastCharacter(false) })
	f.h.TypeText("pässwort")
	if got := f.ed.Text(); got != "pässwort" {
		t.Fatalf("the document is %q", got)
	}
	if got, want := f.node().Value(), strings.Repeat("•", 8); got != want {
		t.Fatalf("the field reports %q, want %q", got, want)
	}
	if got := f.change[len(f.change)-1]; got != "pässwort" {
		t.Fatalf("OnChange got %q", got)
	}
	if n := fieldDots(f.h); n != 8 {
		t.Fatalf("the field draws %d dots, want 8", n)
	}
	if ids := fieldGlyphs(f.h); len(ids) != 0 {
		t.Fatalf("the field draws glyphs %v besides the dots", ids)
	}
	// A click after the third dot puts the caret after the third character,
	// which is the fourth byte: ä takes two. A dot takes the room of a digit.
	dot := ui.MeasureForTest(loadTestFont(t), "0", 16, geom.Unbounded()).W
	b := f.node().Bounds()
	f.h.ClickAt(geom.Pt(b.Min.X+fieldPadLeft+3*dot+1, b.Min.Y+b.Height()/2))
	if got := f.ed.Caret(); got != 4 {
		t.Fatalf("a click after the third dot put the caret at %d, want 4", got)
	}
	// Revealed shows the text in the clear.
	g := newSecureField(t, "abc", func(v ui.TextFieldView) ui.TextFieldView { return v.Revealed(true) })
	if got := g.node().Value(); got != "abc" {
		t.Fatalf("a revealed field reports %q", got)
	}
	if ids := fieldGlyphs(g.h); len(ids) != 3 || allSame(ids) || fieldDots(g.h) != 0 {
		t.Fatalf("a revealed field draws %v and %d dots", ids, fieldDots(g.h))
	}
}

func TestASecureFieldNeitherCopiesNorCutsButPastes(t *testing.T) {
	cb := &ui.MemoryClipboard{}
	ui.SetClipboard(cb)
	t.Cleanup(func() { ui.SetClipboard(nil) })
	cb.SetText("before")

	f := newSecureField(t, "secret", nil)
	f.h.Key(gift.KeyA, gift.ShortcutModifier)
	f.h.Key(gift.KeyC, gift.ShortcutModifier)
	f.h.Key(gift.KeyX, gift.ShortcutModifier)
	if got, _ := cb.Text(); got != "before" {
		t.Fatalf("the clipboard holds %q after copy and cut", got)
	}
	if got := f.ed.Text(); got != "secret" {
		t.Fatalf("cut changed the document to %q", got)
	}
	f.h.Key(gift.KeyEnd)
	f.h.Key(gift.KeyV, gift.ShortcutModifier)
	if got := f.ed.Text(); got != "secretbefore" {
		t.Fatalf("paste produced %q", got)
	}
}

func TestASecureFieldHasNoWords(t *testing.T) {
	f := newSecureField(t, "two words", nil)
	f.h.Key(gift.KeyLeft, gift.WordModifier)
	if got := f.ed.Caret(); got != 0 {
		t.Fatalf("the word motion stopped at %d, want the start", got)
	}
	f.h.Key(gift.KeyRight, gift.WordModifier)
	if got := f.ed.Caret(); got != len("two words") {
		t.Fatalf("the word motion stopped at %d, want the end", got)
	}
	// A double click selects everything, not the word under it.
	b := f.node().Bounds()
	p := geom.Pt(b.Min.X+fieldPadLeft+4, b.Min.Y+b.Height()/2)
	f.h.ClickAt(p)
	f.h.ClickAt(p)
	if lo, hi := f.ed.Selection(); lo != 0 || hi != len("two words") {
		t.Fatalf("a double click selected %d..%d", lo, hi)
	}
}

func TestASecureFieldShowsTheLastCharacterBriefly(t *testing.T) {
	f := newSecureField(t, "", func(v ui.TextFieldView) ui.TextFieldView { return v.ShowsLastCharacter(true) })
	f.h.TypeText("ab")
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 1 || len(ids) != 1 {
		t.Fatalf("right after typing the field draws %d dots and %v, want a dot and the b", fieldDots(f.h), ids)
	}
	if got := f.node().Value(); got != "••" {
		t.Fatalf("the field reports %q while revealing", got)
	}
	f.h.Advance(ui.SecureRevealDuration + 100*time.Millisecond)
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 2 || len(ids) != 0 {
		t.Fatalf("after the reveal the field draws %d dots and %v, want two dots", fieldDots(f.h), ids)
	}
	// Any key hides it at once.
	f.h.TypeText("c")
	if ids := fieldGlyphs(f.h); len(ids) != 1 || fieldDots(f.h) != 2 {
		t.Fatal("the c was not shown")
	}
	f.h.Key(gift.KeyLeft)
	if ids := fieldGlyphs(f.h); fieldDots(f.h) != 3 || len(ids) != 0 {
		t.Fatalf("after a key the field draws %d dots and %v, want three dots", fieldDots(f.h), ids)
	}
	// Without the on-screen keyboard the default is off, as on macOS.
	g := newSecureField(t, "", nil)
	g.h.TypeText("ab")
	if ids := fieldGlyphs(g.h); len(ids) != 0 || fieldDots(g.h) != 2 {
		t.Fatal("a desktop field showed the last character")
	}
}
