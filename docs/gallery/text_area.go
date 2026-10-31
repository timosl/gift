package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input focuses the first area and selects from the caret, at the
	// end, back up a line with shift, which shows the caret and a selection
	// over two lines.
	register(Demo{
		Name: "text-area", Group: "basic", Width: 520, Height: 420,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"focus"},{"op":"key","key":"up","mods":["shift"]},{"op":"waitFrames","frames":2}]}`,
	}, textAreaDemo)
}

func textAreaDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	notes := ui.Editor(ctx, "notes", "Pick up the prints on Friday.\nCall the framer about the oak frames, "+
		"the ones for the large landscapes.")
	message := ui.Editor(ctx, "message", "")

	labeled := func(title string, field gift.View) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			field,
		).Gap(4).Align(geom.Alignment{X: 0, Y: 0})
	}

	return ui.VStack(
		// Grows with its text from three lines to six, then scrolls.
		labeled("Notes", ui.TextArea(notes).Lines(3, 6)),
		labeled("Message", ui.TextArea(message).Placeholder("Write a message").Lines(4, 4)),
	).Gap(14).Frame(340, geom.Unbounded())
	// snippet:end
}
