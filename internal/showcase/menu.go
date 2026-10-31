package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "menu", Group: "dialogs", Title: "Menu", Summary: "A control that opens a list of actions, chosen with a tap or with one press, slide and lift.", Width: 440, Height: 420, FullWindow: true,
		// A tap on the button opens the menu for the screenshot.
		Input: `{"steps":[{"op":"waitFrames","frames":4},{"op":"tap","x":52,"y":44},{"op":"waitFrames","frames":40}]}`},
		menuDemo)
}

func menuDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	sort := ctx.State("sort", "Date")
	ctx.Read(sort)
	by := func(s string) ui.MenuItemView {
		return ui.MenuItem(s, func() { sort.Set(s) }).Checked(sort.Get() == s)
	}
	return ui.VStack(
		ui.Menu(
			ui.HStack(ui.Icon(outline.DotsHorizontal), ui.Text("Options")).Gap(6).Padding(10),
			ui.MenuItem("Share", func() {}).Icon(outline.ShareNodes),
			ui.MenuItem("Bookmark", func() {}).Icon(outline.Bookmark),
			ui.MenuSection("Sort by", by("Date"), by("Name"), by("Size")),
			ui.MenuSection("",
				ui.MenuItem("Delete", func() {}).Icon(outline.TrashBin).Destructive(),
			),
		).Label("Options"),
	).Padding(14).Align(geom.TopLeading)
	// snippet:end
}
