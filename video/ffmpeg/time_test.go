package ffmpeg

import (
	"testing"
	"time"
)

func TestToDuration(t *testing.T) {
	for _, c := range []struct {
		pts  int64
		tb   [2]int64
		want time.Duration
	}{
		{90000, [2]int64{1, 90000}, time.Second},
		{1, [2]int64{1, 25}, 40 * time.Millisecond},
		{1001, [2]int64{1, 30000}, 33366666 * time.Nanosecond},
		// Ten hours at 90 kHz does not overflow.
		{10 * 3600 * 90000, [2]int64{1, 90000}, 10 * time.Hour},
		{5, [2]int64{0, 0}, 0},
	} {
		if got := toDuration(c.pts, c.tb); got != c.want {
			t.Errorf("toDuration(%d, %v) = %v, want %v", c.pts, c.tb, got, c.want)
		}
	}
}

func TestDisplayAspect(t *testing.T) {
	for _, c := range []struct {
		info Info
		want float64
	}{
		{Info{Width: 720, Height: 576, SARNum: 16, SARDen: 15}, 4.0 / 3},
		{Info{Width: 720, Height: 576, SARNum: 64, SARDen: 45}, 16.0 / 9},
		{Info{Width: 1920, Height: 1080}, 16.0 / 9},
	} {
		if got := c.info.DisplayAspect(); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%+v: %v, want %v", c.info, got, c.want)
		}
	}
}
