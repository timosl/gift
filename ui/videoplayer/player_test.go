package videoplayer

import (
	"testing"
	"time"

	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/video"
	"github.com/worldiety/gift/video/ffmpeg"
)

func TestMain(m *testing.M) { gifttest.Main(m) }

// openClip opens the two track clip of the video package, or skips where
// FFmpeg is not installed.
func openClip(t *testing.T) *video.Player {
	t.Helper()
	if !ffmpeg.Available() {
		t.Skip("FFmpeg is not installed")
	}
	p, err := video.Open("../../internal/sampleclip/bbb.mp4", video.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// mount puts v in a window of the size of a five inch panel.
func mount(t *testing.T, v PlayerView) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{
		View:  v,
		Size:  geom.Sz(800, 480),
		Theme: ui.DarkTheme(),
		Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	h.Settle()
	return h
}

func TestControlsFitAFiveInchPanel(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	for _, k := range []string{"play", "back", "forward", "mute", "volume", "tracks", "scrubber", "elapsed", "remaining"} {
		h.AssertExists(gifttest.ByKey(k))
	}
	h.AssertNoOverflow()
	// Every button is at least a finger wide.
	for _, k := range []string{"play", "back", "forward", "mute", "tracks"} {
		if b := h.Find(gifttest.ByKey(k)).LayoutBounds(); b.Width() < 44 || b.Height() < 44 {
			t.Errorf("%s is %vx%v, smaller than a 44 pixel touch target", k, b.Width(), b.Height())
		}
	}
}

func TestPlayButtonStartsAndPauses(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	h.Find(gifttest.ByKey("play")).Tap()
	h.Settle()
	if p.TimeControlStatus() == video.Paused {
		t.Fatal("the play button did not start the player")
	}
	h.Find(gifttest.ByKey("play")).Tap()
	h.Settle()
	if p.TimeControlStatus() != video.Paused {
		t.Fatal("the button did not pause the player")
	}
}

func TestTrackMenuSelectsATrackAndCloses(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	h.AssertNone(gifttest.ByKey("track-menu"))
	h.Find(gifttest.ByKey("tracks")).Tap()
	h.Settle()
	h.AssertExists(gifttest.ByKey("track-menu"))
	h.AssertExists(gifttest.ByKey("track-eng-0"))
	h.Find(gifttest.ByKey("track-deu-1")).Tap()
	h.Settle()
	if got := p.AudioTrack(); got != 1 {
		t.Errorf("AudioTrack = %d after choosing the German track", got)
	}
	h.AssertNone(gifttest.ByKey("track-menu"))
}

func TestTapOnThePictureTogglesTheControls(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	h.AssertExists(gifttest.ByKey("play"))
	// Below the transport row and above the bottom bar: the picture.
	h.TapAt(geom.Pt(400, 360))
	h.Settle()
	h.AssertNone(gifttest.ByKey("play"))
	h.TapAt(geom.Pt(400, 360))
	h.Settle()
	h.AssertExists(gifttest.ByKey("play"))
}

func TestScrubbingSeeksWhereTheFingerLifts(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	bar := h.Find(gifttest.ByKey("scrubber")).LayoutBounds()
	y := (bar.Min.Y + bar.Max.Y) / 2
	h.TouchDownAt(geom.Pt(bar.Min.X+bar.Width()*0.2, y))
	h.Settle()
	h.TouchMoveTo(geom.Pt(bar.Min.X+bar.Width()*0.5, y))
	h.Settle()
	h.TouchUpAt(geom.Pt(bar.Min.X+bar.Width()*0.5, y))
	h.Settle()
	want := p.Duration() / 2
	if got := p.CurrentTime(); got < want-100*time.Millisecond || got > want+100*time.Millisecond {
		t.Errorf("CurrentTime = %v after lifting at the middle of %v", got, p.Duration())
	}
	if p.TimeControlStatus() != video.Paused {
		t.Error("scrubbing a paused video started it")
	}
}

func TestVolumeFollowsTheFingerAndMuteToggles(t *testing.T) {
	p := openClip(t)
	h := mount(t, New(p).AutoHide(0))
	vol := h.Find(gifttest.ByKey("volume")).LayoutBounds()
	y := (vol.Min.Y + vol.Max.Y) / 2
	h.TapAt(geom.Pt(vol.Min.X+volumeKnob/2, y))
	h.Settle()
	if got := p.Volume(); got > 0.05 {
		t.Errorf("Volume = %v after a tap at the left end", got)
	}
	h.Find(gifttest.ByKey("mute")).Tap()
	h.Settle()
	if !p.Muted() {
		t.Error("the mute button did not mute")
	}
}

func TestClockFormats(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"}, {999 * time.Millisecond, "0:00"}, {65 * time.Second, "1:05"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"}, {-time.Second, "0:00"},
	} {
		if got := clock(c.d); got != c.want {
			t.Errorf("clock(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if got := remaining(1500*time.Millisecond, 3*time.Second); got != "-0:02" {
		t.Errorf("remaining = %q, want -0:02 so that the two labels add up", got)
	}
}

func TestTrackNames(t *testing.T) {
	for _, c := range []struct {
		t    ffmpeg.AudioTrack
		want string
	}{
		{ffmpeg.AudioTrack{Language: "eng", Channels: 2}, "English · Stereo"},
		{ffmpeg.AudioTrack{Language: "deu", Channels: 6}, "Deutsch · 5.1"},
		{ffmpeg.AudioTrack{Language: "und", Channels: 1}, "Mono"},
		{ffmpeg.AudioTrack{Title: "Director's commentary", Language: "eng"}, "Director's commentary"},
		{ffmpeg.AudioTrack{}, "Track 3"},
	} {
		if got := trackName(c.t, 2); got != c.want {
			t.Errorf("trackName(%+v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestFitKeepsTheAspectAndCentres(t *testing.T) {
	r := fit(geom.Rc(0, 0, 800, 480), 16.0/9)
	if r.Width() != 800 || r.Min.Y <= 0 || r.Max.Y >= 480 {
		t.Errorf("fit 16:9 into 800x480 = %v, want full width with bars above and below", r)
	}
	r = fill(geom.Rc(0, 0, 800, 480), 16.0/9)
	if r.Height() != 480 || r.Min.X >= 0 || r.Max.X <= 800 {
		t.Errorf("fill 16:9 into 800x480 = %v, want full height, cut at the sides", r)
	}
}

func TestTheCloseButtonCallsOnClose(t *testing.T) {
	p := openClip(t)
	closed := 0
	h := mount(t, New(p).AutoHide(0).OnClose(func() { closed++ }))
	h.Find(gifttest.ByKey("close")).Tap()
	if closed != 1 {
		t.Fatalf("the close button called OnClose %d times", closed)
	}
	g := mount(t, New(p).AutoHide(0))
	g.AssertNone(gifttest.ByKey("close"))
}
