package video

import (
	"testing"
	"time"
)

func TestClockFollowsSmallErrorsSlowly(t *testing.T) {
	var c clock
	c.rate = 1
	t0 := time.Unix(0, 0)
	c.set(0, t0)
	c.run(t0)

	// The audio position says 8 ms ahead: a sixteenth is taken over.
	at := t0.Add(100 * time.Millisecond)
	if d := c.follow(108*time.Millisecond, at); d != 8*time.Millisecond {
		t.Fatalf("error %v, want 8ms", d)
	}
	if got := c.now(at); got != 100*time.Millisecond+time.Millisecond/2 {
		t.Fatalf("now %v after one correction", got)
	}
}

func TestClockSnapsToLargeErrors(t *testing.T) {
	var c clock
	c.rate = 1
	t0 := time.Unix(0, 0)
	c.set(0, t0)
	c.run(t0)
	at := t0.Add(time.Second)
	c.follow(3*time.Second, at)
	if got := c.now(at); got != 3*time.Second {
		t.Fatalf("now %v, want the reference", got)
	}
}

func TestClockStopsAndResumes(t *testing.T) {
	var c clock
	c.rate = 1
	t0 := time.Unix(0, 0)
	c.set(time.Second, t0)
	c.run(t0)
	c.stop(t0.Add(500 * time.Millisecond))
	if got := c.now(t0.Add(time.Hour)); got != 1500*time.Millisecond {
		t.Fatalf("a stopped clock moved: %v", got)
	}
	c.run(t0.Add(time.Hour))
	if got := c.now(t0.Add(time.Hour + time.Second)); got != 2500*time.Millisecond {
		t.Fatalf("resumed at %v", got)
	}
}

func TestPlayedHoldsThroughSilence(t *testing.T) {
	s := &AudioStream{rate: 1000, segs: []seg{{0, 10 * time.Second}, {500, 10*time.Second + 300*time.Millisecond}}}
	// 300 ms of sound, then 200 ms of silence filled in for an underrun:
	// during the silence the clock stays at the end of the sound.
	if got := s.played(200 * time.Millisecond); got != 10*time.Second+200*time.Millisecond {
		t.Fatalf("in the sound: %v", got)
	}
	if got := s.played(400 * time.Millisecond); got != 10*time.Second+300*time.Millisecond {
		t.Fatalf("in the silence: %v", got)
	}
	if got := s.played(600 * time.Millisecond); got != 10*time.Second+400*time.Millisecond {
		t.Fatalf("after the silence: %v", got)
	}
}
