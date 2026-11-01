package syntax

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/worldiety/gift/ui"
)

// Typst colours Typst documents, https://typst.app/docs/reference/syntax/.
//
// Typst nests three modes into each other, and the highlighter keeps a stack
// of them: markup at the top level and inside content blocks, [...]; code
// inside parentheses and code blocks, (...) and {...}, and after a # in
// markup; and math between dollar signs, which it colours as one span. It
// colours
//
//   - in markup: headings, the markers of lists, enumerations and terms,
//     strong and emphasised text, raw text, math, escapes, labels,
//     references and comments;
//   - in code: keywords, strings, numbers with their units, comments, and
//     the names of the functions that are called;
//   - after a # in markup: the keyword or the name of the called function,
//     with the #, and then the code the expression goes on with. That is the
//     rest of the line after #let, #set, #show, #import, #include and
//     #return; the condition and the bodies after #if, #for and #while, with
//     an else between bodies; and fields, arguments and content blocks after
//     a name.
//
// It is a lexer and not Typst's parser, and where telling the two apart
// would take a parser it settles for the simpler rule:
//
//   - A statement such as #let runs to the end of its line or a semicolon,
//     where Typst ends it with its expression.
//   - Strong and emphasised text is one span with its delimiters, and a
//     heading one span to the end of its line or to a comment or label on
//     it; code and markup inside them are not coloured on their own.
//   - A string ends at the end of its line when it has no closing quote yet,
//     so that a quote just typed does not colour the rest of the document.
//   - A closing bracket that closes nothing is skipped. One that belongs to a
//     bracket further out than the innermost closes everything inside it.
//
// It takes time linear in the length of the text and allocates nothing
// beyond the growth of dst, unless brackets nest deeper than thirty-two.
var Typst ui.Highlighter = ui.HighlighterFunc(highlightTypst)

// frame is one level of the nesting of Typst's modes.
type frame uint8

const (
	// frameContent is a content block: markup up to its ].
	frameContent frame = iota
	// frameGroup is a parenthesised expression or argument list: code up to
	// its ).
	frameGroup
	// frameBlock is a code block: code up to its }.
	frameBlock
	// frameStatement follows #let, #set and their kin in markup: code up to
	// the end of the line or a semicolon.
	frameStatement
	// frameCondition follows #if, #for and #while in markup: code up to the
	// body, which has to start on the same line.
	frameCondition
	// frameElse follows the body of a frameCondition, where an else may
	// continue it.
	frameElse
	// frameChain follows an expression embedded in markup with #: a field,
	// arguments or a content block continue it, and anything else ends it.
	frameChain
)

// typstLexer is the state of one run of [Typst] over a text.
type typstLexer struct {
	src string
	pos int

	// depth is the number of open frames. The outermost of them are in
	// shallow and those nested deeper than it holds in deep: a fixed array
	// in the lexer keeps the usual nesting off the heap, which a slice over
	// it would not, because the escape analysis cannot tell that the slice
	// never leaves the lexer. Outside of all frames is the markup of the top
	// level.
	depth   int
	shallow [32]frame
	deep    []frame
	// open counts the open frames that a closing bracket closes, by frame,
	// so that a closer learns in constant time whether it closes anything at
	// all.
	open [frameBlock + 1]int
	// lineStart reports whether only spaces precede pos on its line, which
	// is where headings and list markers are recognised.
	lineStart bool
}

func highlightTypst(src string, dst []ui.Token) []ui.Token {
	l := typstLexer{src: src, lineStart: true}
	// Every step either moves pos on or pops a frame, and every frame was
	// pushed by a step that moved pos on, so the loop ends after at most two
	// steps per byte.
	for l.pos < len(src) {
		switch l.top() {
		case frameContent:
			dst = l.markup(dst)
		case frameGroup, frameBlock, frameStatement, frameCondition:
			dst = l.code(dst)
		case frameElse:
			dst = l.orElse(dst)
		case frameChain:
			dst = l.chain(dst)
		}
	}
	return dst
}

// top returns the innermost frame. The top level is markup, as in a content
// block that never closes.
func (l *typstLexer) top() frame {
	switch {
	case l.depth == 0:
		return frameContent
	case l.depth <= len(l.shallow):
		return l.shallow[l.depth-1]
	}
	return l.deep[l.depth-1-len(l.shallow)]
}

// setTop replaces the innermost frame with f, which is how a condition turns
// into the wait for its else. Neither is counted in open.
func (l *typstLexer) setTop(f frame) {
	if l.depth <= len(l.shallow) {
		l.shallow[l.depth-1] = f
		return
	}
	l.deep[l.depth-1-len(l.shallow)] = f
}

func (l *typstLexer) push(f frame) {
	if l.depth < len(l.shallow) {
		l.shallow[l.depth] = f
	} else {
		l.deep = append(l.deep, f)
	}
	l.depth++
	if f <= frameBlock {
		l.open[f]++
	}
}

// pop closes the innermost frame and returns it.
func (l *typstLexer) pop() frame {
	f := l.top()
	l.depth--
	if l.depth >= len(l.shallow) {
		l.deep = l.deep[:l.depth-len(l.shallow)]
	}
	if f <= frameBlock {
		l.open[f]--
	}
	return f
}

// close ends the innermost open frame want, together with every frame
// opened inside it, for the closing bracket that belongs to it. It reports
// false and changes nothing when no such frame is open.
func (l *typstLexer) close(want frame) bool {
	if l.open[want] == 0 {
		return false
	}
	for l.pop() != want {
	}
	return true
}

// emit appends the token from start to end and moves pos past it.
func (l *typstLexer) emit(dst []ui.Token, start, end int, kind ui.TokenKind) []ui.Token {
	l.pos = end
	return append(dst, ui.Token{Start: start, End: end, Kind: kind})
}

// markupSpecial marks the bytes at which something other than plain text
// can start in markup, away from the start of a line. Most of a document is
// prose, and this lets markup skip it in a tight loop.
var markupSpecial = [256]bool{
	'\n': true, '/': true, '*': true, '_': true, '`': true, '$': true,
	'\\': true, '<': true, '@': true, '#': true, '[': true, ']': true,
}

// markup lexes one thing of markup at pos.
func (l *typstLexer) markup(dst []ui.Token) []ui.Token {
	src, i := l.src, l.pos
	if !l.lineStart {
		for i < len(src) && !markupSpecial[src[i]] {
			i++
		}
		if i == len(src) {
			l.pos = i
			return dst
		}
	}
	c := src[i]
	switch c {
	case '\n':
		l.lineStart = true
		l.pos = i + 1
		return dst
	case ' ', '\t', '\r':
		l.pos = i + 1
		return dst
	}
	inContent := l.depth > 0
	if l.lineStart {
		l.lineStart = false
		if end, kind := lineMarker(src, i, inContent); end > i {
			return l.emit(dst, i, end, kind)
		}
	}

	switch c {
	case '/':
		// Typst reads a URL's scheme as a link; short of that, a // after a
		// colon is taken for one and not for a comment.
		if i > 0 && src[i-1] == ':' && i+1 < len(src) && src[i+1] == '/' {
			l.pos = i + 2
			return dst
		}
		if end := commentEnd(src, i); end > i {
			return l.emit(dst, i, end, ui.TokenComment)
		}
	case '*', '_':
		if !inWord(src, i) {
			return l.emit(dst, i, emphasisEnd(src, i, inContent), ui.TokenEmphasis)
		}
	case '`':
		return l.emit(dst, i, rawEnd(src, i), ui.TokenRaw)
	case '$':
		return l.emit(dst, i, mathEnd(src, i), ui.TokenMath)
	case '\\':
		return l.emit(dst, i, escapeEnd(src, i), ui.TokenMarkup)
	case '<':
		if end := labelEnd(src, i); end > i {
			return l.emit(dst, i, end, ui.TokenMarkup)
		}
	case '@':
		if end := refEnd(src, i); end > i {
			return l.emit(dst, i, end, ui.TokenMarkup)
		}
	case '#':
		return l.hash(dst, i)
	case '[':
		// A bracket in markup is text, but it pairs with the next ] all
		// the same, which then does not close the content block around it.
		l.push(frameContent)
	case ']':
		if inContent {
			l.pop()
		}
	}
	l.pos = i + 1
	return dst
}

// lineMarker recognises what Typst sees only at the start of a line: a
// heading, and the markers of lists, enumerations and terms. It returns i
// when there is none at i.
func lineMarker(src string, i int, inContent bool) (int, ui.TokenKind) {
	switch c := src[i]; {
	case c == '=':
		j := i
		for j < len(src) && src[j] == '=' {
			j++
		}
		if spaceOrEnd(src, j) {
			return headingEnd(src, j, inContent), ui.TokenHeading
		}
	case c == '-' || c == '+':
		if spaceOrEnd(src, i+1) {
			return i + 1, ui.TokenMarkup
		}
	case c == '/':
		// A term is marked up to the colon after it, when the colon is on
		// the same line.
		if i+1 < len(src) && (src[i+1] == ' ' || src[i+1] == '\t') {
			for j := i + 2; j < len(src) && src[j] != '\n'; j++ {
				if src[j] == ':' {
					return j + 1, ui.TokenMarkup
				}
			}
			return i + 1, ui.TokenMarkup
		}
	case isDigit(c):
		j := digitsEnd(src, i)
		if j < len(src) && src[j] == '.' && spaceOrEnd(src, j+1) {
			return j + 1, ui.TokenMarkup
		}
	}
	return i, ui.TokenPlain
}

// headingEnd returns the end of the heading whose text starts at i: the end
// of the line, or a comment or label on it, or the ] of the content block it
// is in.
func headingEnd(src string, i int, inContent bool) int {
	depth := 0
	for ; i < len(src); i++ {
		switch src[i] {
		case '\n':
			return i
		case '/':
			if i+1 < len(src) && (src[i+1] == '*' || src[i+1] == '/' && src[i-1] != ':') {
				return i
			}
		case '<':
			if labelEnd(src, i) > i {
				return i
			}
		case '[':
			depth++
		case ']':
			if depth == 0 && inContent {
				return i
			}
			if depth > 0 {
				depth--
			}
		case '\\':
			if i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		}
	}
	return len(src)
}

// emphasisEnd returns the end of the strong or emphasised text whose
// delimiter is at i: after the matching delimiter or, when there is none,
// at the paragraph break, the ] of the content block it is in, or the end of
// the text, which is where Typst gives up on it as well.
func emphasisEnd(src string, i int, inContent bool) int {
	delim := src[i]
	depth := 0
	for j := i + 1; j < len(src); j++ {
		switch c := src[j]; {
		case c == delim:
			if !inWord(src, j) {
				return j + 1
			}
		case c == '\\':
			if j+1 < len(src) && src[j+1] != '\n' {
				j++
			}
		case c == '\n':
			if blankLineAfter(src, j) {
				return j
			}
		case c == '[':
			depth++
		case c == ']':
			if depth == 0 && inContent {
				return j
			}
			if depth > 0 {
				depth--
			}
		}
	}
	return len(src)
}

// blankLineAfter reports whether the line after the line break at i is
// blank, which makes the break a paragraph break.
func blankLineAfter(src string, i int) bool {
	j := i + 1
	for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r') {
		j++
	}
	return j == len(src) || src[j] == '\n'
}

// inWord reports whether the * or _ at i stands between two letters or
// digits. Typst does not read it as a delimiter there, so that snake_case
// and 2*3 stay text.
func inWord(src string, i int) bool {
	if i == 0 || i+1 >= len(src) {
		return false
	}
	before, _ := utf8.DecodeLastRuneInString(src[:i])
	after, _ := utf8.DecodeRuneInString(src[i+1:])
	return isAlnum(before) && isAlnum(after)
}

// rawEnd returns the end of the raw text whose backticks start at i. Two
// backticks are empty raw text; one, or three and more, are closed by as
// many backticks in a row, or by the end of the text.
func rawEnd(src string, i int) int {
	n := 0
	for i+n < len(src) && src[i+n] == '`' {
		n++
	}
	if n == 2 {
		return i + 2
	}
	found := 0
	for j := i + n; j < len(src); j++ {
		if src[j] != '`' {
			found = 0
			continue
		}
		if found++; found == n {
			return j + 1
		}
	}
	return len(src)
}

// mathEnd returns the end of the math whose dollar sign is at i: after the
// closing dollar sign, which is neither escaped nor in a string, or the end
// of the text.
func mathEnd(src string, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '$':
			return j + 1
		case '\\':
			j++
		case '"':
			for j++; j < len(src) && src[j] != '"' && src[j] != '\n'; j++ {
				if src[j] == '\\' {
					j++
				}
			}
		}
	}
	return len(src)
}

// escapeEnd returns the end of the escape whose backslash is at i: the
// character after it, a Unicode escape \u{...}, or the backslash alone when
// white space follows, which makes it a line break.
func escapeEnd(src string, i int) int {
	j := i + 1
	if j >= len(src) {
		return j
	}
	switch src[j] {
	case ' ', '\t', '\n', '\r':
		return j
	case 'u':
		if j+1 < len(src) && src[j+1] == '{' {
			k := j + 2
			for k < len(src) && isHex(src[k]) {
				k++
			}
			if k < len(src) && src[k] == '}' {
				k++
			}
			return k
		}
	}
	_, size := utf8.DecodeRuneInString(src[j:])
	return j + size
}

// labelEnd returns the end of the label <name> at i, or i when there is
// none.
func labelEnd(src string, i int) int {
	j := labelNameEnd(src, i+1)
	if j > i+1 && j < len(src) && src[j] == '>' {
		return j + 1
	}
	return i
}

// refEnd returns the end of the reference @name at i, or i when there is
// none. A full stop or colon at the end belongs to the sentence, not to the
// name.
func refEnd(src string, i int) int {
	j := labelNameEnd(src, i+1)
	for j > i+1 && (src[j-1] == '.' || src[j-1] == ':') {
		j--
	}
	if j > i+1 {
		return j
	}
	return i
}

// labelNameEnd returns the end of the name of a label or reference that
// starts at i: letters, digits and - _ . :.
func labelNameEnd(src string, i int) int {
	for i < len(src) {
		c := src[i]
		if c < utf8.RuneSelf {
			if !isAlnum(rune(c)) && c != '-' && c != '_' && c != '.' && c != ':' {
				return i
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(src[i:])
		if !isAlnum(r) {
			return i
		}
		i += size
	}
	return i
}

// hash lexes what the # at i embeds in markup. Followed by nothing that
// starts an expression, the # is text.
func (l *typstLexer) hash(dst []ui.Token, i int) []ui.Token {
	src := l.src
	start, j := i, i+1
	l.pos = j
	// The loop goes round once more for each context keyword, which takes
	// an expression of its own.
	for j < len(src) {
		switch c := src[j]; {
		case c == '(':
			l.push(frameChain)
			l.push(frameGroup)
			l.pos = j + 1
			return dst
		case c == '{':
			l.push(frameChain)
			l.push(frameBlock)
			l.pos = j + 1
			return dst
		case c == '[':
			l.push(frameChain)
			l.openContent(j)
			return dst
		case c == '"':
			l.push(frameChain)
			return l.emit(dst, j, stringEnd(src, j), ui.TokenString)
		case isDigit(c):
			return l.emit(dst, j, numberEnd(src, j), ui.TokenNumber)
		}

		end := identEnd(src, j)
		if end == j {
			return dst
		}
		switch src[j:end] {
		case "let", "set", "show", "import", "include", "return":
			l.push(frameStatement)
		case "if", "for", "while":
			l.push(frameCondition)
		case "context":
			dst = l.emit(dst, start, end, ui.TokenKeyword)
			for j = end; j < len(src) && (src[j] == ' ' || src[j] == '\t'); j++ {
			}
			start = j
			continue
		case "else", "in", "break", "continue", "none", "auto", "true", "false":
		default:
			l.push(frameChain)
			if isCall(src, end) {
				return l.emit(dst, start, end, ui.TokenFunction)
			}
			l.pos = end
			return dst
		}
		return l.emit(dst, start, end, ui.TokenKeyword)
	}
	return dst
}

// openContent opens the content block whose [ is at i, after which a
// heading or list marker may start as at the start of a line.
func (l *typstLexer) openContent(i int) {
	l.push(frameContent)
	l.lineStart = true
	l.pos = i + 1
}

// code lexes one thing of code at pos.
func (l *typstLexer) code(dst []ui.Token) []ui.Token {
	src, i := l.src, l.pos
	c := src[i]
	switch {
	case c == '\n':
		if f := l.top(); f == frameStatement || f == frameCondition {
			// The line ends what #let or #if began, and the line break
			// itself is the markup's.
			l.pop()
			return dst
		}
	case c == ';':
		if l.top() == frameStatement {
			l.pop()
		}
	case c == '/':
		if end := commentEnd(src, i); end > i {
			return l.emit(dst, i, end, ui.TokenComment)
		}
	case c == '"':
		return l.emit(dst, i, stringEnd(src, i), ui.TokenString)
	case c == '$':
		return l.emit(dst, i, mathEnd(src, i), ui.TokenMath)
	case c == '`':
		return l.emit(dst, i, rawEnd(src, i), ui.TokenRaw)
	case isDigit(c):
		return l.emit(dst, i, numberEnd(src, i), ui.TokenNumber)
	case c == '(':
		l.push(frameGroup)
	case c == '{' || c == '[':
		if l.top() == frameCondition {
			l.setTop(frameElse)
		}
		if c == '[' {
			l.openContent(i)
			return dst
		}
		l.push(frameBlock)
	case c == ')':
		l.close(frameGroup)
	case c == '}':
		l.close(frameBlock)
	case c == ']':
		l.close(frameContent)
	default:
		end := identEnd(src, i)
		if end == i {
			break
		}
		if isKeyword(src[i:end]) {
			return l.emit(dst, i, end, ui.TokenKeyword)
		}
		if isCall(src, end) {
			return l.emit(dst, i, end, ui.TokenFunction)
		}
		l.pos = end
		return dst
	}
	l.pos = i + 1
	return dst
}

// chain continues the expression that a # embedded in markup, or ends it.
func (l *typstLexer) chain(dst []ui.Token) []ui.Token {
	src, i := l.src, l.pos
	switch src[i] {
	case '(':
		l.push(frameGroup)
		l.pos = i + 1
		return dst
	case '[':
		l.openContent(i)
		return dst
	case '.':
		// A full stop that no name follows ends the sentence, not the
		// expression's field.
		if end := identEnd(src, i+1); end > i+1 {
			if isCall(src, end) {
				return l.emit(dst, i+1, end, ui.TokenFunction)
			}
			l.pos = end
			return dst
		}
	}
	l.pop()
	return dst
}

// orElse continues a condition with the else after its body, or ends it.
func (l *typstLexer) orElse(dst []ui.Token) []ui.Token {
	src := l.src
	j := l.pos
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if end := identEnd(src, j); src[j:end] == "else" {
		l.setTop(frameCondition)
		return l.emit(dst, j, end, ui.TokenKeyword)
	}
	l.pop()
	return dst
}

// isKeyword reports whether word is one of Typst's keywords in code.
func isKeyword(word string) bool {
	switch word {
	case "let", "set", "show", "if", "else", "for", "in", "while", "break",
		"continue", "return", "import", "include", "context", "none", "auto",
		"true", "false", "and", "or", "not", "as":
		return true
	}
	return false
}

// isCall reports whether arguments or a content block follow at i, which
// makes the name before i a function that is called.
func isCall(src string, i int) bool {
	return i < len(src) && (src[i] == '(' || src[i] == '[')
}

// commentEnd returns the end of the comment at i, or i when none starts
// there. A line comment ends before its line break. Block comments nest, as
// in Typst, and one that is not closed runs to the end of the text.
func commentEnd(src string, i int) int {
	if i+1 >= len(src) || src[i] != '/' {
		return i
	}
	switch src[i+1] {
	case '/':
		if j := strings.IndexByte(src[i+2:], '\n'); j >= 0 {
			return i + 2 + j
		}
		return len(src)
	case '*':
		depth := 1
		for j := i + 2; j+1 < len(src); j++ {
			switch {
			case src[j] == '/' && src[j+1] == '*':
				depth++
				j++
			case src[j] == '*' && src[j+1] == '/':
				j++
				if depth--; depth == 0 {
					return j + 1
				}
			}
		}
		return len(src)
	}
	return i
}

// stringEnd returns the end of the string literal at i: after the closing
// quote or, while there is none, at the end of the line.
func stringEnd(src string, i int) int {
	for j := i + 1; j < len(src); j++ {
		switch src[j] {
		case '"':
			return j + 1
		case '\n':
			return j
		case '\\':
			if j+1 < len(src) && src[j+1] != '\n' {
				j++
			}
		}
	}
	return len(src)
}

// numberEnd returns the end of the number at i with its fraction, exponent
// and unit or percent sign: 12pt, 1.5em, 1e3, 50%. Letters after the digits
// are taken for the unit, which also covers 0xff.
func numberEnd(src string, i int) int {
	j := digitsEnd(src, i)
	if j+1 < len(src) && src[j] == '.' && isDigit(src[j+1]) {
		j = digitsEnd(src, j+1)
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1
		if k < len(src) && (src[k] == '+' || src[k] == '-') {
			k++
		}
		if k < len(src) && isDigit(src[k]) {
			j = digitsEnd(src, k)
		}
	}
	if j < len(src) && src[j] == '%' {
		return j + 1
	}
	for j < len(src) && ('a' <= src[j] && src[j] <= 'z' || 'A' <= src[j] && src[j] <= 'Z') {
		j++
	}
	return j
}

// identEnd returns the end of the identifier at i, or i when none starts
// there. An identifier starts with a letter or an underscore and goes on
// with letters, digits, underscores and hyphens; a hyphen counts only
// before a letter, so that a minus before a number stays out of the name.
func identEnd(src string, i int) int {
	j := i
	for j < len(src) {
		c := src[j]
		switch {
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_':
			j++
		case '0' <= c && c <= '9':
			if j == i {
				return i
			}
			j++
		case c == '-':
			if j == i || !letterAt(src, j+1) {
				return j
			}
			j++
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRuneInString(src[j:])
			if !unicode.IsLetter(r) && (j == i || !unicode.IsDigit(r) && !unicode.IsMark(r)) {
				return j
			}
			j += size
		default:
			return j
		}
	}
	return j
}

// letterAt reports whether a letter or an underscore is at i.
func letterAt(src string, i int) bool {
	if i >= len(src) {
		return false
	}
	if c := src[i]; c < utf8.RuneSelf {
		return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_'
	}
	r, _ := utf8.DecodeRuneInString(src[i:])
	return unicode.IsLetter(r)
}

// spaceOrEnd reports whether white space or the end of the text is at i.
func spaceOrEnd(src string, i int) bool {
	return i >= len(src) || src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r'
}

func digitsEnd(src string, i int) int {
	for i < len(src) && isDigit(src[i]) {
		i++
	}
	return i
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func isHex(c byte) bool { return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' }

func isAlnum(r rune) bool {
	if r < utf8.RuneSelf {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
