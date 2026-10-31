package showcase

import (
	"errors"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/videoplayer"
	"github.com/worldiety/gift/video"
)

func init() {
	register(Demo{Name: "video-player", Group: "media", Title: "Video Player", Summary: "A video with sound and touch controls for play, pause, position, volume and the audio track, or the bare picture without them.", Width: 720, Height: 420, Settle: 30}, videoPlayerDemo)
}

// demoPlayer opens the sample clip once and seeks into it, so that the
// screenshot shows a picture and not the first frame, and closes it when the
// demo leaves the screen. It reports why when there is no player: FFmpeg is
// loaded at run time and may be missing.
func demoPlayer(ctx *gift.Context) (*video.Player, error) {
	slot := ctx.State("player", (*video.Player)(nil))
	if p := slot.Get(); p != nil {
		return p, nil
	}
	if clipPath == "" {
		return nil, errors.New("the sample clip was not written; see showcase.Samples")
	}
	p, err := video.Open(clipPath, video.Options{})
	if err != nil {
		return nil, err
	}
	p.Seek(2200 * time.Millisecond)
	slot.Set(p)
	ctx.OnUnmount("player", func() { p.Close() })
	return p, nil
}

func videoPlayerDemo(ctx *gift.Context) gift.View {
	p, err := demoPlayer(ctx)
	if err != nil {
		return ui.Text("No video: "+err.Error()).Foreground(ui.ColorSecondaryLabel).Frame(640, 360)
	}
	// snippet:begin
	return ui.ZStack(
		videoplayer.New(p).
			AutoHide(0).
			CornerRadius(16),
	).Frame(640, 360)
	// snippet:end
}
