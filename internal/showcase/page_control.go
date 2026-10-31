package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "page-control", Group: "basic", Title: "Page Control", Summary: "A row of dots that shows and turns the current page.", Width: 440, Height: 300}, pageControlDemo)
}

func pageControlDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	page := ctx.State("page", 1)
	photo := ctx.State("photo", 2)
	return ui.VStack(
		ui.PageControl(5, ctx.Read(page), page.Set),
		// Over a picture the dots sit on a dark capsule, in white.
		ui.ZStack(
			ui.Image(pictures[2]).Fit(ui.FitCover).Frame(300, 170).CornerRadius(12).Clip(true),
			ui.VStack(ui.PageControl(4, ctx.Read(photo), photo.Set).Capsule(true)).
				Padding(10),
		).Align(geom.Bottom),
	).Gap(20)
	// snippet:end
}
