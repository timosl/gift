package ui

// TokenKind is what a stretch of source code is, which decides its colour in
// a [CodeEditorView]. The kinds are those most languages share; a highlighter
// maps its own grammar onto them.
type TokenKind uint8

// The kinds of [Token]. Text no token covers is TokenPlain.
const (
	TokenPlain    TokenKind = iota
	TokenKeyword            // a reserved word: if, for, let, func
	TokenString             // a string or character literal
	TokenNumber             // a number, with its unit where the language has units
	TokenComment            // a comment
	TokenFunction           // the name of a function where it is called or defined
	TokenType               // the name of a type
	TokenHeading            // a heading of a markup language
	TokenEmphasis           // emphasised or strong text with its delimiters
	TokenMarkup             // other markup: list markers, escapes, labels, references
	TokenRaw                // raw or verbatim text inside markup
	TokenMath               // a formula inside markup

	tokenKinds
)

// Token is a stretch of source code of one kind, by byte offsets: Start is
// the first byte and End the one after the last.
type Token struct {
	Start, End int
	Kind       TokenKind
}

// Highlighter splits source code into tokens for a [CodeEditorView].
//
// Highlight appends the tokens of src to dst and returns the extended slice.
// The tokens are in order and do not overlap; text between them is plain.
// It is called on the UI goroutine after an edit, so it has to be quick and
// must accept any text, including half typed code that does not parse:
// a highlighter colours what it recognises and leaves the rest plain.
type Highlighter interface {
	Highlight(src string, dst []Token) []Token
}

// HighlighterFunc adapts a function to [Highlighter].
type HighlighterFunc func(src string, dst []Token) []Token

// Highlight implements [Highlighter].
func (f HighlighterFunc) Highlight(src string, dst []Token) []Token { return f(src, dst) }
