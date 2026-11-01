package showcase

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	_ "github.com/worldiety/gift/typst/embedded"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/typstview"
)

func init() {
	register(Demo{
		Name: "typst-pdf", Group: "documents", Title: "Typst PDF",
		Summary: "The pages of a Typst document on the screen, and the same pages as a PDF file.",
		Width:   760, Height: 560, FullWindow: true, Settle: 30,
	}, typstPDFDemo)
}

const invoice = `#set page(width: 420pt, height: 300pt, margin: 28pt, numbering: "1 / 1")
#set text(font: "Inter", size: 10pt)

#grid(columns: (1fr, auto), [= Invoice 2026-117], align(right)[1 November 2026])

#table(
  columns: (1fr, auto, auto),
  align: (left, right, right),
  stroke: 0.5pt + rgb("#9aa5b4"),
  fill: (_, y) => if y == 0 { rgb("#e8eef7") },
  table.header([*Item*], [*Hours*], [*Amount*]),
  [Workshop], [8], [960.00 €],
  [Design of the editor], [24], [2,880.00 €],
  [Typst integration], [40], [4,800.00 €],
  table.cell(colspan: 2, align: right)[*Total*], [*8,640.00 €*],
)

Payable within 14 days. Thank you for the good work together.
`

func typstPDFDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	doc := typstview.Use(ctx, "invoice", invoice)
	saved := ctx.State("saved", "")
	ctx.Read(saved)
	path := filepath.Join(os.TempDir(), "gift-invoice.pdf")

	export := func() {
		// Typst's own PDF writer, from the pages on the screen. A long
		// document would be exported on a goroutine.
		pdf, err := doc.PDF()
		if err == nil {
			err = os.WriteFile(path, pdf, 0o644)
		}
		if err != nil {
			saved.Set(err.Error())
			return
		}
		saved.Set(fmt.Sprintf("%s, %d KB", path, (len(pdf)+1023)/1024))
	}
	open := func() {
		cmd := "xdg-open"
		if runtime.GOOS == "darwin" {
			cmd = "open"
		}
		_ = exec.Command(cmd, path).Start()
	}

	bar := []gift.View{ui.Button(ui.Text("Export PDF"), export)}
	if saved.Get() != "" {
		bar = append(bar,
			ui.Text(saved.Get()).FontSize(13).Foreground(ui.ColorSecondaryLabel).Flex(1),
			ui.Button(ui.Text("Open"), open))
	}
	return ui.VStack(
		ui.HStack(bar...).Gap(12).Align(geom.Center).Padding(12),
		ui.Divider(),
		typstview.Viewer(doc).Flex(1),
	).Gap(0)
	// snippet:end
}
