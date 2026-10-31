package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

func init() {
	register(Demo{Name: "image-viewer", Group: "media", Width: 720, Height: 420, Settle: 40, FullWindow: true}, imageViewerDemo)
}

// demoViewer is a viewer over the sample pictures, opened on the second one.
func demoViewer(ctx *gift.Context) *imageviewer.Viewer {
	slot := ctx.State("viewer", (*imageviewer.Viewer)(nil))
	if v := slot.Get(); v != nil {
		return v
	}
	items := make([]asset.Metadata, len(pictures))
	byID := make(map[asset.ID]asset.Source, len(pictures))
	for i, p := range pictures {
		items[i] = p.Metadata()
		byID[items[i].ID] = p
	}
	v := imageviewer.New(asset.NewCollection(items), func(id asset.ID) asset.Source { return byID[id] })
	v.Open(items[1].ID)
	slot.Set(v)
	return v
}

func imageViewerDemo(ctx *gift.Context) gift.View {
	v := demoViewer(ctx)
	// snippet:begin
	return ui.ZStack(
		ui.Box(), // the gallery or whatever the viewer opens from
		imageviewer.Fullscreen(v).
			Card(func(id asset.ID) gift.View {
				return ui.Text(string(id)).Padding(24)
			}),
	)
	// snippet:end
}
