package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
)

var codeEditorType = gift.RegisterType("ui.CodeEditor")

// monospaceFamily is the family font/ibmplexmono registers. A code editor
// takes it when the program imports that package and sets no font of its
// own; ui cannot import the package, which imports ui.
const monospaceFamily = "IBM Plex Mono"

// CodeEditorView edits source code. It is created by [CodeEditor]; the zero
// value is not useful.
//
//	src := ui.Editor(ctx, "src", "package main\n")
//	ui.CodeEditor(src).Highlighter(syntax.Go).Marks(ui.CodeMark{Start: 8, End: 12})
//
// It is a [TextAreaView] over the same [TextEditor], with what a code editor
// adds: the numbers of the lines in a gutter on the left, the text coloured
// by a [Highlighter], and [CodeMark]s for the places a parser or a compiler
// complained about, whose lines are tinted and whose text is underlined.
// It sets its text in IBM Plex Mono when the program imports
// font/ibmplexmono, and in the default font otherwise.
//
// Selecting, scrolling and editing are the area's, with three keys that
// work as in a code editor: tab indents the line of the caret, or every line
// of a selection, and shift-tab takes the indentation away again; enter
// starts the new line at the indentation of the old one. Tab therefore does
// not move the focus on. Escape and then tab does, so that the keyboard does
// not get stuck in an editor.
//
// Long lines wrap at the width of the editor, as the area's do; a wrapped
// line has its number on its first row. There are no tab stops: a tab is as
// wide as one character, which is why the editor indents with spaces.
type CodeEditorView struct {
	area      TextAreaView
	hl        Highlighter
	marks     []CodeMark
	noNumbers bool
	indent    string
}

// CodeMark marks a stretch of the text of a [CodeEditorView], such as where
// a parser found an error: the lines it touches are tinted and the stretch
// itself is underlined. Start and End are byte offsets into the text; a mark
// with End equal to Start marks a place, where something is missing, and is
// underlined over the width of one character.
type CodeMark struct {
	Start, End int
	// Color tints the lines and underlines the text; the zero Color is
	// [ColorDanger].
	Color Color
}

// CodeEditor returns an editor of the source code in ed, with line numbers,
// without a highlighter and indenting by four spaces. A nil editor panics,
// as in [TextArea].
func CodeEditor(ed *TextEditor) CodeEditorView {
	return CodeEditorView{area: TextArea(ed).FontSize(13), indent: "    "}
}

// ViewType implements gift.View.
func (v CodeEditorView) ViewType() gift.TypeID { return codeEditorType }

// Build implements gift.View: the element of the area, with the state of the
// code editor attached to its node.
func (v CodeEditorView) Build(bc *gift.BuildContext) gift.Element {
	a := v.area
	if a.font.IsZero() {
		if f, ok := ResolveFont(FontQuery{Family: monospaceFamily}); ok {
			a.font = f
		}
	}
	el := a.Build(bc)
	n := el.Layouter.(*textAreaNode)
	c := &codeState{hl: v.hl, numbers: !v.noNumbers, indent: v.indent, base: n.pad.Left}
	c.colors = lightCodeColors
	if CurrentTheme().IsDark() {
		c.colors = darkCodeColors
	}
	c.colors[TokenPlain] = n.fg
	c.numberFG = ResolveColor(Fade(ColorLabel, 0.35))
	c.currentFG = ResolveColor(ColorSecondaryLabel)
	c.edge = ResolveColor(ColorSeparator)
	danger := ResolveColor(ColorDanger)
	for _, m := range v.marks {
		if m.Color == (Color{}) {
			m.Color = danger
		}
		m.Color = ResolveColor(m.Color)
		c.marks = append(c.marks, m)
	}
	n.code = c
	return el
}

// Highlighter sets what colours the text; nil leaves it in one colour.
func (v CodeEditorView) Highlighter(h Highlighter) CodeEditorView { v.hl = h; return v }

// Marks sets the stretches of text that are marked; see [CodeMark]. Marks
// are not moved by edits: the program that set them computes them anew from
// the text it is given, as a parser does.
func (v CodeEditorView) Marks(m ...CodeMark) CodeEditorView { v.marks = m; return v }

// LineNumbers shows or hides the gutter with the numbers of the lines. It is
// shown unless turned off.
func (v CodeEditorView) LineNumbers(b bool) CodeEditorView { v.noNumbers = !b; return v }

// Indent sets what tab inserts and shift-tab removes at the start of a line,
// four spaces unless set. An empty string keeps four spaces.
func (v CodeEditorView) Indent(s string) CodeEditorView {
	if s != "" {
		v.indent = s
	}
	return v
}

// Lines sets the height in lines; see [TextAreaView.Lines].
func (v CodeEditorView) Lines(min, max int) CodeEditorView { v.area = v.area.Lines(min, max); return v }

// Placeholder sets the text shown while the editor is empty.
func (v CodeEditorView) Placeholder(s string) CodeEditorView {
	v.area = v.area.Placeholder(s)
	return v
}

// Label sets the accessible name.
func (v CodeEditorView) Label(s string) CodeEditorView { v.area = v.area.Label(s); return v }

// OnChange sets the function called with the text after every edit.
func (v CodeEditorView) OnChange(f func(string)) CodeEditorView {
	v.area = v.area.OnChange(f)
	return v
}

// Bind reads the text from b and writes every edit back; see
// [TextAreaView.Bind].
func (v CodeEditorView) Bind(b gift.Binding[string]) CodeEditorView {
	v.area = v.area.Bind(b)
	return v
}

// Disabled takes the editor out of input.
func (v CodeEditorView) Disabled(b bool) CodeEditorView { v.area = v.area.Disabled(b); return v }

// Font sets the font of the text, instead of the monospaced one.
func (v CodeEditorView) Font(f Font) CodeEditorView { v.area = v.area.Font(f); return v }

// FontSize sets the size of the text, 13 unless set.
func (v CodeEditorView) FontSize(s float32) CodeEditorView { v.area = v.area.FontSize(s); return v }

// Foreground sets the colour of plain text.
func (v CodeEditorView) Foreground(c Color) CodeEditorView { v.area = v.area.Foreground(c); return v }

// Padding sets the room between the edge and the text on every side; the
// gutter comes on top of it on the left.
func (v CodeEditorView) Padding(f float32) CodeEditorView { v.area = v.area.Padding(f); return v }

// PaddingInsets sets the room between the edge and the text per side; the
// gutter comes on top of it on the left.
func (v CodeEditorView) PaddingInsets(i geom.Insets) CodeEditorView {
	v.area = v.area.PaddingInsets(i)
	return v
}

// Frame fixes the size; see [BoxView.Frame].
func (v CodeEditorView) Frame(w, h float32) CodeEditorView { v.area = v.area.Frame(w, h); return v }

// MinWidth sets the smallest width.
func (v CodeEditorView) MinWidth(f float32) CodeEditorView { v.area = v.area.MinWidth(f); return v }

// MinHeight sets the smallest height.
func (v CodeEditorView) MinHeight(f float32) CodeEditorView { v.area = v.area.MinHeight(f); return v }

// MaxWidth sets the largest width.
func (v CodeEditorView) MaxWidth(f float32) CodeEditorView { v.area = v.area.MaxWidth(f); return v }

// MaxHeight sets the largest height, past which the lines scroll.
func (v CodeEditorView) MaxHeight(f float32) CodeEditorView { v.area = v.area.MaxHeight(f); return v }

// Background sets the face.
func (v CodeEditorView) Background(b Background) CodeEditorView {
	v.area = v.area.Background(b)
	return v
}

// Border sets the outline.
func (v CodeEditorView) Border(b Border) CodeEditorView { v.area = v.area.Border(b); return v }

// Shadow sets the shadow.
func (v CodeEditorView) Shadow(sh Shadow) CodeEditorView { v.area = v.area.Shadow(sh); return v }

// Clip panics for false, as [TextAreaView.Clip] does: the lines scroll
// inside the editor and are cut at its edge.
func (v CodeEditorView) Clip(b bool) CodeEditorView { v.area = v.area.Clip(b); return v }

// CornerRadius sets the radius of the corners.
func (v CodeEditorView) CornerRadius(f float32) CodeEditorView {
	v.area = v.area.CornerRadius(f)
	return v
}

// Key sets the reconciliation key.
func (v CodeEditorView) Key(s string) CodeEditorView { v.area = v.area.Key(s); return v }

// Flex sets the flex factor in a stack.
func (v CodeEditorView) Flex(f float32) CodeEditorView { v.area = v.area.Flex(f); return v }

// The colours of the token kinds, after the default themes of Xcode, whose
// light and dark sets are made to be read beside each other. Plain text is
// the foreground of the editor and is filled in at build.
var (
	lightCodeColors = [tokenKinds]Color{
		TokenKeyword:  RGB(0xAD, 0x3D, 0xA4),
		TokenString:   RGB(0xC4, 0x1A, 0x16),
		TokenNumber:   RGB(0x27, 0x2A, 0xD8),
		TokenComment:  RGB(0x5D, 0x6C, 0x79),
		TokenFunction: RGB(0x32, 0x6D, 0x74),
		TokenType:     RGB(0x0B, 0x4F, 0x79),
		TokenHeading:  RGB(0x0F, 0x68, 0xA0),
		TokenEmphasis: RGB(0x80, 0x4F, 0xB8),
		TokenMarkup:   RGB(0x78, 0x49, 0x2A),
		TokenRaw:      RGB(0x64, 0x38, 0x20),
		TokenMath:     RGB(0x1C, 0x46, 0x4A),
	}
	darkCodeColors = [tokenKinds]Color{
		TokenKeyword:  RGB(0xFF, 0x7A, 0xB2),
		TokenString:   RGB(0xFF, 0x81, 0x70),
		TokenNumber:   RGB(0xD9, 0xC9, 0x7C),
		TokenComment:  RGB(0x7F, 0x8C, 0x98),
		TokenFunction: RGB(0x67, 0xB7, 0xA4),
		TokenType:     RGB(0xDA, 0xBA, 0xFF),
		TokenHeading:  RGB(0x6B, 0xDF, 0xFF),
		TokenEmphasis: RGB(0xB2, 0x81, 0xEB),
		TokenMarkup:   RGB(0xFD, 0x8F, 0x3F),
		TokenRaw:      RGB(0xAC, 0xF2, 0xE4),
		TokenMath:     RGB(0x4E, 0xB0, 0xCC),
	}
)

// The room in the gutter left and right of the numbers.
const (
	gutterLeft  = float32(10)
	gutterRight = float32(8)
)

// codeState is what a [CodeEditorView] adds to the node of its area. It is
// made anew by every build, with the tokens of the text cached for as long
// as the text does not change.
type codeState struct {
	hl      Highlighter
	marks   []CodeMark
	numbers bool
	indent  string

	colors                    [tokenKinds]Color
	numberFG, currentFG, edge Color

	// base is the left padding of the area without the gutter, and gutter
	// the width of the gutter at the last layout.
	base, gutter float32

	// escaped is set by escape and lets the next tab move the focus.
	escaped bool

	// src is the text tokens were made from.
	src    string
	hlOK   bool
	tokens []Token
}

// layout makes room for the gutter: as wide as the number of the last line,
// in at least two digits, so that the text does not jump at line ten.
func (c *codeState) layout(n *textAreaNode) {
	n.pad.Left = c.base
	c.gutter = 0
	if !c.numbers {
		return
	}
	lines := strings.Count(n.ed.Text(), "\n") + 1
	digits := max(2, len(strconv.Itoa(lines)))
	w := numberPara(n, strings.Repeat("0", digits)).Lines[0].Advance
	c.gutter = roundf(gutterLeft + w + gutterRight)
	n.pad.Left += c.gutter
}

// numberPara shapes the number s in the font of the text, so that it shares
// the line height and the baseline of the line it stands beside.
func numberPara(n *textAreaNode, s string) *text.Paragraph {
	return text.Default().Layout(text.Request{Text: s, Font: n.req.Font, Size: n.req.Size, MaxWidth: geom.Unbounded()})
}

// tokensOf returns the tokens of src, highlighting it only when it changed.
func (c *codeState) tokensOf(src string) []Token {
	if c.hl == nil {
		return nil
	}
	if !c.hlOK || src != c.src {
		c.tokens = c.hl.Highlight(src, c.tokens[:0])
		c.src, c.hlOK = src, true
	}
	return c.tokens
}

// lineSpan is the logical line, from its first byte to the break after it,
// that holds the offsets lo to hi.
func lineSpan(s string, lo, hi int) (int, int) {
	lo, hi = max(0, min(lo, len(s))), max(0, min(hi, len(s)))
	start := strings.LastIndexByte(s[:lo], '\n') + 1
	end := len(s)
	if i := strings.IndexByte(s[hi:], '\n'); i >= 0 {
		end = hi + i
	}
	return start, end
}

// paintUnder draws what lies under the text: the tint of the marked lines
// and the gutter with its numbers, inside the content box vertically, so
// that they scroll with the lines.
func (c *codeState) paintUnder(ctx *gift.PaintContext, n *textAreaNode, b, box geom.Rect) {
	p := n.para()
	if len(p.Lines) == 0 {
		return
	}
	src := n.ed.Text()
	lh := p.Metrics.LineHeight
	y := roundf(b.Min.Y + n.pad.Top - n.ed.scrollY)
	ctx.PushClip(deviceRect(ctx, geom.Rc(b.Min.X, box.Min.Y, b.Max.X, box.Max.Y)))
	defer ctx.PopClip()

	left := b.Min.X + c.gutter
	for _, m := range c.marks {
		lo, hi := lineSpan(src, m.Start, max(m.Start, m.End))
		tint := Fade(m.Color, 0.14)
		for i := range p.Lines {
			ln := &p.Lines[i]
			if ln.End < lo || ln.Start > hi {
				continue
			}
			top := y + float32(i)*lh
			if top+lh < box.Min.Y || top > box.Max.Y {
				continue
			}
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(left+1, top, b.Max.X-1, top+lh), Color: tint})
		}
	}

	if !c.numbers {
		return
	}
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(left, b.Min.Y+1, left+1, b.Max.Y-1), Color: c.edge})
	caretLine := strings.Count(src[:min(n.ed.caret, len(src))], "\n")
	logical := 0
	for i := range p.Lines {
		// A row that does not follow a line break is a wrapped row of the
		// line above. (The ends of empty lines are not what [hardAfter]
		// expects, so the text is asked.)
		if st := p.Lines[i].Start; i > 0 && (st == 0 || st > len(src) || src[st-1] != '\n') {
			continue
		}
		if i > 0 {
			logical++
		}
		top := y + float32(i)*lh
		if top+lh < box.Min.Y || top > box.Max.Y {
			continue
		}
		fg := c.numberFG
		if logical == caretLine {
			fg = c.currentFG
		}
		lo, hi := lineSpan(src, p.Lines[i].Start, p.Lines[i].Start)
		for _, m := range c.marks {
			if a, z := lineSpan(src, m.Start, max(m.Start, m.End)); a <= hi && z >= lo {
				fg = m.Color
			}
		}
		num := numberPara(n, strconv.Itoa(logical+1))
		x := roundf(left - gutterRight - num.Lines[0].Advance)
		paintLines(ctx, b, num, fg, x, top)
	}
}

// paintText draws the lines of p as [paintLines] does, in the colour of the
// token each glyph belongs to: one operation per run of glyphs of one
// colour.
func (c *codeState) paintText(ctx *gift.PaintContext, n *textAreaNode, b geom.Rect, p *text.Paragraph, fg Color, x, y float32) {
	src := n.ed.Text()
	toks := c.tokensOf(src)
	colors := c.colors
	colors[TokenPlain] = fg
	lh := p.Metrics.LineHeight
	first, cur := ctx.GlyphsLen(), fg
	flush := func() {
		if count := ctx.GlyphsLen() - first; count > 0 && !cur.IsTransparent() {
			ctx.Add(render.Op{Kind: render.OpGlyphs, Bounds: b, Color: cur, Glyphs: first, GlyphCount: count})
		}
		first = ctx.GlyphsLen()
	}
	ti, last := 0, -1
	for i := range p.Lines {
		ln := &p.Lines[i]
		top := y + float32(i)*lh
		if top+lh < b.Min.Y || top > b.Max.Y {
			continue
		}
		baseline := y + ln.Baseline
		for ri := range ln.Runs {
			run := &ln.Runs[ri]
			id := render.FontID(run.Font.ID())
			for gi := range run.Glyphs {
				g := &run.Glyphs[gi]
				// Glyphs come in the order of the text, except in a
				// right-to-left run; there the token is searched for.
				cl := int(g.Cluster)
				if cl < last {
					ti = sort.Search(len(toks), func(k int) bool { return toks[k].End > cl })
				}
				last = cl
				for ti < len(toks) && toks[ti].End <= cl {
					ti++
				}
				col := fg
				if ti < len(toks) && toks[ti].Start <= cl {
					col = colors[toks[ti].Kind]
				}
				if col != cur {
					flush()
					cur = col
				}
				// The shaper has no glyph for a tab and would draw a box;
				// it stays empty, one character wide.
				if cl < len(src) && src[cl] == '\t' {
					continue
				}
				ctx.AppendGlyph(render.Glyph{Font: id, ID: render.GlyphID(g.ID), Size: run.Size, X: x + g.X, Y: baseline + g.Y})
			}
		}
	}
	flush()
}

// paintOver underlines the text of the marks, below the baseline of each row
// it covers.
func (c *codeState) paintOver(ctx *gift.PaintContext, n *textAreaNode, p *text.Paragraph, x, y float32) {
	if len(p.Lines) == 0 {
		return
	}
	lh := p.Metrics.LineHeight
	thick := max(1, roundf(n.req.Size/10))
	for _, m := range c.marks {
		a, z := max(0, m.Start), max(m.Start, m.End)
		for i := range p.Lines {
			ln := &p.Lines[i]
			lo, hi := max(a, ln.Start), min(z, ln.End)
			if lo > hi || (lo == hi && !(a == z && a >= ln.Start && a <= ln.End)) {
				continue
			}
			x0, x1 := areaX(p, i, lo), areaX(p, i, hi)
			if x1-x0 < n.req.Size/2 {
				x1 = x0 + n.req.Size/2
			}
			// Just below the baseline, and above the next row.
			uy := min(roundf(y+ln.Baseline+1), roundf(y+float32(i+1)*lh)-thick)
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x+x0, uy, x+x1, uy+thick), Color: m.Color})
			if a == z {
				break
			}
		}
	}
}

// key handles the keys a code editor adds to an area. It reports whether
// it decided the key, and if so whether the key was used.
func (c *codeState) key(ctx *gift.EventContext, n *textAreaNode, e gift.Event) (decided, used bool) {
	escaped := c.escaped
	c.escaped = false
	if e.Mods.Has(gift.ShortcutModifier) || e.Mods.Has(gift.ModAlt) {
		return false, false
	}
	switch e.Key {
	case gift.KeyEscape:
		c.escaped = true
		return true, false
	case gift.KeyTab:
		if escaped {
			return true, false
		}
		c.indentLines(n.ed, !e.Mods.Has(gift.ModShift))
		n.afterEdit(ctx, e.Time)
		return true, true
	case gift.KeyEnter:
		src := n.ed.Text()
		lo, _ := n.ed.selection()
		start, _ := lineSpan(src, lo, lo)
		ws := start
		for ws < lo && (src[ws] == ' ' || src[ws] == '\t') {
			ws++
		}
		n.ed.Insert("\n" + src[start:ws])
		n.afterEdit(ctx, e.Time)
		return true, true
	}
	return false, false
}

// indentLines indents the line of the caret, or every line the selection
// touches, by one step, or takes one step of indentation away. Without a
// selection, indenting inserts the step at the caret, as a tab key does in
// the middle of a line.
func (c *codeState) indentLines(ed *TextEditor, in bool) {
	src := ed.Text()
	lo, hi := ed.selection()
	if in && lo == hi {
		ed.Insert(c.indent)
		return
	}
	// A selection that ends at the start of a line does not take that line
	// along.
	if hi > lo && hi > 0 && src[hi-1] == '\n' {
		hi--
	}
	start, end := lineSpan(src, lo, hi)

	// The edits are made at the starts of the lines, in the offsets of the
	// old text: ins bytes inserted or del bytes removed at at.
	type edit struct{ at, ins, del int }
	var edits []edit
	var b strings.Builder
	b.WriteString(src[:start])
	for ls := start; ; {
		le := end
		if i := strings.IndexByte(src[ls:end], '\n'); i >= 0 {
			le = ls + i
		}
		line := src[ls:le]
		if in {
			b.WriteString(c.indent)
			edits = append(edits, edit{at: ls, ins: len(c.indent)})
		} else {
			// One step is as many spaces as the indent has, or one tab.
			k := 0
			for k < len(line) && k < len(c.indent) && line[k] == ' ' {
				k++
			}
			if k == 0 && len(line) > 0 && line[0] == '\t' {
				k = 1
			}
			line = line[k:]
			edits = append(edits, edit{at: ls, del: k})
		}
		b.WriteString(line)
		if le >= end {
			break
		}
		b.WriteByte('\n')
		ls = le + 1
	}
	b.WriteString(src[end:])

	// moved is where an offset of the old text is in the new one. An offset
	// at the start of a line moves with the indent inserted there, except
	// the start of the selection, which keeps the indent inside it.
	moved := func(o int) int {
		d := 0
		for _, e := range edits {
			switch {
			case e.ins > 0 && (o > e.at || (o == e.at && o != lo)):
				d += e.ins
			case e.del > 0 && o >= e.at+e.del:
				d -= e.del
			case e.del > 0 && o > e.at:
				d -= o - e.at
			}
		}
		return o + d
	}
	anchor, caret := moved(ed.anchor), moved(ed.caret)
	ed.replace(0, len(src), b.String())
	ed.anchor, ed.caret = ed.snap(anchor), ed.snap(caret)
}
