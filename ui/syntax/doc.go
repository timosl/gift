// Package syntax colours source code for [ui.CodeEditor].
//
// Each language is a [ui.Highlighter] in a variable named after it, which the
// editor takes as it is:
//
//	src := ui.Editor(ctx, "src", "package main\n")
//	ui.CodeEditor(src).Highlighter(syntax.Go)
//
// The package exists so that ui itself carries no lexer. A highlighter is
// code and tables that every binary linking ui would carry, whether it edits
// source code or not; kept here, a program that does not import this package
// pays nothing for them, in the same way that a program which does not
// import font/inter carries no typeface.
//
// # Lexers, not parsers
//
// A highlighter runs on the UI goroutine after every edit, over the whole
// text, and most of the texts it sees do not parse, because they are being
// typed. The highlighters here therefore lex in a single pass, in time linear
// in the length of the text, and recognise only what a few bytes of context
// can tell. They cannot fail: what they do not recognise stays plain, and
// what is not closed yet, a string, a comment or a bracket, is coloured as
// far as each highlighter's documentation says. None of them keeps state
// between calls, so one value serves any number of editors, on any number of
// goroutines.
package syntax
