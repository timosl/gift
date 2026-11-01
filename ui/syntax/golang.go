package syntax

import (
	"go/scanner"
	"go/token"
	"strings"

	"github.com/worldiety/gift/ui"
)

// Go colours Go source code.
//
// It is built on the standard library's go/scanner, so it knows Go's lexical
// grammar exactly, comments included: keywords and the predeclared constants
// true, false, nil and iota are keywords, string and rune literals strings,
// and integer, floating-point and imaginary literals numbers. On top of the
// tokens it looks one token ahead: an identifier followed by a parenthesis
// names a function, and so does the identifier right after func, which
// covers a generic function before its type parameters. The predeclared
// types, from bool to uintptr, are types; other type names are not told
// apart from other identifiers, which would take a parser.
//
// Scanner errors are ignored. Half typed code is full of them, and the
// scanner goes on past each one with the next token it can make out.
//
// The scanner allocates a string for every identifier, literal and comment,
// so unlike [Typst] this highlighter allocates in proportion to its input.
var Go ui.Highlighter = ui.HighlighterFunc(highlightGo)

func highlightGo(src string, dst []ui.Token) []ui.Token {
	file := token.NewFileSet().AddFile("", -1, len(src))
	var s scanner.Scanner
	s.Init(file, []byte(src), nil, scanner.ScanComments)

	// call is an identifier whose kind is decided by the token after it: it
	// names a function when that token is a parenthesis, and is plain
	// otherwise. Its End is 0 while there is none.
	var call ui.Token
	afterFunc := false
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return dst
		}
		if call.End != 0 {
			if tok == token.LPAREN {
				dst = append(dst, call)
			}
			call = ui.Token{}
		}

		// file.Offset turns the scanner's 1-based file position into a byte
		// offset into src, and clamps it to the length of src.
		start := file.Offset(pos)
		var kind ui.TokenKind
		end := start + len(lit)
		switch {
		case tok == token.IDENT:
			kind = goIdentKind(lit)
			if kind == ui.TokenPlain {
				if afterFunc {
					kind = ui.TokenFunction
				} else {
					call = ui.Token{Start: start, End: end, Kind: ui.TokenFunction}
				}
			}
		case tok.IsKeyword():
			kind = ui.TokenKeyword
		case tok == token.STRING || tok == token.CHAR:
			kind = ui.TokenString
			if lit != "" && lit[0] == '`' {
				end = goRawStringEnd(src, start)
			}
		case tok == token.INT || tok == token.FLOAT || tok == token.IMAG:
			kind = ui.TokenNumber
		case tok == token.COMMENT:
			kind = ui.TokenComment
			end = goCommentEnd(src, start)
		}
		if kind != ui.TokenPlain && start < end && end <= len(src) {
			dst = append(dst, ui.Token{Start: start, End: end, Kind: kind})
		}
		afterFunc = tok == token.FUNC
	}
}

// goIdentKind is the kind of a predeclared identifier, and TokenPlain for
// every other one.
func goIdentKind(ident string) ui.TokenKind {
	switch ident {
	case "true", "false", "nil", "iota":
		return ui.TokenKeyword
	case "bool", "byte", "rune", "string", "error", "any", "comparable",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64", "complex64", "complex128":
		return ui.TokenType
	}
	return ui.TokenPlain
}

// goRawStringEnd returns the end of the raw string literal that starts at
// i. It is measured in src because the scanner drops carriage returns from
// the literal it returns, which would make the literal's length too short.
func goRawStringEnd(src string, i int) int {
	if j := strings.IndexByte(src[i+1:], '`'); j >= 0 {
		return i + 1 + j + 1
	}
	return len(src)
}

// goCommentEnd returns the end of the comment that starts at i, measured in
// src for the reason given at [goRawStringEnd]. A line comment ends before
// its line break.
func goCommentEnd(src string, i int) int {
	if strings.HasPrefix(src[i:], "/*") {
		if j := strings.Index(src[i+2:], "*/"); j >= 0 {
			return i + 2 + j + 2
		}
		return len(src)
	}
	if j := strings.IndexByte(src[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(src)
}
