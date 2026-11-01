package typst_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift/typst"
	_ "github.com/worldiety/gift/typst/embedded"
)

// engine opens the library built by `cargo build --release` in ffi/, if
// there is one, so that a change to the crate is tested before it is
// embedded, and otherwise the one typst/embedded carries. On a platform it
// carries none for, and without a build, the test is skipped.
func engine(tb testing.TB) *typst.Engine {
	tb.Helper()
	name := "libgifttypst.so"
	if runtime.GOOS == "darwin" {
		name = "libgifttypst.dylib"
	}
	path, _ := filepath.Abs(filepath.Join("ffi", "target", "release", name))
	if _, err := os.Stat(path); err != nil {
		path = ""
	}
	if err := typst.Open(path); err != nil {
		tb.Skipf("libgifttypst is not available: %v", err)
	}
	e, err := typst.New()
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { e.Close() })
	for _, f := range []string{"Inter-Regular.ttf", "Inter-Bold.ttf"} {
		data, err := os.ReadFile(filepath.Join("..", "font", "inter", f))
		if err != nil {
			tb.Fatal(err)
		}
		if err := e.AddFont(data); err != nil {
			tb.Fatal(err)
		}
	}
	return e
}

const doc = `#set page(width: 300pt, height: 200pt, margin: 20pt)
#set text(font: "Inter", size: 11pt)
= Rechnung

Hallo Welt.

#table(columns: 2, [Posten], [Preis], [Tee], [3,50])
`

func TestADocumentWithATableIsLaidOutInGlyphsAndRules(t *testing.T) {
	e := engine(t)
	if err := e.SetSource(doc); err != nil {
		t.Fatal(err)
	}
	d, err := e.Compile()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 1 || d.Pages[0].Width != 300 || d.Pages[0].Height != 200 {
		t.Fatalf("pages = %+v", d.Pages)
	}
	p := d.Pages[0]
	var texts, rects int
	for _, it := range p.Items {
		switch it.Kind {
		case typst.ItemText:
			texts++
			if it.Font == 0 {
				t.Errorf("a run without a gift font: %+v", it)
			}
		case typst.ItemRect:
			rects++
		}
	}
	// A table of two columns and two rows has three horizontal and three
	// vertical rules.
	if texts < 5 || rects < 6 {
		t.Fatalf("%d text runs and %d rectangles", texts, rects)
	}
}

func TestEveryGlyphOfTheSourceTextLeadsBackToIt(t *testing.T) {
	e := engine(t)
	e.SetSource(doc)
	d, err := e.Compile()
	if err != nil {
		t.Fatal(err)
	}
	p := d.Pages[0]
	var got strings.Builder
	for _, g := range p.Glyphs {
		if g.Source >= 0 {
			got.WriteString(doc[g.Source : g.Source+g.Len])
		}
	}
	for _, w := range []string{"Rechnung", "Hallo", "Welt.", "Posten", "Preis", "Tee", "3,50"} {
		if !strings.Contains(got.String(), w) {
			t.Errorf("%q is not in the text the glyphs lead back to: %q", w, got.String())
		}
	}

	// A click on the right half of the H of Hallo lands after it, and the
	// caret there is drawn at its right edge.
	at := strings.Index(doc, "Hallo")
	var h typst.Glyph
	for _, g := range p.Glyphs {
		if g.Source == at {
			h = g
		}
	}
	off, ok := p.SourceAt(h.X+h.Advance*0.75, h.Y-2)
	if !ok || off != at+1 {
		t.Fatalf("SourceAt = %d, %v, want %d", off, ok, at+1)
	}
	c, ok := d.CaretAt(at + 1)
	if !ok || c.X != h.X+h.Advance {
		t.Fatalf("CaretAt = %+v, %v, want x %v", c, ok, h.X+h.Advance)
	}
}

func TestAnErrorNamesItsPlaceInTheSource(t *testing.T) {
	e := engine(t)
	src := "Gut\n#unbekannt\n"
	e.SetSource(src)
	_, err := e.Compile()
	var ce *typst.CompileError
	if !errors.As(err, &ce) || len(ce.Diagnostics) == 0 {
		t.Fatalf("err = %v", err)
	}
	if d := ce.Diagnostics[0]; src[d.Start:d.End] != "unbekannt" {
		t.Fatalf("diagnostic %+v covers %q", d, src[d.Start:d.End])
	}
}

func TestTheLastDocumentIsExportedAsPDF(t *testing.T) {
	e := engine(t)
	e.SetSource(doc)
	if _, err := e.Compile(); err != nil {
		t.Fatal(err)
	}
	pdf, err := e.PDF()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("not a PDF: %q", pdf[:min(len(pdf), 16)])
	}
}

// TestAnEditIsCompiledIncrementally reports how long a compile takes after
// a one character edit in a document of many pages, against the first
// compile. It asserts only that the second is faster.
func TestAnEditIsCompiledIncrementally(t *testing.T) {
	e := engine(t)
	var b strings.Builder
	b.WriteString("#set text(font: \"Inter\")\n")
	for i := range 400 {
		b.WriteString("== Abschnitt\n\nDies ist ein Absatz mit etwas Text, der umbrochen werden muss, damit die Seite gefüllt wird. ")
		if i%10 == 0 {
			b.WriteString("\n\n#table(columns: 3, [a], [b], [c], [1], [2], [3])\n")
		}
		b.WriteString("\n\n")
	}
	src := b.String()
	e.SetSource(src)
	t0 := time.Now()
	d, err := e.Compile()
	if err != nil {
		t.Fatal(err)
	}
	first := time.Since(t0)
	mid := len(src) / 2
	mid = strings.Index(src[mid:], "Absatz") + mid
	e.SetSource(src[:mid] + "X" + src[mid:])
	t0 = time.Now()
	if _, err := e.Compile(); err != nil {
		t.Fatal(err)
	}
	again := time.Since(t0)
	t.Logf("%d pages: first compile %v, after an edit in the middle %v", len(d.Pages), first, again)
	if again >= first {
		t.Errorf("the edit took %v, the first compile %v", again, first)
	}
}

// BenchmarkEdit measures what a keystroke costs: one character inserted in
// the middle of a document of a given length, set again and handed to Go.
//
//	go test ./typst -run '^$' -bench Edit
func BenchmarkEdit(b *testing.B) {
	for _, paras := range []int{4, 40, 400, 1200} {
		var s strings.Builder
		s.WriteString("#set text(font: \"Inter\")\n#set par(justify: true)\n")
		for i := range paras {
			s.WriteString("== Abschnitt\n\nDies ist ein Absatz mit etwas Text, der umbrochen werden muss, damit die Seite gefüllt wird, und der lang genug ist, um mehrere Zeilen zu füllen. Er enthält keine besonderen Auszeichnungen und steht für gewöhnlichen Fließtext.\n\n")
			if i%10 == 0 {
				s.WriteString("#table(columns: 3, [a], [b], [c], [1], [2], [3])\n\n")
			}
		}
		src := s.String()
		mid := strings.Index(src[len(src)/2:], "Absatz") + len(src)/2
		b.Run(fmt.Sprintf("paragraphs=%d", paras), func(b *testing.B) {
			e := engine(b)
			e.SetSource(src)
			d, err := e.Compile()
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := range b.N {
				// Alternate between two texts, so that every compile has
				// an edit to process and none is a cache hit of the last.
				x := "X"
				if i%2 == 1 {
					x = "Y"
				}
				e.SetSource(src[:mid] + x + src[mid:])
				if _, err := e.Compile(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(d.Pages)), "pages")
		})
	}
}
