package typstview

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/syntax"
)

// The editing of a document, as the pages of an [Editor] do it and as a
// program does it with the same methods, from a toolbar of its own say.
//
// Every edit is an edit of the source, made so that the source stays a
// document Typst sets: typed characters Typst would read as markup are
// escaped, a deletion removes what the pages show and keeps the markup around
// it, and formatting wraps text in markup or takes the markup away again.

// edited publishes an edit made through the document.
func (d *Document) edited() {
	if d.onChange != nil {
		d.onChange(d.src)
	}
}

// Insert types s at the caret, replacing what the selection shows, as typing
// on the page does: characters Typst would read as markup are escaped, so
// that the page shows what was typed. A line break in s is a line break of
// the source, which Typst sets as a space, and a blank line a paragraph.
func (d *Document) Insert(s string) {
	if s == "" {
		return
	}
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
	}
	at := d.caret
	d.typing = true
	d.replace(at, at, escape(d.src, at, s), true)
	d.typing = false
	d.edited()
}

// escape makes s safe to insert into src at at.
func escape(src string, at int, s string) string {
	switch contextAt(src, at) {
	case ui.TokenString:
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
		return r.Replace(s)
	case ui.TokenRaw:
		return strings.ReplaceAll(s, "`", "'")
	}
	var b strings.Builder
	prev, _ := utf8.DecodeLastRuneInString(src[:at])
	lineStart := strings.TrimLeft(src[lineStartOf(src, at):at], " \t") == ""
	for _, r := range s {
		switch {
		case strings.ContainsRune(`\#$*_`+"`"+`@<>[]~`, r):
			b.WriteByte('\\')
		// These are markup only at the start of a line, a heading or a
		// list item, or doubled, a dash or a comment.
		case strings.ContainsRune(`=-+/`, r) && (lineStart || prev == r):
			b.WriteByte('\\')
		}
		b.WriteRune(r)
		if r == '\n' {
			lineStart = true
		} else if r != ' ' && r != '\t' {
			lineStart = false
		}
		prev = r
	}
	return b.String()
}

func lineStartOf(src string, at int) int { return strings.LastIndexByte(src[:at], '\n') + 1 }

// contextAt is the kind of token at at: a string or raw text in which
// characters are escaped differently, or anything else.
func contextAt(src string, at int) ui.TokenKind {
	for _, t := range syntax.Typst.Highlight(src, nil) {
		if t.Start < at && at < t.End {
			return t.Kind
		}
		if t.Start >= at {
			break
		}
	}
	return ui.TokenPlain
}

// NewParagraph starts a new paragraph at the caret, as enter does on the
// page. In a list it starts the next item, and in an item without text it
// ends the list.
func (d *Document) NewParagraph() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
	}
	src, at := d.src, d.caret
	ls := lineStartOf(src, at)
	if m := marker(src[ls:]); m == "- " || m == "+ " {
		if le := lineEnd(src, at); strings.TrimSpace(src[ls+len(m):le]) == "" {
			// A blank line ends the list, and what is typed next is a
			// paragraph of its own.
			d.replace(ls, le, "\n", true)
		} else {
			d.replace(at, at, "\n"+m, true)
		}
		d.edited()
		return
	}
	d.replace(at, at, "\n\n", true)
	d.edited()
}

// marker is the heading or list marker a line starts with, with the space
// after it, or empty.
func marker(line string) string {
	i := 0
	for i < len(line) && line[i] == '=' {
		i++
	}
	if i == 0 && len(line) > 0 && (line[0] == '-' || line[0] == '+') {
		i = 1
	}
	if i == 0 || i >= len(line) || line[i] != ' ' {
		return ""
	}
	return line[:i+1]
}

// DeleteBackward deletes what the selection shows, or the character the page
// shows before the caret, as backspace does on the page. At the start of a
// heading or a list item it turns the line into a plain paragraph, and at
// the start of a paragraph it joins it to the one before.
func (d *Document) DeleteBackward() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
		d.edited()
		return
	}
	src, at := d.src, d.caret
	ls := lineStartOf(src, at)
	if m := marker(src[ls:]); m != "" && ls+len(m) == at {
		d.replace(ls, at, "", false)
		d.edited()
		return
	}
	g, ok := d.idx().before(at)
	if !ok {
		return
	}
	if gap := src[g[1]:at]; strings.TrimSpace(gap) == "" && strings.Contains(gap, "\n") {
		d.replace(g[1], at, "", true)
	} else {
		_, n := utf8.DecodeLastRuneInString(src[g[0]:g[1]])
		d.replace(escaped(src, g[1]-n), g[1], "", true)
	}
	d.edited()
}

// escaped is r, or the offset of the backslash before r that escapes it: a
// character typed as markup is deleted with its escape.
func escaped(src string, r int) int {
	if r > 0 && src[r-1] == '\\' && (r < 2 || src[r-2] != '\\') {
		return r - 1
	}
	return r
}

// DeleteForward deletes what the selection shows, or the character the page
// shows after the caret, as delete does on the page. At the end of a
// paragraph it joins the next one to it.
func (d *Document) DeleteForward() {
	if lo, hi := d.Selection(); lo != hi {
		d.deleteShown(lo, hi)
		d.edited()
		return
	}
	src, at := d.src, d.caret
	g, ok := d.idx().after(at)
	if !ok {
		return
	}
	if gap := src[at:g[0]]; strings.TrimSpace(gap) == "" && strings.Contains(gap, "\n") {
		d.replace(at, g[0], "", false)
	} else {
		_, n := utf8.DecodeRuneInString(src[g[0]:g[1]])
		lo := escaped(src, g[0])
		d.replace(lo, g[0]+n, "", false)
		d.caret, d.anchor = min(at, lo), min(at, lo)
	}
	d.edited()
}

// deleteShown deletes the text the pages show between lo and hi, and the
// paragraph breaks between it, and keeps the markup: deleting a selection
// that starts inside strong text and ends outside it must not delete the
// star that closes it.
func (d *Document) deleteShown(lo, hi int) {
	src := d.src
	gone := make([]bool, hi-lo)
	for _, r := range d.idx().shown(lo, hi) {
		for i := r[0]; i < r[1]; i++ {
			gone[i-lo] = true
		}
	}
	// White space with a line break in it, between shown text, is a
	// paragraph or a line break.
	for i := lo; i < hi; {
		j := i
		for j < hi && !gone[j-lo] && strings.IndexByte(" \t\n", src[j]) >= 0 {
			j++
		}
		if j > i && strings.Contains(src[i:j], "\n") {
			for k := i; k < j; k++ {
				gone[k-lo] = true
			}
		}
		i = max(j, i+1)
	}
	var kept strings.Builder
	for i := lo; i < hi; i++ {
		if !gone[i-lo] {
			kept.WriteByte(src[i])
		}
	}
	d.replace(lo, hi, kept.String(), false)
	d.caret, d.anchor = lo, lo
}

// SelectedText returns the text the selection shows, without the markup.
func (d *Document) SelectedText() string {
	lo, hi := d.Selection()
	var b strings.Builder
	for _, r := range d.idx().shown(lo, hi) {
		if b.Len() > 0 && strings.Contains(d.src[lo:r[0]], "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(d.src[r[0]:r[1]])
		lo = r[1]
	}
	return b.String()
}

// ToggleStrong makes the selection, or the word at the caret, strong, or
// plain again if it is strong already.
func (d *Document) ToggleStrong() { d.toggle("*", "strong") }

// ToggleEmphasis makes the selection, or the word at the caret, emphasised,
// or plain again if it is emphasised already.
func (d *Document) ToggleEmphasis() { d.toggle("_", "emph") }

// toggle wraps the selection in delim, or in a call of fn where delim would
// not work, inside a word, or unwraps it.
func (d *Document) toggle(delim, fn string) {
	src := d.src
	lo, hi := d.Selection()
	if lo == hi {
		lo, hi = wordAt(src, d.caret)
	}
	call := "#" + fn + "["
	switch {
	case lo >= len(delim) && hi+len(delim) <= len(src) && src[lo-len(delim):lo] == delim && src[hi:hi+len(delim)] == delim:
		d.replace(hi, hi+len(delim), "", false)
		d.replace(lo-len(delim), lo, "", false)
	case lo >= len(call) && hi < len(src) && src[lo-len(call):lo] == call && src[hi] == ']':
		d.replace(hi, hi+1, "", false)
		d.replace(lo-len(call), lo, "", false)
	case lo == hi:
		d.replace(lo, lo, delim+delim, true)
		d.caret, d.anchor = lo+len(delim), lo+len(delim)
	case strings.Contains(src[lo:hi], "\n\n"):
		// Strong text does not span paragraphs.
		return
	default:
		before, _ := utf8.DecodeLastRuneInString(src[:lo])
		after, _ := utf8.DecodeRuneInString(src[hi:])
		open, closing := delim, delim
		if isWordRune(before) || isWordRune(after) {
			open, closing = call, "]"
		}
		d.replace(hi, hi, closing, false)
		d.replace(lo, lo, open, false)
		d.anchor, d.caret = lo+len(open), hi+len(open)
	}
	d.edited()
	d.changed()
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// wordAt is the word around off, or the empty range at off.
func wordAt(s string, off int) (int, int) {
	lo, hi := off, off
	for lo > 0 {
		r, n := utf8.DecodeLastRuneInString(s[:lo])
		if !isWordRune(r) {
			break
		}
		lo -= n
	}
	for hi < len(s) {
		r, n := utf8.DecodeRuneInString(s[hi:])
		if !isWordRune(r) {
			break
		}
		hi += n
	}
	return lo, hi
}

// SetHeading makes the line of the caret a heading of level 1 to 6, or a
// plain paragraph for level 0.
func (d *Document) SetHeading(level int) {
	level = max(0, min(level, 6))
	d.setMarker(strings.Repeat("=", level) + " "[:min(level, 1)])
}

// ToggleList makes the line of the caret an item of a bulleted list, or of
// a numbered one, or a plain paragraph if it is one already.
func (d *Document) ToggleList(numbered bool) {
	m := "- "
	if numbered {
		m = "+ "
	}
	if ls := lineStartOf(d.src, d.caret); marker(d.src[ls:]) == m {
		m = ""
	}
	d.setMarker(m)
}

// setMarker replaces the heading or list marker of the line of the caret.
func (d *Document) setMarker(m string) {
	ls := lineStartOf(d.src, d.caret)
	old := marker(d.src[ls:])
	if old == m {
		return
	}
	d.replace(ls, ls+len(old), m, false)
	d.edited()
}

// InsertTable inserts a table of the given number of columns and rows below
// the paragraph of the caret, with a header row, and puts the caret into its
// first cell.
func (d *Document) InsertTable(columns, rows int) {
	columns, rows = max(1, columns), max(1, rows)
	src := d.src
	at := len(src)
	if i := strings.Index(src[d.caret:], "\n\n"); i >= 0 {
		at = d.caret + i
	}
	var b strings.Builder
	b.WriteString("\n\n#table(\n  columns: ")
	b.WriteString(strconv.Itoa(columns))
	b.WriteString(",\n  table.header(")
	first := 0
	for c := range columns {
		if c > 0 {
			b.WriteString(", ")
		}
		b.WriteString("[*")
		if c == 0 {
			first = b.Len()
		}
		b.WriteString("Column " + strconv.Itoa(c+1))
		b.WriteString("*]")
	}
	b.WriteString("),\n")
	for range rows {
		b.WriteString("  ")
		for c := range columns {
			if c > 0 {
				b.WriteString(", ")
			}
			b.WriteString("[Text]")
		}
		b.WriteString(",\n")
	}
	b.WriteString(")")
	d.replace(at, at, b.String(), false)
	d.caret, d.anchor = at+first, at+first
	d.edited()
}

// move moves the caret, as an arrow key does: left and right to the next
// place a caret can stand at, up and down to the line above or below, on
// the next page past the last line of one.
func (d *Document) move(dx, dy int, extend bool) {
	ix := d.idx()
	off := d.caret
	switch {
	case dx != 0:
		if lo, hi := d.Selection(); lo != hi && !extend {
			off = lo
			if dx > 0 {
				off = hi
			}
			break
		}
		off = ix.step(off, dx > 0)
		d.hasGoal = false
	case dy != 0:
		g, x, ok := ix.caret(off)
		if !ok {
			return
		}
		if !d.hasGoal {
			d.goalX, d.hasGoal = x, true
		}
		li := g.line + dy
		if li < 0 || li >= len(ix.lines) {
			return
		}
		off = ix.inLine(li, d.goalX)
	}
	d.caret = off
	if !extend {
		d.anchor = off
	}
	d.changed()
}
