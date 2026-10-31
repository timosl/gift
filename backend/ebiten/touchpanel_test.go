package ebiten

import (
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// fakePanel is a panel whose events the test queues.
type fakePanel struct {
	q    []panelEvent
	down int
	last time.Duration
}

func (p *fakePanel) drain(dst []panelEvent) []panelEvent {
	dst = append(dst, p.q...)
	p.q = p.q[:0]
	return dst
}
func (p *fakePanel) recent(now, d time.Duration) bool { return p.down > 0 || now-p.last < d }
func (p *fakePanel) close()                           {}

func ms(f float64) time.Duration { return time.Duration(f * float64(time.Millisecond)) }

func TestTheDecoderFollowsProtocolB(t *testing.T) {
	d := mtDecoder{maxX: 1000, maxY: 1000}
	var out []panelEvent
	abs := func(code uint16, v int32) { out = d.feed(evAbs, code, v, 0, out) }
	syn := func(at time.Duration) { out = d.feed(evSyn, synReport, 0, at, out) }
	// A finger in slot 0, then a second one in slot 1.
	abs(absMTSlot, 0)
	abs(absMTTrackingID, 7)
	abs(absMTPositionX, 100)
	abs(absMTPositionY, 200)
	syn(ms(10))
	abs(absMTPositionX, 150)
	abs(absMTSlot, 1)
	abs(absMTTrackingID, 8)
	abs(absMTPositionX, 900)
	abs(absMTPositionY, 900)
	syn(ms(20))
	// The first lifts.
	abs(absMTSlot, 0)
	abs(absMTTrackingID, -1)
	syn(ms(30))
	// Events lost: what is down is lifted.
	out = d.feed(evSyn, synDropped, 0, ms(35), out)
	syn(ms(40))
	type ev struct {
		k    panelKind
		id   int32
		x, y float64
		t    time.Duration
	}
	var got []ev
	for _, e := range out {
		got = append(got, ev{e.kind, e.id, e.x, e.y, e.t})
	}
	want := []ev{
		{panelDown, 7, 0.1, 0.2, ms(10)},
		{panelMove, 7, 0.15, 0.2, ms(20)},
		{panelDown, 8, 0.9, 0.9, ms(20)},
		{panelUp, 7, 0.15, 0.2, ms(30)},
		{panelUp, 8, 0.9, 0.9, ms(40)},
	}
	if len(got) != len(want) {
		t.Fatalf("events %+v, want %+v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.k != w.k || g.id != w.id || math.Abs(g.x-w.x) > 1e-9 || math.Abs(g.y-w.y) > 1e-9 || g.t != w.t {
			t.Errorf("event %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestResamplingInterpolatesAndExtrapolatesLikeAndroid(t *testing.T) {
	var c contact
	c.add(sample{t: ms(0), x: 0})
	c.add(sample{t: ms(10), x: 10})
	c.add(sample{t: ms(20), x: 20})
	if s, _ := c.at(ms(14)); s.x != 14 || s.t != ms(14) {
		t.Errorf("between two samples: %+v, want x 14 at 14 ms", s)
	}
	// 3 ms past the last sample: predicted.
	if s, _ := c.at(ms(23)); math.Abs(s.x-23) > 1e-9 {
		t.Errorf("3 ms ahead: %+v, want x 23", s)
	}
	// Far ahead: capped at half the sample distance, 5 ms.
	if s, _ := c.at(ms(40)); math.Abs(s.x-25) > 1e-9 || s.t != ms(25) {
		t.Errorf("20 ms ahead: %+v, want the prediction capped at x 25", s)
	}
	// A finger that stopped is not extrapolated.
	c.add(sample{t: ms(60), x: 20})
	if s, _ := c.at(ms(70)); s.x != 20 {
		t.Errorf("after a pause: %+v, want the last sample", s)
	}
}

// TestAPanelFasterThanTheFrameRateIsDrawnWithoutJudder is the reason for the
// resampling: a 100 Hz panel read by 60 Hz ticks delivers one sample in some
// ticks and two in others, and a finger moving at a steady speed would be
// drawn moving 10 or 20 pixels per frame. Resampled, every frame moves the
// same distance.
func TestAPanelFasterThanTheFrameRateIsDrawnWithoutJudder(t *testing.T) {
	ka := newKeyboardApp(t)
	b := newInputBridge(ka.app)
	fp := &fakePanel{}
	b.panel = fp
	m := panelMap{w: 200, h: 200, scale: 1}
	const speed = 1000.0 // px/s, so 10 px per 10 ms sample
	sampleAt := func(t time.Duration) float64 { return 10 + speed*t.Seconds() }

	next := time.Duration(0)
	fp.q = append(fp.q, panelEvent{kind: panelDown, id: 1, x: sampleAt(0) / 200, y: 0.1, t: 0})
	fp.down = 1
	next += 10 * time.Millisecond
	var xs []float32
	var times []time.Duration
	frame := time.Second / 60
	for tick := 1; tick <= 9; tick++ {
		now := time.Duration(tick) * frame
		for next <= now {
			fp.q = append(fp.q, panelEvent{kind: panelMove, id: 1, x: sampleAt(next) / 200, y: 0.1, t: next})
			next += 10 * time.Millisecond
		}
		ka.events = ka.events[:0]
		ka.app.BeginInput(now)
		b.pollPanel(now, m)
		for _, e := range ka.events {
			if e.Kind == gift.EventPointerMove {
				xs = append(xs, e.Pos.X)
				times = append(times, e.Time)
			}
		}
	}
	if len(xs) < 7 {
		t.Fatalf("%d moves in 9 frames", len(xs))
	}
	for i := 1; i < len(xs); i++ {
		step := xs[i] - xs[i-1]
		if math.Abs(float64(step)-speed/60) > 0.5 {
			t.Errorf("frame %d moved %v px, want a steady %.1f; positions %v", i, step, speed/60, xs)
			break
		}
	}
	// The events carry the time the finger was there, 5 ms behind the frame.
	if d := time.Duration(9)*frame - times[len(times)-1]; d != resampleLatency {
		t.Errorf("the last move is stamped %v behind its frame, want %v", d, resampleLatency)
	}
	// And the position is where the finger was at that time.
	if want := sampleAt(times[len(times)-1]); math.Abs(float64(xs[len(xs)-1])-want) > 0.01 {
		t.Errorf("the last move is at %v, want %v", xs[len(xs)-1], want)
	}
	_ = geom.Point{}
}

func TestTheEmulatedMouseOfATouchIsIgnored(t *testing.T) {
	ka := newKeyboardApp(t)
	b := newInputBridge(ka.app)
	fp := &fakePanel{down: 1}
	b.panel = fp
	b.cursor = func() (float64, float64) { return 50, 20 }
	ka.events = ka.events[:0]
	ka.app.BeginInput(0)
	b.pollMouse(0)
	fp.down, fp.last = 0, 0
	// After the linger, a cursor that has not moved is still not a mouse.
	ka.app.BeginInput(time.Second)
	b.pollMouse(time.Second)
	for _, e := range ka.events {
		if e.Device == gift.PointerMouse && e.IsPointer() {
			t.Fatalf("the emulated mouse reached the view: %+v", e)
		}
	}
	// A mouse that moves is a mouse.
	b.cursor = func() (float64, float64) { return 60, 20 }
	ka.app.BeginInput(time.Second + 16*time.Millisecond)
	b.pollMouse(time.Second + 16*time.Millisecond)
	moved := false
	for _, e := range ka.events {
		moved = moved || e.Kind == gift.EventPointerMove && e.Device == gift.PointerMouse
	}
	if !moved {
		t.Error("a real mouse movement after a touch was ignored")
	}
}

// TestASlowJitteryPanelIsDrawnWithoutJudder is the Raspberry Pi case: a panel
// polled every 17 ms that the kernel's 250 Hz tick turns into 18 to 22 ms,
// read at 60 frames a second. Drawn at Android's 5 ms behind the frame most
// frames would extrapolate from jittering samples; the latency follows the
// panel instead, and every frame is interpolated.
func TestASlowJitteryPanelIsDrawnWithoutJudder(t *testing.T) {
	ka := newKeyboardApp(t)
	b := newInputBridge(ka.app)
	fp := &fakePanel{down: 1}
	b.panel = fp
	m := panelMap{w: 400, h: 200, scale: 1}
	const speed = 600.0
	pos := func(t time.Duration) float64 { return 10 + speed*t.Seconds() }
	jitter := []time.Duration{18, 22, 20, 19, 21, 22, 18, 20}
	fp.q = append(fp.q, panelEvent{kind: panelDown, id: 1, x: pos(0) / 400, y: 0.1})
	next, j := time.Duration(0), 0
	var xs []float32
	frame := time.Second / 60
	for tick := 1; tick <= 40; tick++ {
		now := time.Duration(tick) * frame
		for next+jitter[j%len(jitter)]*time.Millisecond <= now {
			next += jitter[j%len(jitter)] * time.Millisecond
			j++
			fp.q = append(fp.q, panelEvent{kind: panelMove, id: 1, x: pos(next) / 400, y: 0.1, t: next})
		}
		ka.events = ka.events[:0]
		ka.app.BeginInput(now)
		b.pollPanel(now, m)
		for _, e := range ka.events {
			if e.Kind == gift.EventPointerMove {
				xs = append(xs, e.Pos.X)
			}
		}
	}
	if b.latency() < 20*time.Millisecond {
		t.Errorf("the latency is %v for a panel of about 20 ms, want it to follow the panel", b.latency())
	}
	// Once the period is known, every frame moves the same distance.
	for i := len(xs) - 15; i < len(xs); i++ {
		if step := xs[i] - xs[i-1]; math.Abs(float64(step)-speed/60) > 0.5 {
			t.Fatalf("frame %d moved %v px, want a steady %.1f; positions %v", i, step, speed/60, xs[len(xs)-16:])
		}
	}
}
