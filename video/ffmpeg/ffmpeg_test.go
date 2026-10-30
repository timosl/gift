package ffmpeg

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDecodeDir decodes every .mp4 in $GIFT_VIDEO_DIR as fast as it can and
// reports the speed. It is a measurement on the target, not a unit test, and
// skips everywhere else.
func TestDecodeDir(t *testing.T) {
	dir := os.Getenv("GIFT_VIDEO_DIR")
	if dir == "" {
		t.Skip("GIFT_VIDEO_DIR not set")
	}
	if !Available() {
		t.Skip(lib.err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.mp4"))
	hw := os.Getenv("GIFT_VIDEO_HW") != ""
	for _, path := range files {
		if filepath.Base(path) == "bbb_sunflower_1080p_60fps_normal.mp4" {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := Open(path, Options{HW: hw})
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			info := f.Info()
			var frame Frame
			var pcm []float32
			frames, last := 0, time.Duration(0)
			start := time.Now()
			drainV := func() {
				for {
					if err := f.Video().Receive(&frame); err != nil {
						if !errors.Is(err, ErrAgain) && !errors.Is(err, ErrEOF) {
							t.Fatal(err)
						}
						return
					}
					frames++
					last = frame.PTS
				}
			}
			drainA := func() {
				for {
					var err error
					if pcm, _, err = f.Audio().Receive(pcm[:0]); err != nil {
						if !errors.Is(err, ErrAgain) && !errors.Is(err, ErrEOF) {
							t.Fatal(err)
						}
						return
					}
				}
			}
			for {
				p, err := f.ReadPacket()
				if errors.Is(err, ErrEOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				switch p.Kind {
				case KindVideo:
					for f.Video().Send(p) == ErrAgain {
						drainV()
					}
					drainV()
				case KindAudio:
					for f.Audio().Send(p) == ErrAgain {
						drainA()
					}
					drainA()
				}
				p.Free()
			}
			f.Video().Send(nil)
			drainV()
			el := time.Since(start)
			t.Logf("%s %dx%d SAR %d:%d %.2f fps hw=%q layout=%d matrix=%d interlaced=%v | %d frames to %v in %v = %.1f fps (%.2fx realtime)",
				info.VideoCodec, info.Width, info.Height, info.SARNum, info.SARDen, info.FrameRate, f.Video().HW(),
				frame.Layout, frame.Matrix, frame.Interlaced, frames, last.Round(time.Millisecond), el.Round(time.Millisecond),
				float64(frames)/el.Seconds(), float64(frames)/info.FrameRate/el.Seconds())
		})
	}
}
