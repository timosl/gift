package main

import (
	"strings"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input types a term, which brings up the clear button and filters
	// the list.
	register(Demo{
		Name: "search-field", Group: "basic", Width: 440, Height: 300,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"focus"},{"op":"text","text":"er"},{"op":"waitFrames","frames":2}]}`,
	}, searchFieldDemo)
}

func searchFieldDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	q := ctx.State("q", "")
	term := strings.ToLower(ctx.Read(q))
	var rows []gift.View
	for _, city := range []string{"Amsterdam", "Berlin", "Lisbon", "Madrid", "Oldenburg", "Vienna"} {
		if strings.Contains(strings.ToLower(city), term) {
			rows = append(rows, ui.Text(city))
		}
	}
	return ui.VStack(
		ui.SearchField(ui.Editor(ctx, "q-editor", "")).Placeholder("Search cities").Bind(q.Binding()),
		ui.VStack(rows...).Gap(8).Padding(4),
	).Gap(14).Align(geom.Alignment{X: 0, Y: 0}).Frame(340, geom.Unbounded())
	// snippet:end
}
