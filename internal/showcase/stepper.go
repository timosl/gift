package showcase

import (
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "stepper", Group: "basic", Title: "Stepper", Summary: "A minus and a plus button that change a number a step at a time.", Width: 440, Height: 200}, stepperDemo)
}

func stepperDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	tickets := ctx.State("tickets", 2.0)
	weight := ctx.State("weight", 1.5)
	nights := ctx.State("nights", 14.0)

	row := func(label string, stepper gift.View) gift.View {
		return ui.HStack(ui.Text(label).Flex(1), stepper).Align(geom.Center)
	}
	return ui.VStack(
		row("Tickets: "+strconv.Itoa(int(ctx.Read(tickets))),
			ui.Stepper(tickets.Get(), tickets.Set).Range(1, 10)),
		row("Weight: "+strconv.FormatFloat(ctx.Read(weight), 'f', 1, 64)+" kg",
			ui.Stepper(weight.Get(), weight.Set).Range(0.5, 5).Step(0.5)),
		// At the end of its range the plus half is greyed out.
		row("Nights: "+strconv.Itoa(int(ctx.Read(nights))),
			ui.Stepper(nights.Get(), nights.Set).Range(1, 14)),
	).Gap(16).Frame(340, geom.Unbounded())
	// snippet:end
}
