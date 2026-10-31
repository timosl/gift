package showcase

import (
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "circular-slider", Group: "basic", Title: "Circular Slider", Summary: "A slider on a ring, for a thermostat or a dial.", Width: 320, Height: 260}, circularSliderDemo)
}

func circularSliderDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	t := ctx.State("t", 21.5)
	return ui.ZStack(
		ui.CircularSlider(t.Get(), t.Set).Range(16, 28).Step(0.5).Size(200).
			Tint(ui.RGB(255, 149, 0)).Label("Temperature"),
		ui.VStack(
			ui.Text(strconv.FormatFloat(ctx.Read(t), 'f', 1, 64)+"°").FontSize(40),
			ui.Text("Living room").FontSize(14).Foreground(ui.ColorSecondaryLabel),
		).Gap(2),
	).Align(geom.Center)
	// snippet:end
}
