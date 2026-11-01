package showcase

import (
	"strings"

	"github.com/worldiety/gift"
	_ "github.com/worldiety/gift/font/ibmplexmono"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/syntax"
)

func init() {
	register(Demo{
		Name: "code-editor", Group: "basic", Title: "Code Editor",
		Summary: "Source code with line numbers, colours by its syntax and marks where it is wrong.", Width: 620, Height: 380,
	}, codeEditorDemo)
}

const goSample = `package main

import "fmt"

// greet says hello to everyone in names.
func greet(names []string) int {
    for i, name := range names {
        fmt.Printn(i, "Hello,", name)
    }
    return len(names)
}
`

func codeEditorDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	code := ctx.State("code", goSample)
	ctx.Read(code)
	src := ui.Editor(ctx, "editor", goSample)

	// A real program asks its compiler; here the typo is the error, and the
	// mark goes away once it is fixed.
	var marks []ui.CodeMark
	if i := strings.Index(code.Get(), "Printn"); i >= 0 {
		marks = append(marks, ui.CodeMark{Start: i, End: i + len("Printn")})
	}
	return ui.CodeEditor(src).
		Highlighter(syntax.Go).
		Marks(marks...).
		Bind(code.Binding()).
		Lines(12, 12).
		Frame(540, geom.Unbounded())
	// snippet:end
}
