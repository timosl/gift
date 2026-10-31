package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input taps the button, and the screenshot is taken while the
	// toast is on its way in and settled.
	register(Demo{
		Name: "toast", Group: "dialogs", Title: "Toast", Summary: "A short message that slides in at the edge of the window and goes again by itself.", Width: 440, Height: 260, FullWindow: true,
		Input: `{"steps":[{"op":"waitFrames","frames":4},{"op":"tap","x":220,"y":225},{"op":"waitFrames","frames":45}]}`,
	}, toastDemo)
}

func toastDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	copied := ctx.State("copied", false)
	return ui.Toast(
		ui.ZStack(
			ui.Button(ui.Text("Copy link"), func() { copied.Set(true) }),
		).Padding(20).Frame(440, 260).Align(geom.Bottom),
		"Link copied",
	).Subtitle("gift.dev/docs").Icon(outline.Link).
		Presented(ctx.Read(copied)).
		OnDismiss(func() { copied.Set(false) })
	// snippet:end
}
