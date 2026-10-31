package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "popover", Group: "dialogs", Width: 520, Height: 320, FullWindow: true, Settle: 40}, popoverDemo)
}

func popoverDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The application owns whether the popover is up; the demo starts with it
	// presented.
	open := ctx.State("open", true)
	return ui.VStack(
		ui.Popover(
			ui.Button(ui.Text("Opening hours"), func() { open.Set(true) }).CornerRadius(8),
			ui.VStack(
				ui.Text("Opening hours").FontSize(17),
				ui.Text("Monday to Friday, 9 to 17").Foreground(ui.ColorSecondaryLabel),
				ui.Text("Saturday, 10 to 14").Foreground(ui.ColorSecondaryLabel),
			).Gap(6),
		).Presented(ctx.Read(open)).OnDismiss(func() { open.Set(false) }),
	).Padding(24).Align(geom.Alignment{X: 0.5, Y: 0})
	// snippet:end
}
