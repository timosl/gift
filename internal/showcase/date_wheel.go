package showcase

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "date-wheel", Group: "basic", Title: "Date and Time Wheels", Summary: "A date or a time of day on wheels.", Width: 560, Height: 260}, dateWheelDemo)
}

func dateWheelDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	day := ctx.State("day", ui.DateOf(2026, time.October, 31))
	at := ctx.State("at", ui.TimeOf(14, 30))
	return ui.HStack(
		ui.DateWheel().Bind(day.Binding()).Background(ui.ColorBackground),
		ui.TimeWheel().Bind(at.Binding()).MinuteInterval(5).Background(ui.ColorBackground),
	).Gap(16)
	// snippet:end
}
