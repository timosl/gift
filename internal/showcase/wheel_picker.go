package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "wheel-picker", Group: "basic", Title: "Wheel Picker", Summary: "One or more turning wheels to choose values from, as on iOS.", Width: 420, Height: 260}, wheelPickerDemo)
}

func wheelPickerDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	sizes := []string{"XS", "S", "M", "L", "XL", "XXL"}
	colours := []string{"Black", "White", "Navy", "Forest", "Sand", "Red"}
	size := ctx.State("size", 2)
	colour := ctx.State("colour", 1)
	return ui.WheelPicker(
		ui.Wheel(len(sizes), func(i int) string { return sizes[i] }).
			Bind(size.Binding()).Width(100),
		ui.Wheel(len(colours), func(i int) string { return colours[i] }).
			Bind(colour.Binding()).Width(160),
	).Background(ui.ColorBackground)
	// snippet:end
}
