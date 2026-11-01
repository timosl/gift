package typstview

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/typst"
	_ "github.com/worldiety/gift/typst/embedded"
)

// The Inter font is registered by its package, and a document takes every
// registered font.
var _ = inter.Family

const font = "#set text(font: \"Inter\")\n"

// doc returns a document of src that has been set, or skips the test where
// the library is not available.
func doc(t *testing.T, src string) *Document {
	t.Helper()
	if !typst.Available() {
		t.Skip("libgifttypst is not available")
	}
	d := NewDocument(font + src)
	t.Cleanup(d.Close)
	settle(t, d)
	return d
}

// settle waits until the last source has been set without an error.
func settle(t *testing.T, d *Document) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for d.Busy() {
		if time.Now().After(deadline) {
			t.Fatal("the document was not set in time")
		}
		time.Sleep(time.Millisecond)
	}
	if err := d.Err(); err != nil {
		t.Fatal(err)
	}
	if g := d.Diagnostics(); len(g) > 0 {
		t.Fatalf("the source %q does not set: %v", d.Source(), g)
	}
}

// at puts the caret before the n-th occurrence of s in the body of the
// source, or after it with after set.
func at(t *testing.T, d *Document, s string, after bool) int {
	t.Helper()
	i := strings.Index(d.Source()[len(font):], s)
	if i < 0 {
		t.Fatalf("%q is not in %q", s, d.Source())
	}
	off := len(font) + i
	if after {
		off += len(s)
	}
	d.SetSelection(off, off)
	return off
}

func body(d *Document) string { return strings.TrimPrefix(d.Source(), font) }

func TestTypedMarkupIsEscapedSoThatThePageShowsIt(t *testing.T) {
	d := doc(t, "Price")
	at(t, d, "Price", true)
	d.Insert(" #1 *net* <a> = $5 // -- x")
	settle(t, d)
	var shown strings.Builder
	for _, s := range d.idx().spans {
		shown.WriteString(d.src[s[0]:s[1]])
	}
	// The source of an escaped character is the escape.
	got := regexp.MustCompile(`\\(.)`).ReplaceAllString(shown.String(), "$1")
	if !strings.Contains(got, "#1") || !strings.Contains(got, "*net*") || !strings.Contains(got, "$5") {
		t.Fatalf("the page shows %q of %q", got, body(d))
	}
}

func TestInsideAStringTypedQuotesAreEscapedForTheString(t *testing.T) {
	d := doc(t, `#text(fill: blue)["x"] and #("say")`)
	at(t, d, `say`, true)
	d.Insert(`"hi"`)
	settle(t, d)
	if got := body(d); !strings.Contains(got, `say\"hi\"`) {
		t.Fatalf("%q", got)
	}
}

func TestBackspaceDeletesWhatThePageShowsAndKeepsTheMarkup(t *testing.T) {
	d := doc(t, "a *bold* b")
	at(t, d, "bold*", true)
	d.DeleteBackward()
	if got := body(d); got != "a *bol* b" {
		t.Fatalf("after backspace behind the star %q", got)
	}
	// Right after typing, before the page is set again, backspace deletes
	// what was typed.
	d.Insert("#x")
	d.DeleteBackward()
	d.DeleteBackward()
	if got := body(d); got != "a *bol* b" {
		t.Fatalf("after typing and two backspaces %q", got)
	}
	settle(t, d)
}

func TestASelectionOverMarkupDeletesOnlyItsText(t *testing.T) {
	d := doc(t, "a *bold* b\n\nnext")
	lo := at(t, d, "old", false)
	hi := at(t, d, "b\n", true) - 1
	d.SetSelection(lo, hi)
	d.DeleteBackward()
	if got := body(d); got != "a *b*\n\nnext" {
		t.Fatalf("%q", got)
	}
	settle(t, d)
}

func TestStrongWrapsAWordAndUnwrapsIt(t *testing.T) {
	d := doc(t, "one two")
	at(t, d, "tw", true)
	d.ToggleStrong()
	if got := body(d); got != "one *two*" {
		t.Fatalf("%q", got)
	}
	settle(t, d)
	d.ToggleStrong()
	if got := body(d); got != "one two" {
		t.Fatalf("%q", got)
	}
	// Inside a word a star would not work.
	lo := at(t, d, "tw", false)
	d.SetSelection(lo, lo+2)
	d.ToggleEmphasis()
	if got := body(d); got != "one #emph[tw]o" {
		t.Fatalf("%q", got)
	}
	settle(t, d)
}

func TestHeadingsListsAndParagraphs(t *testing.T) {
	d := doc(t, "Title\n\nitem")
	at(t, d, "Title", false)
	d.SetHeading(2)
	at(t, d, "item", true)
	d.ToggleList(false)
	if got := body(d); got != "== Title\n\n- item" {
		t.Fatalf("%q", got)
	}
	// Enter in an item starts the next, and in an empty one ends the list.
	d.NewParagraph()
	d.Insert("more")
	d.NewParagraph()
	d.NewParagraph()
	d.Insert("after")
	if got := body(d); got != "== Title\n\n- item\n- more\n\nafter" {
		t.Fatalf("%q", got)
	}
	settle(t, d)
	// Backspace at the start of a heading makes it a paragraph.
	at(t, d, "Title", false)
	d.DeleteBackward()
	if !strings.HasPrefix(body(d), "Title\n") {
		t.Fatalf("%q", body(d))
	}
	settle(t, d)
}

func TestATableIsInsertedBelowTheParagraphWithTheCaretInItsFirstCell(t *testing.T) {
	d := doc(t, "Before the table.\n\nAfter.")
	at(t, d, "Before", true)
	d.InsertTable(2, 1)
	settle(t, d)
	if !strings.HasPrefix(d.src[d.caret:], "Column 1") {
		t.Fatalf("the caret is before %q", d.src[d.caret:min(len(d.src), d.caret+12)])
	}
	if b := body(d); !strings.HasPrefix(b, "Before the table.\n\n#table(") || !strings.HasSuffix(b, ")\n\nAfter.") {
		t.Fatalf("%q", b)
	}
}

func TestArrowsMoveThroughTheTextThePageShows(t *testing.T) {
	d := doc(t, "ab *cd*\n\nef")
	off := at(t, d, "b", true)
	d.move(1, 0, false) // past the space
	d.move(1, 0, false) // past the star, onto the c
	if got := d.src[d.caret:]; !strings.HasPrefix(got, "d*") && !strings.HasPrefix(got, "cd*") {
		t.Fatalf("two steps right of %d land before %q", off, got)
	}
	d.move(0, 1, false)
	if got := d.src[d.caret:]; !strings.HasPrefix(got, "ef") && !strings.HasPrefix(got, "f") && got != "" {
		t.Fatalf("down lands before %q", got)
	}
}
