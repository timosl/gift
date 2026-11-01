---
title: Code Editor
---

A code editor lets the user edit source code: a configuration, a script, a template, the markup of a document. It
is a [Text Area](../text-area/) over the same `ui.TextEditor`, with what a code editor adds: the numbers of the lines
in a gutter, the text colored by its syntax, and marks for the places a parser or a compiler complained about,
whose lines are tinted and whose text is underlined.

{{< gallery-shot alt="Go source in a monospaced face with line numbers, keywords, strings, comments and function names in their own colors, and one line tinted red with the misspelled call in it underlined" >}}

{{< gallery-code >}}

## Usage

```go
src := ui.Editor(ctx, "src", "")
ui.CodeEditor(src).Highlighter(syntax.Go).Marks(marks...)
```

The editor, `Bind`, `OnChange`, `Lines`, `Placeholder`, `Label`, `Disabled` and the box modifiers work as in a
[Text Area](../text-area/). The text is set in IBM Plex Mono when the program imports
[`font/ibmplexmono`](https://github.com/worldiety/gift/tree/main/font/ibmplexmono), at 13 points, and in the default
font otherwise; `Font` and `FontSize` choose another.

### Highlighting

A `ui.Highlighter` splits the text into tokens of a few kinds that most languages share: keywords, strings, numbers,
comments, function and type names, and for markup languages headings, emphasis, other markup, raw text and math. The
editor colors each kind, in the light and the dark theme. Package
[`ui/syntax`](https://github.com/worldiety/gift/tree/main/ui/syntax) has highlighters for Go and for
[Typst](../../documents/typst-editor/); `ui.HighlighterFunc` turns a function into one. A highlighter runs after every
edit and must accept any text, half typed code included.

### Marks

`Marks` takes `ui.CodeMark`s, each a range of byte offsets into the text and a color, red unless set. The lines a
mark touches are tinted, its text is underlined and its line number takes its color. A mark is not moved by edits:
compute the marks from the text you are given, as a parser does, and pass them in every build.

```go
var marks []ui.CodeMark
for _, e := range parse(src.Text()) {
	marks = append(marks, ui.CodeMark{Start: e.Start, End: e.End})
}
```

### Keyboard

Everything a text area does, a code editor does, with three keys of its own. Tab indents the line of the caret, or
every line of a selection, and shift-tab takes the indentation away; `Indent` sets what one step is, four spaces
unless set. Enter starts the new line at the indentation of the old one. Because tab indents, escape and then tab
moves the focus on, so that the keyboard does not get stuck in the editor.

### What it is not

There are no tab stops: a tab is drawn as wide as one character, which is why the editor indents with spaces. Long
lines wrap; a wrapped line has its number on its first row. There is no undo, and no folding or completion.

## API

{{< api "CodeEditorView" >}}

{{< api type="CodeMark" decl="true" >}}

{{< api type="Token" decl="true" >}}

{{< api type="TokenKind" decl="true" >}}

## Related

- [Text Area](../text-area/), for prose
- [Typst Editor](../../documents/typst-editor/), which edits the document a Typst source sets on its pages
- [Typst example](../../../examples/example-typst/), a code editor with Typst highlighting and the errors of Typst
