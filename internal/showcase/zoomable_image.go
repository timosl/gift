package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

func init() {
	register(Demo{Name: "zoomable-image", Group: "media", Title: "Zoomable Image", Summary: "One picture to pinch, double tap and wheel into, in place.", Width: 520, Height: 360, Settle: 30}, zoomableImageDemo)
}

func zoomableImageDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return imageviewer.Zoomable(pictures[0]).
		Background(ui.Fade(ui.ColorLabel, 0.06)).
		Frame(480, 320).
		Label("Sample picture")
	// snippet:end
}
