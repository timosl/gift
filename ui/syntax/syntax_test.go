package syntax_test

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/syntax"
)

// typstSample is a short document that uses most of what [syntax.Typst]
// recognises, in the shape of a real one.
const typstSample = `#set page(width: 420pt, margin: 36pt, numbering: "1")
#set text(font: "Inter", size: 10.5pt, lang: "de")
= Angebot 2026-117
Sehr geehrte *Damen* und _Herren_, siehe @tab und <x>. // Kommentar
- Punkt eins
+ Nummer
#table(
  columns: (auto, 1fr),
  fill: (_, y) => if y == 0 { rgb("#e8eef7") },
  table.header([*Pos.*], [*Leistung*]),
  ..range(1, 61).map(i => ([#i], [Arbeitspaket #i])).flatten(),
)
$ a^2 + b^2 = c^2 $ und ` + "`raw`" + ` text.
#let x = 1 /* block */
`

// goSample is the same for [syntax.Go].
const goSample = "package main\n\nimport \"fmt\"\n\n" +
	"// Greet says hello.\n" +
	"func Greet(name string) error {\n" +
	"\tx := 0x1F + 3.5 + 2i\n" +
	"\tr := 'a'\n" +
	"\ts := `raw\nstring`\n" +
	"\tvar ok bool = true && nil == nil\n" +
	"\tfmt.Println(\"hi\", x, r, s, ok, len(name))\n" +
	"\treturn nil\n" +
	"}\n\n" +
	"func (t *T) Method(n int) {}\n\n" +
	"func Map[E any](xs []E) {}\n\n" +
	"const A = iota\n\n" +
	"type T struct {\n\tb byte\n\tf float64\n\tu uintptr\n\tc complex128\n}\n\n" +
	"/* block */\n"

// span is a stretch of a source that a test expects to be of one kind: sub
// inside the first occurrence of in, or all of in when sub is empty.
type span struct {
	in, sub string
	kind    ui.TokenKind
}

type highlightCase struct {
	name  string
	src   string
	spans []span
}

var typstCases = []highlightCase{
	{"sample", typstSample, []span{
		{"#set page", "#set", ui.TokenKeyword},
		{"page(", "page", ui.TokenFunction},
		{"width:", "", ui.TokenPlain},
		{"420pt", "", ui.TokenNumber},
		{`"1"`, "", ui.TokenString},
		{"10.5pt", "", ui.TokenNumber},
		{"= Angebot 2026-117", "", ui.TokenHeading},
		{"Sehr geehrte ", "", ui.TokenPlain},
		{"*Damen*", "", ui.TokenEmphasis},
		{" und ", "", ui.TokenPlain},
		{"_Herren_", "", ui.TokenEmphasis},
		{"@tab", "", ui.TokenMarkup},
		{"<x>", "", ui.TokenMarkup},
		{"// Kommentar", "", ui.TokenComment},
		{"- Punkt", "-", ui.TokenMarkup},
		{"Punkt eins", "", ui.TokenPlain},
		{"+ Nummer", "+", ui.TokenMarkup},
		{"#table", "", ui.TokenFunction},
		{"columns", "", ui.TokenPlain},
		{"(auto", "auto", ui.TokenKeyword},
		{"1fr", "", ui.TokenNumber},
		{"if y", "if", ui.TokenKeyword},
		{"y == 0", "y", ui.TokenPlain},
		{"y == 0", "0", ui.TokenNumber},
		{"rgb(", "rgb", ui.TokenFunction},
		{`"#e8eef7"`, "", ui.TokenString},
		{"table.header", "table", ui.TokenPlain},
		{"table.header", "header", ui.TokenFunction},
		{"[*Pos.*]", "*Pos.*", ui.TokenEmphasis},
		{"[*Leistung*]", "*Leistung*", ui.TokenEmphasis},
		{"range(", "range", ui.TokenFunction},
		{".map(", "map", ui.TokenFunction},
		{".flatten(", "flatten", ui.TokenFunction},
		{"Arbeitspaket", "", ui.TokenPlain},
		{"$ a^2 + b^2 = c^2 $", "", ui.TokenMath},
		{"`raw`", "", ui.TokenRaw},
		{" text.", "", ui.TokenPlain},
		{"#let", "", ui.TokenKeyword},
		{"x = 1", "x", ui.TokenPlain},
		{"x = 1", "1", ui.TokenNumber},
		{"/* block */", "", ui.TokenComment},
	}},
	{"nested block comments", "a /* x /* y */ z */ b // c\nd", []span{
		{"/* x /* y */ z */", "", ui.TokenComment},
		{"*/ b", "b", ui.TokenPlain},
		{"// c", "", ui.TokenComment},
		{"\nd", "d", ui.TokenPlain},
	}},
	{"a URL is not a comment", "see https://typst.app/docs for more", []span{
		{"see https://typst.app/docs for more", "", ui.TokenPlain},
	}},
	{"raw blocks", "```rust\nfn main() { \"*\" }\n```\nafter *x*", []span{
		{"```rust\nfn main() { \"*\" }\n```", "", ui.TokenRaw},
		{"after", "", ui.TokenPlain},
		{"*x*", "", ui.TokenEmphasis},
	}},
	{"raw blocks close with as many backticks", "````\n```\ninner\n```\n```` done", []span{
		{"````\n```\ninner\n```\n````", "", ui.TokenRaw},
		{" done", "", ui.TokenPlain},
	}},
	{"empty raw text", "`` x", []span{
		{"``", "", ui.TokenRaw},
		{" x", "", ui.TokenPlain},
	}},
	{"unclosed strong text ends at the paragraph", "*bold\nstill bold\n\nnext", []span{
		{"*bold\nstill bold", "", ui.TokenEmphasis},
		{"next", "", ui.TokenPlain},
	}},
	{"delimiters inside words are text", "snake_case_name and 2*3 stay", []span{
		{"snake_case_name and 2*3 stay", "", ui.TokenPlain},
	}},
	{"escapes", `\#not code \*not strong \u{1F600} \ end \é`, []span{
		{`\#`, "", ui.TokenMarkup},
		{"not code", "", ui.TokenPlain},
		{`\*`, "", ui.TokenMarkup},
		{"not strong", "", ui.TokenPlain},
		{`\u{1F600}`, "", ui.TokenMarkup},
		{`\ end`, `\`, ui.TokenMarkup},
		{`\ end`, ` end`, ui.TokenPlain},
		{`\é`, "", ui.TokenMarkup},
	}},
	{"if and else", "#if x > 1 [big] else [small] and more\n", []span{
		{"#if", "", ui.TokenKeyword},
		{"x > 1", "1", ui.TokenNumber},
		{"big", "", ui.TokenPlain},
		{"else", "", ui.TokenKeyword},
		{" and more", "", ui.TokenPlain},
	}},
	{"else if", "#if a {1} else if b {2} else [three] after", []span{
		{"} else if", "else", ui.TokenKeyword},
		{"else if", "if", ui.TokenKeyword},
		{"{2} else [", "else", ui.TokenKeyword},
		{"three", "", ui.TokenPlain},
		{" after", "", ui.TokenPlain},
	}},
	{"for", "#for x in range(3) [#x, ]\nText or not", []span{
		{"#for", "", ui.TokenKeyword},
		{" in ", "in", ui.TokenKeyword},
		{"range", "", ui.TokenFunction},
		{"Text or not", "", ui.TokenPlain},
	}},
	{"while with a code block", "#while n < 3 { n += 1 }\n= Next", []span{
		{"#while", "", ui.TokenKeyword},
		{"+= 1", "1", ui.TokenNumber},
		{"= Next", "", ui.TokenHeading},
	}},
	{"a condition ends with its line", "#if\n*b*", []span{
		{"#if", "", ui.TokenKeyword},
		{"*b*", "", ui.TokenEmphasis},
	}},
	{"terms and enumerations", "/ Term: description\n1. first\n12. twelfth\n2026 was a year", []span{
		{"/ Term:", "", ui.TokenMarkup},
		{" description", "", ui.TokenPlain},
		{"1.", "", ui.TokenMarkup},
		{"12.", "", ui.TokenMarkup},
		{"2026 was a year", "", ui.TokenPlain},
	}},
	{"markers only at the start of a line", "a - b + c / d: e = f 1. g", []span{
		{"a - b + c / d: e = f 1. g", "", ui.TokenPlain},
	}},
	{"indented markers", "  - item\n\t= Heading", []span{
		{"- item", "-", ui.TokenMarkup},
		{"= Heading", "", ui.TokenHeading},
	}},
	{"headings end at labels and comments", "== Sub *x* <sec>\n=== Third // note\n=no heading", []span{
		{"== Sub *x* ", "", ui.TokenHeading},
		{"<sec>", "", ui.TokenMarkup},
		{"=== Third ", "", ui.TokenHeading},
		{"// note", "", ui.TokenComment},
		{"=no heading", "", ui.TokenPlain},
	}},
	{"show and set", "#show heading: set text(blue)\nAfter", []span{
		{"#show", "", ui.TokenKeyword},
		{": set", "set", ui.TokenKeyword},
		{"text(", "text", ui.TokenFunction},
		{"blue", "", ui.TokenPlain},
		{"After", "", ui.TokenPlain},
	}},
	{"context", "#context text.lang is here", []span{
		{"#context", "", ui.TokenKeyword},
		{"text.lang", "", ui.TokenPlain},
		{" is here", "", ui.TokenPlain},
	}},
	{"context with content", "#context [*a*] b", []span{
		{"#context", "", ui.TokenKeyword},
		{"*a*", "", ui.TokenEmphasis},
		{" b", "", ui.TokenPlain},
	}},
	{"code blocks", "#{\n  let a = (1, 2)\n  a.len()\n}\n*after*", []span{
		{"let", "", ui.TokenKeyword},
		{"(1, 2)", "1", ui.TokenNumber},
		{"len", "", ui.TokenFunction},
		{"*after*", "", ui.TokenEmphasis},
	}},
	{"a content block starts a line", "#box[= Inside\n- item]", []span{
		{"#box", "", ui.TokenFunction},
		{"= Inside", "", ui.TokenHeading},
		{"- item", "-", ui.TokenMarkup},
		{"item", "", ui.TokenPlain},
	}},
	{"stray closers are skipped", "] ) } *b*", []span{
		{"] ) } ", "", ui.TokenPlain},
		{"*b*", "", ui.TokenEmphasis},
	}},
	{"a closer ends what was opened inside its bracket", "[#f(a ] *c*", []span{
		{"#f", "", ui.TokenFunction},
		{"*c*", "", ui.TokenEmphasis},
	}},
	{"brackets in markup pair up", "#f[a [b] *c*] *d*", []span{
		{"*c*", "", ui.TokenEmphasis},
		{"*d*", "", ui.TokenEmphasis},
	}},
	{"a statement ends with its line", "#let x = 1\n*b*", []span{
		{"1", "", ui.TokenNumber},
		{"*b*", "", ui.TokenEmphasis},
	}},
	{"a statement ends with a semicolon", "#let y = 2; *s* text", []span{
		{"*s*", "", ui.TokenEmphasis},
		{" text", "", ui.TokenPlain},
	}},
	{"a statement goes on in its brackets", "#let f(x) = [\n= Title\n] + 1", []span{
		{"f(", "f", ui.TokenFunction},
		{"= Title", "", ui.TokenHeading},
		{"+ 1", "1", ui.TokenNumber},
	}},
	{"numbers and units", "#f(50%, 2deg, 0.5cm, 3mm, 1in, 1.5em, 1e3, 12pt, 0xff, 2.5e-3)", []span{
		{"50%", "", ui.TokenNumber},
		{"2deg", "", ui.TokenNumber},
		{"0.5cm", "", ui.TokenNumber},
		{"3mm", "", ui.TokenNumber},
		{"1in", "", ui.TokenNumber},
		{"1.5em", "", ui.TokenNumber},
		{"1e3", "", ui.TokenNumber},
		{"12pt", "", ui.TokenNumber},
		{"0xff", "", ui.TokenNumber},
		{"2.5e-3", "", ui.TokenNumber},
	}},
	{"fields and methods", "#x.at(0).len() and #y.field. End", []span{
		{"#x", "", ui.TokenPlain},
		{"at", "", ui.TokenFunction},
		{"len", "", ui.TokenFunction},
		{" and ", "", ui.TokenPlain},
		{"#y.field. End", "", ui.TokenPlain},
	}},
	{"strings with escapes", `#f("a\"b", "c") d`, []span{
		{`"a\"b"`, "", ui.TokenString},
		{`"c"`, "", ui.TokenString},
		{" d", "", ui.TokenPlain},
	}},
	{"an unclosed string ends with its line", "#f(\"abc\nnext)\n", []span{
		{"\"abc", "", ui.TokenString},
		{"next", "", ui.TokenPlain},
	}},
	{"labels and references", "See @fig:a-b.c. and <fig:a-b.c_d> or a < b > c, mail@x", []span{
		{"@fig:a-b.c", "", ui.TokenMarkup},
		{". and ", "", ui.TokenPlain},
		{"<fig:a-b.c_d>", "", ui.TokenMarkup},
		{"a < b > c", "", ui.TokenPlain},
	}},
	{"hyphens in names", "#table-header(x-1, a - b)", []span{
		{"#table-header", "", ui.TokenFunction},
		{"x-1", "x", ui.TokenPlain},
		{"x-1", "1", ui.TokenNumber},
	}},
	{"math", `$ a \$ "$" b $ c`, []span{
		{`$ a \$ "$" b $`, "", ui.TokenMath},
		{" c", "", ui.TokenPlain},
	}},
	{"math in code", "#f($x^2$, 1)", []span{
		{"$x^2$", "", ui.TokenMath},
		{", 1", "1", ui.TokenNumber},
	}},
	{"a lone hash is text", "C# and #1 and # alone", []span{
		{"C# and ", "", ui.TokenPlain},
		{"#1", "1", ui.TokenNumber},
		{"# alone", "", ui.TokenPlain},
	}},
	{"keywords on their own", "#none #auto #true #false #break", []span{
		{"#none", "", ui.TokenKeyword},
		{"#auto", "", ui.TokenKeyword},
		{"#true", "", ui.TokenKeyword},
		{"#false", "", ui.TokenKeyword},
		{"#break", "", ui.TokenKeyword},
	}},
	{"content arguments", "#f(1)[body *b*][two] three", []span{
		{"#f", "", ui.TokenFunction},
		{"*b*", "", ui.TokenEmphasis},
		{"two", "", ui.TokenPlain},
		{" three", "", ui.TokenPlain},
	}},
	{"import", `#import "x.typ": a, b`, []span{
		{"#import", "", ui.TokenKeyword},
		{`"x.typ"`, "", ui.TokenString},
	}},
	{"markup inside code inside markup", `#f[a #g("s", [c *d*]) e] f`, []span{
		{"#g", "", ui.TokenFunction},
		{`"s"`, "", ui.TokenString},
		{"*d*", "", ui.TokenEmphasis},
		{" e", "", ui.TokenPlain},
		{"] f", " f", ui.TokenPlain},
	}},
	{"code keywords", "#{ not a and b or c as d }", []span{
		{"not", "", ui.TokenKeyword},
		{"and", "", ui.TokenKeyword},
		{"or", "", ui.TokenKeyword},
		{"as", "", ui.TokenKeyword},
	}},
	{"non-ASCII text", "Grüße *für* _Öl_ und #straße[ä] — ü*x*y", []span{
		{"Grüße ", "", ui.TokenPlain},
		{"*für*", "", ui.TokenEmphasis},
		{"_Öl_", "", ui.TokenEmphasis},
		{"#straße", "", ui.TokenFunction},
		{"ü*x*y", "", ui.TokenPlain},
	}},
}

var goCases = []highlightCase{
	{"sample", goSample, []span{
		{"package", "", ui.TokenKeyword},
		{"main", "", ui.TokenPlain},
		{"import", "", ui.TokenKeyword},
		{`"fmt"`, "", ui.TokenString},
		{"// Greet says hello.", "", ui.TokenComment},
		{"func Greet", "func", ui.TokenKeyword},
		{"Greet(", "Greet", ui.TokenFunction},
		{"name string", "name", ui.TokenPlain},
		{"name string", "string", ui.TokenType},
		{") error", "error", ui.TokenType},
		{"x :=", "x", ui.TokenPlain},
		{"0x1F", "", ui.TokenNumber},
		{"3.5", "", ui.TokenNumber},
		{"2i", "", ui.TokenNumber},
		{"'a'", "", ui.TokenString},
		{"`raw\nstring`", "", ui.TokenString},
		{"var ok", "var", ui.TokenKeyword},
		{"ok bool", "bool", ui.TokenType},
		{"true", "", ui.TokenKeyword},
		{"nil ==", "nil", ui.TokenKeyword},
		{"fmt.Println", "fmt", ui.TokenPlain},
		{"Println", "", ui.TokenFunction},
		{`"hi"`, "", ui.TokenString},
		{"len(", "len", ui.TokenFunction},
		{"return nil", "return", ui.TokenKeyword},
		{"(t *T)", "t", ui.TokenPlain},
		{"Method", "", ui.TokenFunction},
		{"n int", "int", ui.TokenType},
		{"Map[", "Map", ui.TokenFunction},
		{"E any", "any", ui.TokenType},
		{"xs []E", "xs", ui.TokenPlain},
		{"iota", "", ui.TokenKeyword},
		{"type T struct", "type", ui.TokenKeyword},
		{"type T struct", "T", ui.TokenPlain},
		{"type T struct", "struct", ui.TokenKeyword},
		{"byte", "", ui.TokenType},
		{"float64", "", ui.TokenType},
		{"uintptr", "", ui.TokenType},
		{"complex128", "", ui.TokenType},
		{"/* block */", "", ui.TokenComment},
	}},
	{"carriage returns", "// c\r\nvar s = `a\r\nb` /* d\r\n */\r\nx", []span{
		{"// c", "", ui.TokenComment},
		{"`a\r\nb`", "", ui.TokenString},
		{"/* d\r\n */", "", ui.TokenComment},
		{"\nx", "x", ui.TokenPlain},
	}},
	{"half typed", "func main() {\n\tfmt.Println(\"unterminated\n}\nfunc f(", []span{
		{"main", "", ui.TokenFunction},
		{"Println", "", ui.TokenFunction},
		{"\"unterminated", "", ui.TokenString},
		{"f(", "f", ui.TokenFunction},
	}},
	{"conversions are types", "b := string(r) + rune(0)", []span{
		{"string", "", ui.TokenType},
		{"rune", "", ui.TokenType},
		{"0", "", ui.TokenNumber},
	}},
	{"calls", "f (x)\ng\n(y)\nfunc h[T any]() {}\nvar fn func() T", []span{
		{"f (x)", "f", ui.TokenFunction},
		{"g\n", "g", ui.TokenPlain},
		{"h[", "h", ui.TokenFunction},
		{"fn func", "fn", ui.TokenPlain},
		{"() T", "T", ui.TokenPlain},
	}},
	{"comments and literals", "x := 1_000 + .5 + 1e-3 + 'x' + '\\n' // done", []span{
		{"1_000", "", ui.TokenNumber},
		{".5", "", ui.TokenNumber},
		{"1e-3", "", ui.TokenNumber},
		{"'x'", "", ui.TokenString},
		{"'\\n'", "", ui.TokenString},
		{"// done", "", ui.TokenComment},
	}},
}

func TestTypstColoursWhatItRecognises(t *testing.T) {
	runCases(t, syntax.Typst, typstCases)
}

func TestGoColoursWhatItRecognises(t *testing.T) {
	runCases(t, syntax.Go, goCases)
}

func runCases(t *testing.T, h ui.Highlighter, cases []highlightCase) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tokens := h.Highlight(c.src, nil)
			if err := validate(c.src, tokens); err != nil {
				t.Fatal(err)
			}
			for _, s := range c.spans {
				at := strings.Index(c.src, s.in)
				if at < 0 {
					t.Fatalf("%q is not in the source", s.in)
				}
				sub := s.in
				if s.sub != "" {
					j := strings.Index(s.in, s.sub)
					if j < 0 {
						t.Fatalf("%q is not in %q", s.sub, s.in)
					}
					at, sub = at+j, s.sub
				}
				for off := at; off < at+len(sub); off++ {
					if got := kindAt(tokens, off); got != s.kind {
						t.Errorf("%q in %q: byte %d is %s, want %s\n%s",
							sub, s.in, off-at, kindName(got), kindName(s.kind), dump(c.src, tokens))
						break
					}
				}
			}
		})
	}
}

// TestTypstEmbeddedExpressionsInArgumentsDoNotDerail checks the #i inside
// the content blocks of the sample's last table row, which Typst reads as a
// variable: whatever it is coloured, it must not leave the rest of the
// document in the wrong mode.
func TestTypstEmbeddedExpressionsInArgumentsDoNotDerail(t *testing.T) {
	tokens := syntax.Typst.Highlight(typstSample, nil)
	at := strings.Index(typstSample, "[#i]") + 1
	if k := kindAt(tokens, at); k != ui.TokenPlain && k != ui.TokenFunction {
		t.Errorf("#i is %s, want plain or a function", kindName(k))
	}
	at = strings.Index(typstSample, "Arbeitspaket #i") + len("Arbeitspaket ")
	if k := kindAt(tokens, at); k != ui.TokenPlain && k != ui.TokenFunction {
		t.Errorf("the second #i is %s, want plain or a function", kindName(k))
	}
}

func TestHighlightAppendsToDst(t *testing.T) {
	for _, h := range []ui.Highlighter{syntax.Typst, syntax.Go} {
		want := h.Highlight(typstSample, nil)
		sentinel := ui.Token{Start: -7, End: -3, Kind: ui.TokenMath}
		got := h.Highlight(typstSample, []ui.Token{sentinel})
		if got[0] != sentinel {
			t.Fatalf("the token already in dst became %+v", got[0])
		}
		if fmt.Sprint(got[1:]) != fmt.Sprint(want) {
			t.Fatalf("the tokens appended to dst differ from those of an empty dst")
		}
	}
}

// TestHighlightersAcceptAnyText checks the promises of [ui.Highlighter] on
// texts that do not parse: the sources above cut off after every byte,
// unbalanced brackets, quotes and delimiters, invalid UTF-8, random bytes
// and deep nesting.
func TestHighlightersAcceptAnyText(t *testing.T) {
	for _, src := range anyText() {
		for _, h := range []ui.Highlighter{syntax.Typst, syntax.Go} {
			if err := validate(src, h.Highlight(src, nil)); err != nil {
				t.Fatalf("%v\nsource %q", err, short(src))
			}
		}
	}
}

func TestTypstAllocatesNothingWhenDstHasRoom(t *testing.T) {
	src := typstDocument(10 << 10)
	dst := syntax.Typst.Highlight(src, nil)
	if allocs := testing.AllocsPerRun(20, func() { dst = syntax.Typst.Highlight(src, dst[:0]) }); allocs != 0 {
		t.Errorf("Typst allocates %v times per run, want 0", allocs)
	}
}

func FuzzTypst(f *testing.F) {
	for _, s := range seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if err := validate(src, syntax.Typst.Highlight(src, nil)); err != nil {
			t.Fatal(err)
		}
	})
}

func FuzzGo(f *testing.F) {
	for _, s := range seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if err := validate(src, syntax.Go.Highlight(src, nil)); err != nil {
			t.Fatal(err)
		}
	})
}

// BenchmarkTypst measures a document of 10 KiB, the size of a long letter
// or a short report, which the editor highlights after every keystroke.
func BenchmarkTypst(b *testing.B) {
	src := typstDocument(10 << 10)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	var dst []ui.Token
	for b.Loop() {
		dst = syntax.Typst.Highlight(src, dst[:0])
	}
}

// BenchmarkGo measures a Go file of 10 KiB, for comparison with
// [BenchmarkTypst].
func BenchmarkGo(b *testing.B) {
	src := strings.Repeat(goSample, (10<<10)/len(goSample)+1)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	var dst []ui.Token
	for b.Loop() {
		dst = syntax.Go.Highlight(src, dst[:0])
	}
}

// typstDocument repeats the sample to at least n bytes.
func typstDocument(n int) string {
	return strings.Repeat(typstSample, (n+len(typstSample)-1)/len(typstSample))
}

// validate checks the promises of [ui.Highlighter] about tokens: in order,
// not overlapping, not empty, inside src and of a kind there is. On valid
// UTF-8 it also checks that no token splits a character.
func validate(src string, tokens []ui.Token) error {
	end := 0
	valid := utf8.ValidString(src)
	for i, tok := range tokens {
		switch {
		case tok.Start < end:
			return fmt.Errorf("token %d %+v starts before the end %d of the one before it", i, tok, end)
		case tok.End <= tok.Start:
			return fmt.Errorf("token %d %+v is empty", i, tok)
		case tok.End > len(src):
			return fmt.Errorf("token %d %+v ends past the source's %d bytes", i, tok, len(src))
		case tok.Kind == ui.TokenPlain || tok.Kind > ui.TokenMath:
			return fmt.Errorf("token %d %+v has kind %s", i, tok, kindName(tok.Kind))
		case valid && (!utf8.RuneStart(src[tok.Start]) || tok.End < len(src) && !utf8.RuneStart(src[tok.End])):
			return fmt.Errorf("token %d %+v splits a character", i, tok)
		}
		end = tok.End
	}
	return nil
}

// kindAt returns the kind of the token over the byte at offset, or
// TokenPlain when no token covers it.
func kindAt(tokens []ui.Token, offset int) ui.TokenKind {
	i := sort.Search(len(tokens), func(i int) bool { return tokens[i].End > offset })
	if i < len(tokens) && tokens[i].Start <= offset {
		return tokens[i].Kind
	}
	return ui.TokenPlain
}

var kindNames = [...]string{
	"Plain", "Keyword", "String", "Number", "Comment", "Function", "Type",
	"Heading", "Emphasis", "Markup", "Raw", "Math",
}

func kindName(k ui.TokenKind) string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("TokenKind(%d)", k)
}

// dump lists the tokens with their text, for a failure message.
func dump(src string, tokens []ui.Token) string {
	var b strings.Builder
	for _, tok := range tokens {
		fmt.Fprintf(&b, "\t%d-%d %s %q\n", tok.Start, tok.End, kindName(tok.Kind), src[tok.Start:tok.End])
	}
	return b.String()
}

func short(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// ragged are texts that are cut off, unbalanced or not UTF-8.
var ragged = []string{
	"", "#", "#f", "#f(", "#f[", "#{", "#(", "#[", "#\"", "#1", "#let", "#let x = (",
	"#if", "#if x [", "#if x [a] else", "#if x [a] else if", "#for x in", "#context", "#context ",
	"#x.", "#x.y(", "[", "]", "(", ")", "{", "}", "[[[", "]]]", ")))", "}}}",
	"#f(]", "#f[)", "#{]", "#f(a, [b)", "#{[}]", "#f([)]", "#{ let x = [ } ]",
	"\"", "#f(\"", "#f(\"\\", "#f(\"\\\n", "$", "$$", "$\\", "$\"", "$\"\\",
	"*", "**", "_", "__", "*a\n\n", "*[", "*]", "[*]", "`", "``", "```", "````a```",
	"/*", "/* /*", "*/", "/*/", "//", "://", "\\", "\\u", "\\u{", "\\u{12", "\\u{zz}",
	"<", "<a", "<>", "<a>", "@", "@.", "@a:", "=", "= ", "==", "-", "- ", "+", "/ ", "/ a", "/ a:", "1.", "1. ",
	"\xff", "\xc3", "#\xff", "*\xc3", "\xc3*", "<a\xc3>", "\\\xc3", "\\\xe2\x82", "#f\xff(",
	"é*x*é", "a_\xff_b", "#\xc3\xa9(", "@\xc3", "#x-\xc3", "1\xc3", "= \xff",
	"\r\n= a\r\n", "#let x = 1\r\n", "\x00", "#\x00(", "\ufeff#f()",
	"func", "func (", "func (r T", "x := `", "'", "/*\n", "package \xff", "a\xffb", "0x", "1e", "'\\",
}

// fragments are the pieces random texts are made of: everything either
// highlighter treats specially, and some text.
var fragments = []string{
	"#", "(", ")", "[", "]", "{", "}", "$", "*", "_", "`", "\"", "'", "/", "\\", "<", ">", "@",
	"=", "-", "+", ".", ":", ";", " ", "  ", "\n", "\n\n", "\t", "\r", "a", "x", "Z9", "é", "ü",
	"let", "if", "else", "for", "in", "set", "show", "context", "func", "go", "1", "2pt", "0.5",
	"e3", "%", "//", "/*", "*/", "```", "#f(", "#f[", "\\u{", "<a>", "@a", "\xff", "\xc3", "\x00",
}

// anyText returns the texts of [TestHighlightersAcceptAnyText].
func anyText() []string {
	var texts []string
	for _, src := range []string{typstSample, goSample} {
		for i := range len(src) {
			texts = append(texts, src[:i], src[i:])
		}
	}
	for _, c := range append(append([]highlightCase(nil), typstCases...), goCases...) {
		texts = append(texts, c.src, c.src[:len(c.src)/2])
	}
	for _, r := range ragged {
		texts = append(texts, r, r+typstSample, typstSample+r, r+goSample, goSample+r, r+r+r)
	}

	rng := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		var b strings.Builder
		for range rng.IntN(60) {
			if rng.IntN(8) == 0 {
				b.WriteByte(byte(rng.IntN(256)))
			} else {
				b.WriteString(fragments[rng.IntN(len(fragments))])
			}
		}
		texts = append(texts, b.String())
	}

	// Deep nesting goes past the frames the Typst highlighter keeps in
	// place, and long runs of one delimiter would show work that is not
	// linear as a test that does not finish.
	for _, r := range []string{
		"(", "[", "{", "#f(", "#f[", "#{", "#if x [", "[#let x = (", "#f[#g(",
		"]", ")", "}", "*", "_ ", "*a\n", "/*", "*/", "$", "`", "<a", "@a.", "#context ",
		"\\", "= ", "#", "#x.", "- ", "/ a", "\"", "func (", "((((]]]]",
	} {
		texts = append(texts, strings.Repeat(r, 50_000/len(r)))
	}
	return texts
}

// seeds are the starting corpus of the fuzz targets.
func seeds() []string {
	s := []string{typstSample, goSample}
	s = append(s, ragged...)
	for _, c := range typstCases {
		s = append(s, c.src)
	}
	for _, c := range goCases {
		s = append(s, c.src)
	}
	return s
}
