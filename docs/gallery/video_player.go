package main

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/videoplayer"
	"github.com/worldiety/gift/video"
)

func init() {
	register(Demo{Name: "video-player", Group: "media", Width: 720, Height: 420, Settle: 30}, videoPlayerDemo)
}

// demoPlayer opens the clip of the video package once and seeks into it, so
// that the screenshot shows a picture and not the first frame. The player is
// never closed: the demo is its own process.
func demoPlayer(ctx *gift.Context) *video.Player {
	slot := ctx.State("player", (*video.Player)(nil))
	if p := slot.Get(); p != nil {
		return p
	}
	p, err := video.Open("video/testdata/bbb.mp4", video.Options{})
	if err != nil {
		panic(err)
	}
	p.Seek(2200 * time.Millisecond)
	slot.Set(p)
	return p
}

func videoPlayerDemo(ctx *gift.Context) gift.View {
	p := demoPlayer(ctx)
	// snippet:begin
	return ui.ZStack(
		videoplayer.New(p).
			AutoHide(0).
			CornerRadius(16),
	).Frame(640, 360)
	// snippet:end
}
