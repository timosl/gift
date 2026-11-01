package showcase

import (
	"github.com/worldiety/gift"
	_ "github.com/worldiety/gift/typst/embedded"
	"github.com/worldiety/gift/ui/typstview"
)

func init() {
	register(Demo{
		Name: "typst-editor", Group: "documents", Title: "Typst Editor",
		Summary: "A rich text document in Typst, edited on the page it is set on.",
		Width:   820, Height: 600, FullWindow: true, Settle: 30,
	}, typstEditorDemo)
}

const releaseNotes = `#set page(width: 420pt, height: auto, margin: 32pt)
#set text(font: "Inter", size: 11pt)

= Release notes

gift now sets *rich text* with _Typst_. Click into this page and type: every keystroke sets the page again, and the toolbar formats the word at the caret.

- Headings, lists and tables
- Strong and emphasised text
- Export to PDF

#table(
  columns: (1fr, auto),
  table.header([*Feature*], [*Status*]),
  [Editing on the page], [done],
  [PDF export], [done],
)
`

func typstEditorDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	doc := typstview.Use(ctx, "notes", releaseNotes)
	return typstview.Editor(doc).Flex(1)
	// snippet:end
}
