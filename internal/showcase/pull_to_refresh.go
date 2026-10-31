package showcase

import (
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input pulls the list down far enough to start a refresh and holds
	// it there, which shows the turning spinner.
	register(Demo{
		Name: "pull-to-refresh", Group: "lists", Title: "Pull to Refresh", Summary: "A refresh started by pulling a scroll view down past its top.", Width: 440, Height: 340, FullWindow: true,
		Input: `{"steps":[{"op":"waitFrames","frames":2},{"op":"pointerDown","x":220,"y":60},` +
			`{"op":"waitFrames","frames":1},{"op":"pointerMove","x":220,"y":90},{"op":"waitFrames","frames":1},` +
			`{"op":"pointerMove","x":220,"y":130},{"op":"waitFrames","frames":1},{"op":"pointerMove","x":220,"y":170},` +
			`{"op":"waitFrames","frames":1},{"op":"pointerMove","x":220,"y":210},{"op":"waitFrames","frames":1},` +
			`{"op":"pointerMove","x":220,"y":250},{"op":"waitFrames","frames":8}]}`,
	}, pullToRefreshDemo)
}

func pullToRefreshDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	busy := ctx.State("busy", false)
	loads := ctx.State("loads", 1)
	// The token posts the end of the refresh back to this component; it is
	// asked for only while idle, so that the rebuild the refresh causes does
	// not replace it.
	var tok gift.Token
	if !ctx.Read(busy) {
		tok = ctx.Request()
	}
	refresh := func() {
		busy.Set(true)
		go func() {
			time.Sleep(1500 * time.Millisecond) // the network, say
			tok.Post(func() { loads.Set(loads.Get() + 1); busy.Set(false) })
		}()
	}
	rows := make([]gift.View, 12)
	for i := range rows {
		rows[i] = ui.Row("Item " + strconv.Itoa(i+1)).Value("load " + strconv.Itoa(ctx.Read(loads)))
	}
	return ui.VScroll(ui.List(rows...)).Refreshable(busy.Get(), refresh)
	// snippet:end
}
