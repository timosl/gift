package video

import (
	"testing"
	"time"

	"github.com/worldiety/gift/video/ffmpeg"
)

// testClip is three seconds of Big Buck Bunny at 640x360 and 25 fps, with an
// English stereo and a German 5.1 track; see testdata/README.md.
const testClip = "testdata/bbb.mp4"

func openTestClip(t *testing.T) *Player {
	t.Helper()
	if !ffmpeg.Available() {
		t.Skip("FFmpeg is not installed")
	}
	p, err := Open(testClip, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

// run calls Frame like a backend at 60 Hz for d of real time and returns the
// number of new pictures.
func run(p *Player, d time.Duration) int {
	n := 0
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(time.Second / 60) {
		if _, changed := p.Frame(time.Now()); changed {
			n++
		}
	}
	return n
}

func TestPlayerListsTheAudioTracks(t *testing.T) {
	p := openTestClip(t)
	tr := p.AudioTracks()
	if len(tr) != 2 {
		t.Fatalf("%d audio tracks, want 2: %+v", len(tr), tr)
	}
	if tr[0].Language != "eng" || tr[0].Channels != 2 || !tr[0].Default {
		t.Errorf("track 0 = %+v, want the default English stereo track", tr[0])
	}
	if tr[1].Language != "deu" || tr[1].Channels != 6 {
		t.Errorf("track 1 = %+v, want the German 5.1 track", tr[1])
	}
	if got := p.AudioTrack(); got != 0 {
		t.Errorf("AudioTrack = %d, want the default track 0", got)
	}
}

func TestPlayerPlaysWithoutAnAudioDevice(t *testing.T) {
	p := openTestClip(t)
	if f, _ := p.Frame(time.Now()); f != nil {
		// Nothing decoded yet is fine; a picture is the poster.
		_ = f
	}
	p.Play()
	n := run(p, 600*time.Millisecond)
	// 25 fps for 0.6 s, less the start: well above a handful, and the
	// clock must have moved although no device reads the sound.
	if n < 8 {
		t.Errorf("%d pictures in 0.6 s at 25 fps", n)
	}
	if ct := p.CurrentTime(); ct < 300*time.Millisecond {
		t.Errorf("CurrentTime = %v after 0.6 s of playing", ct)
	}
	if p.TimeControlStatus() != Playing {
		t.Errorf("TimeControlStatus = %v", p.TimeControlStatus())
	}
}

func TestPlayerSwitchesTheAudioTrackWhilePlaying(t *testing.T) {
	p := openTestClip(t)
	p.Play()
	run(p, 400*time.Millisecond)
	before := p.CurrentTime()
	p.SelectAudioTrack(1)
	if got := p.AudioTrack(); got != 1 {
		t.Fatalf("AudioTrack = %d after selecting 1", got)
	}
	n := run(p, 600*time.Millisecond)
	if n < 5 {
		t.Errorf("%d pictures after the switch", n)
	}
	// The switch is a seek to the current position, not to the start.
	if ct := p.CurrentTime(); ct < before {
		t.Errorf("CurrentTime went back from %v to %v", before, ct)
	}
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerSeeksWhilePausedAndShowsThePicture(t *testing.T) {
	p := openTestClip(t)
	p.Seek(2 * time.Second)
	var f *ffmpeg.Frame
	for end := time.Now().Add(2 * time.Second); f == nil && time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		f, _ = p.Frame(time.Now())
	}
	if f == nil {
		t.Fatal("no picture after a paused seek")
	}
	if f.PTS < 2*time.Second-40*time.Millisecond || f.PTS > 2*time.Second+40*time.Millisecond {
		t.Errorf("the picture after seeking to 2s is at %v", f.PTS)
	}
	if p.TimeControlStatus() != Paused {
		t.Errorf("a seek started playback: %v", p.TimeControlStatus())
	}
}

func TestPlayerPausesAtTheEnd(t *testing.T) {
	p := openTestClip(t)
	ended := make(chan struct{}, 1)
	p.OnEnd(func() { ended <- struct{}{} })
	p.Seek(2700 * time.Millisecond)
	p.Play()
	run(p, 800*time.Millisecond)
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("OnEnd was not called")
	}
	if p.TimeControlStatus() != Paused {
		t.Errorf("TimeControlStatus = %v at the end", p.TimeControlStatus())
	}
}
