// Command example-typst is the spike for Typst as gift's layout engine for
// paged documents: Typst sets the pages, gift draws them, and the pages can
// be edited where they are set.
//
// The Typst compiler is a separate library, libgifttypst, built from the Rust
// crate in typst/ffi and loaded at run time without cgo:
//
//	(cd typst/ffi && cargo build --release)
//	go run ./cmd/example-typst
//
// On the left is the source, on the right the pages. Typing in either one
// edits the source and compiles it again; a click on a page puts the caret
// where the click landed. The status line says how long the last compilation
// took, and the button writes the document as a PDF.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/font/ibmplexmono"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/typst"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/typstview"
)

const sample = `#set page(width: 420pt, height: 595pt, margin: 36pt, numbering: "1")
#set text(font: "Inter", size: 10.5pt, lang: "de")
#set par(justify: true)

= Angebot 2026-117

Sehr geehrte Damen und Herren,

vielen Dank für Ihre Anfrage. Klicken Sie in diesen Text und tippen Sie: Die Seite wird bei jedem Tastendruck neu gesetzt, und der Cursor folgt dem Text. Die Seitenzahlen unten sind erzeugt und lassen sich deshalb nicht anklicken.

#table(
  columns: (auto, 1fr, auto, auto),
  align: (left, left, right, right),
  fill: (_, y) => if y == 0 { rgb("#e8eef7") },
  stroke: 0.5pt + rgb("#9aa5b4"),
  table.header([*Pos.*], [*Leistung*], [*Menge*], [*Preis*]),
  [1], [Konzeption und Workshop], [2], [1.200,00 €],
  [2], [Gestaltung der Oberfläche], [5], [3.000,00 €],
  [3], [Umsetzung des Editors], [12], [9.600,00 €],
  table.cell(colspan: 3, align: right)[*Summe netto*], [*13.800,00 €*],
)

== Positionen im Einzelnen

Die folgende Tabelle ist länger als eine Seite. Typst wiederholt ihren Kopf auf jeder neuen Seite.

#table(
  columns: (auto, 1fr, auto),
  align: (right, left, right),
  stroke: 0.5pt + rgb("#9aa5b4"),
  table.header([*Nr.*], [*Arbeitspaket*], [*Stunden*]),
  ..range(1, 61).map(i => ([#i], [Arbeitspaket #i], [#(calc.rem(i * 7, 13) + 2)])).flatten(),
)

Mit freundlichen Grüßen
`

var (
	app    *gift.App
	engine *typst.Engine
	ed     = ui.NewTextEditor(sample)

	// The results of the last compilation, written on the UI goroutine.
	doc     *typst.Document
	status  string
	problem string
	caret   = -1
	rev     *gift.State[int]

	// work hands the newest source to the compiling goroutine; a source
	// that is replaced before the goroutine takes it is never compiled.
	work = make(chan string, 1)
	mu   sync.Mutex
)

func main() {
	name := "libgifttypst.so"
	if runtime.GOOS == "darwin" {
		name = "libgifttypst.dylib"
	}
	lib := flag.String("lib", filepath.Join("typst", "ffi", "target", "release", name), "path of libgifttypst")
	fonts := flag.String("fonts", filepath.Join("font", "inter"), "directory with the .ttf fonts for Typst")
	flag.Parse()

	if err := run(*lib, *fonts); err != nil {
		fmt.Fprintln(os.Stderr, "example-typst:", err)
		os.Exit(1)
	}
}

func run(lib, fonts string) error {
	if err := example.LoadFont(); err != nil {
		return err
	}
	if err := typst.Open(lib); err != nil {
		return fmt.Errorf("%w (build it with: cd typst/ffi && cargo build --release)", err)
	}
	var err error
	if engine, err = typst.New(); err != nil {
		return err
	}
	files, _ := filepath.Glob(filepath.Join(fonts, "*.ttf"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := engine.AddFont(data); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("no .ttf fonts in %s", fonts)
	}

	go compiler()
	submit(ed.Text())

	app = gift.New(gift.Options{Root: root})
	return backend.Run(app, backend.Config{Title: "gift Typst – " + typst.Version(), Width: 1280, Height: 860})
}

// submit queues src for compilation, replacing a source still waiting.
func submit(src string) {
	mu.Lock()
	defer mu.Unlock()
	select {
	case <-work:
	default:
	}
	work <- src
}

// compiler compiles one source after the other, on its own goroutine, and
// posts each result to the UI goroutine.
func compiler() {
	for src := range work {
		t0 := time.Now()
		err := engine.SetSource(src)
		var d *typst.Document
		if err == nil {
			d, err = engine.Compile()
		}
		took := time.Since(t0)
		post(func() {
			if err != nil {
				problem = describe(src, err)
			} else {
				doc, problem = d, ""
				status = fmt.Sprintf("%d Seiten in %.1f ms gesetzt", len(d.Pages), float64(took.Microseconds())/1000)
			}
			if rev != nil {
				rev.Set(rev.Get() + 1)
			}
		})
	}
}

func post(f func()) {
	for app == nil {
		time.Sleep(time.Millisecond)
	}
	app.Post(f)
}

// describe turns a compile error into a line with the line number of its
// first diagnostic.
func describe(src string, err error) string {
	ce, ok := err.(*typst.CompileError)
	if !ok || len(ce.Diagnostics) == 0 {
		return err.Error()
	}
	d := ce.Diagnostics[0]
	if d.Start < 0 || d.Start > len(src) {
		return d.Message
	}
	return fmt.Sprintf("Zeile %d: %s", strings.Count(src[:d.Start], "\n")+1, d.Message)
}

func root(ctx *gift.Context) gift.View {
	rev = ctx.State("rev", 0)
	ctx.Read(rev)

	var pages []gift.View
	if doc != nil {
		for i := range doc.Pages {
			pages = append(pages, typstview.Page(doc, i).Zoom(1.25).Caret(caret).
				OnCaret(func(off int) {
					caret = off
					ed.SetCaret(off)
					rev.Set(rev.Get() + 1)
				}).
				OnKey(typed).Key(strconv.Itoa(i)))
		}
	}
	line := status
	if problem != "" {
		line = problem
	}
	mono := ui.MustFont(ui.FontQuery{Family: ibmplexmono.Family})
	return ui.Window(ui.VStack(
		ui.HStack(
			ui.Text("Typst in gift").FontSize(18),
			ui.Text(line).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			ui.Spacer(),
			ui.Button(ui.Text("PDF speichern").FontSize(13), savePDF),
		).Gap(16).AlignBaseline(),
		ui.HStack(
			ui.TextArea(ed).Font(mono).FontSize(12).OnChange(func(s string) {
				caret = ed.Caret()
				submit(s)
			}).Flex(1),
			ui.VScroll(pages...).Gap(16).Padding(16).Align(geom.Alignment{X: 0.5}).
				Background(ui.ColorBackground).Flex(1.3),
		).Gap(12).Flex(1),
	).Gap(12).Padding(16).Background(ui.ColorSurface))
}

// typed edits the source at the caret for a key typed on a page. Typed
// characters are escaped where Typst would read them as markup, so that the
// page shows what was typed.
func typed(e gift.Event) bool {
	if caret < 0 {
		return false
	}
	src := ed.Text()
	switch {
	case e.Kind == gift.EventRune:
		s := string(e.Rune)
		if strings.ContainsRune(`\#$*_`+"`"+`@<>[]~=-+/`, e.Rune) {
			s = `\` + s
		}
		edit(src[:caret]+s+src[caret:], caret+len(s))
	case e.Kind == gift.EventKeyDown && e.Key == gift.KeyEnter:
		edit(src[:caret]+"\n\n"+src[caret:], caret+2)
	case e.Kind == gift.EventKeyDown && e.Key == gift.KeyBackspace && caret > 0:
		_, n := utf8.DecodeLastRuneInString(src[:caret])
		edit(src[:caret-n]+src[caret:], caret-n)
	case e.Kind == gift.EventKeyDown && (e.Key == gift.KeyLeft || e.Key == gift.KeyRight):
		caret = step(caret, e.Key == gift.KeyRight)
		ed.SetCaret(caret)
		rev.Set(rev.Get() + 1)
	default:
		return false
	}
	return true
}

func edit(src string, at int) {
	ed.SetText(src)
	ed.SetCaret(at)
	caret = at
	submit(src)
}

// step moves the caret to the next or the previous offset a caret can be
// drawn at, skipping markup the page does not show.
func step(off int, forward bool) int {
	if doc == nil {
		return off
	}
	src := ed.Text()
	for {
		var n int
		if forward {
			if off >= len(src) {
				return off
			}
			_, n = utf8.DecodeRuneInString(src[off:])
		} else {
			if off <= 0 {
				return off
			}
			_, n = utf8.DecodeLastRuneInString(src[:off])
			n = -n
		}
		off += n
		if _, ok := doc.CaretAt(off); ok {
			return off
		}
	}
}

func savePDF() {
	pdf, err := engine.PDF()
	if err == nil {
		err = os.WriteFile("example-typst.pdf", pdf, 0o644)
	}
	if err != nil {
		problem = err.Error()
	} else {
		status = fmt.Sprintf("example-typst.pdf geschrieben, %d KiB", len(pdf)/1024)
	}
	rev.Set(rev.Get() + 1)
}
