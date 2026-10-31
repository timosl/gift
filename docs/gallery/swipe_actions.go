package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input drags the second row to the left and lets it go open, which
	// shows its actions.
	register(Demo{
		Name: "swipe-actions", Group: "lists", Width: 440, Height: 300,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"pointerDown","x":360,"y":120},` +
			`{"op":"waitFrames","frames":1},{"op":"pointerMove","x":340,"y":120},{"op":"waitFrames","frames":1},` +
			`{"op":"pointerMove","x":300,"y":120},{"op":"waitFrames","frames":1},{"op":"pointerMove","x":250,"y":120},` +
			`{"op":"waitFrames","frames":1},{"op":"pointerMove","x":200,"y":120},{"op":"waitFrames","frames":6},` +
			`{"op":"pointerUp","x":200,"y":120},{"op":"waitFrames","frames":40}]}`,
	}, swipeActionsDemo)
}

type mail struct{ from, subject, time string }

func swipeActionsDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	inbox := ctx.State("inbox", 0) // bumped on every change of mails
	ctx.Read(inbox)
	var rows []gift.View
	for i, m := range mails {
		rows = append(rows, ui.Swipe(
			ui.Row(m.from).Subtitle(m.subject).Value(m.time),
		).Trailing(
			ui.SwipeAction("Delete", func() {
				mails = append(mails[:i:i], mails[i+1:]...)
				inbox.Set(inbox.Get() + 1)
			}).Icon(outline.TrashBin).Destructive(),
			ui.SwipeAction("Flag", func() {}).Icon(outline.Flag).Tint(ui.RGB(255, 149, 0)),
		).Leading(
			ui.SwipeAction("Unread", func() {}).Icon(outline.Envelope).Tint(ui.ColorAccent),
		).Background(ui.ColorSurface).Key(m.from))
	}
	return ui.VStack(ui.Card(ui.List(rows...)).Padding(0)).Frame(400, geom.Unbounded())
	// snippet:end
}

var mails = []mail{
	{"Ada Lovelace", "Notes on the engine", "09:41"},
	{"Grace Hopper", "The bug in relay 70", "Yesterday"},
	{"Alan Turing", "On computable numbers", "Monday"},
	{"Edsger Dijkstra", "A case against goto", "Sunday"},
}
