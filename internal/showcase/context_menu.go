package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "context-menu", Group: "dialogs", Title: "Context Menu", Summary: "The actions of one thing, opened with a long press or the secondary mouse button.", Width: 480, Height: 460, FullWindow: true,
		// A long press on the picture: the finger stays down for the
		// screenshot, as it would on a phone.
		Input: `{"steps":[{"op":"waitFrames","frames":30},{"op":"pointerDown","x":124,"y":96},{"op":"waitMs","ms":700},{"op":"waitFrames","frames":40}]}`},
		contextMenuDemo)
}

func contextMenuDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return ui.VStack(
		ui.ContextMenu(
			ui.Image(pictures[0]).Fit(ui.FitCover).Frame(200, 150).CornerRadius(12).Clip(true),
			ui.MenuItem("Copy", func() {}).Icon(outline.FileCopy),
			ui.MenuItem("Share", func() {}).Icon(outline.ShareNodes),
			ui.MenuItem("Add to album", func() {}).Disabled(true),
			ui.MenuDivider(),
			ui.MenuItem("Delete", func() {}).Icon(outline.TrashBin).Destructive(),
		).CornerRadius(12),
	).Padding(24).Align(geom.Alignment{X: 0.5, Y: 0})
	// snippet:end
}
