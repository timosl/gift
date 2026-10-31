package showcase

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "calendar", Group: "basic", Title: "Calendar", Summary: "A date on a month grid that pages sideways.", Width: 400, Height: 380}, calendarDemo)
}

func calendarDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	day := ctx.State("day", ui.DateOf(2026, time.October, 31))
	busy := func(d ui.Date) bool { return d.Day()%7 == 3 }
	return ui.Calendar().Bind(day.Binding()).Marked(busy).Background(ui.ColorSurface).Width(360)
	// snippet:end
}
