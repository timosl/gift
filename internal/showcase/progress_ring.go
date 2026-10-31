package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "progress-ring", Group: "basic", Title: "Progress Ring", Summary: "How much of a task is done as a ring, or the spinner of iOS.", Width: 440, Height: 140}, progressRingDemo)
}

func progressRingDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return ui.HStack(
		ui.ProgressRing(0.25),
		ui.ProgressRing(0.6).Tint(ui.RGB(52, 199, 89)),
		ui.ProgressRing(0.9).Size(56).LineWidth(8),
		// No idea how far along: the spinner of iOS.
		ui.ProgressRing(0).Indeterminate(),
		ui.ProgressRing(0).Indeterminate().Size(40),
	).Gap(28).Align(geom.Center)
	// snippet:end
}
