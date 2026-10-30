package video

import "time"

// clock is the media clock: a wall clock that runs while the player plays and
// is pulled towards the audio position when there is audio.
//
// The audio position alone is a poor clock for pictures. It advances in steps
// of whatever the audio device pulls at once, ten milliseconds or more, and a
// picture chosen by a clock that jumps by most of a frame period at 60 Hz is
// shown twice, then skipped. The wall clock is smooth, the audio position is
// right, and slaving one to the other with a small gain keeps both: small
// errors are absorbed over a few hundred milliseconds, large ones — a seek, an
// underrun, a suspended device — are taken over at once.
type clock struct {
	base    time.Duration // media time at start
	start   time.Time
	running bool
	rate    float64
}

const (
	clockSnap = 60 * time.Millisecond
	clockGain = 16 // a sixteenth of the error per correction
)

func (c *clock) now(t time.Time) time.Duration {
	if !c.running {
		return c.base
	}
	return c.base + time.Duration(float64(t.Sub(c.start))*c.rate)
}

func (c *clock) set(m time.Duration, t time.Time) { c.base, c.start = m, t }

func (c *clock) run(t time.Time) {
	if !c.running {
		c.start, c.running = t, true
	}
}

func (c *clock) stop(t time.Time) {
	if c.running {
		c.base, c.running = c.now(t), false
	}
}

// follow pulls the clock towards ref, a reading of the audio position taken
// at t, and returns the error before the correction.
func (c *clock) follow(ref time.Duration, t time.Time) time.Duration {
	d := ref - c.now(t)
	if d > clockSnap || d < -clockSnap {
		c.set(ref, t)
		return d
	}
	c.base += d / clockGain
	return d
}
