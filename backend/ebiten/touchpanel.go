package ebiten

import (
	"log/slog"
	"os"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

// This file is the platform neutral half of reading a touch panel past the
// display server: the multitouch protocol of the Linux kernel, decoded into
// contacts, and the resampling that turns the panel's samples into one
// position per frame. The device half is evdev_linux.go.
//
// # Why past the display server
//
// On Raspberry Pi OS a touch reaches an X11 client — Ebitengine is one, under
// XWayland — as an emulated mouse: one finger, polled once per tick, with no
// time stamp and no samples in between. Two fingers are impossible, and so is
// an honest velocity. The kernel has all of it: every finger, every sample,
// each stamped by the driver. The panel is a character device the default user
// may read (group input), so gift reads it, as libinput does for the
// compositor, without taking it away from anybody.
//
// # Why resample
//
// A panel samples at its own rate — 60, 90, 100, 120 Hz — and the display
// refreshes at another. Shown as it arrives, the latest sample is between zero
// and one panel period old when a frame is drawn, a different age every frame,
// and a finger moving at a steady speed is drawn moving in uneven steps: the
// judder of a scrolling list on a cheap panel. Android removes it by drawing
// every frame at the position the finger had a fixed few milliseconds before
// the frame — interpolated between the two samples around that moment, or
// extrapolated a little from the last two — and gift does the same with
// Android's constants. Velocities are computed from the samples' own times;
// see [gift.App.PointerMoveAt].

// panelKind is what happened to a contact.
type panelKind uint8

const (
	panelDown panelKind = iota + 1
	panelMove
	panelUp
)

// panelEvent is one change of one contact, in panel coordinates normalised to
// [0, 1] and on the clock of the input bridge.
type panelEvent struct {
	kind panelKind
	id   int32
	x, y float64
	t    time.Duration
}

// panel is a touch panel read past the display server.
type panel interface {
	// drain appends the events since the last call and returns dst.
	drain(dst []panelEvent) []panelEvent
	// recent reports whether a finger is down or was lifted within the last
	// d, at now: the time in which the display server's emulated mouse is
	// the same finger again and must be ignored.
	recent(now, d time.Duration) bool
	close()
}

// mtDecoder is the state machine of the kernel's multitouch protocol B: slots,
// tracking identifiers and frames ended by SYN_REPORT.
type mtDecoder struct {
	minX, maxX, minY, maxY int32
	slot                   int
	slots                  [mtSlots]mtSlot
	dropping               bool
}

// mtSlots is how many slots are followed. Panels report ten at most.
const mtSlots = 16

type mtSlot struct {
	id                  int32
	x, y                int32
	live                bool
	began, moved, ended bool
}

// The kernel's event types and codes the decoder reads; see
// include/uapi/linux/input-event-codes.h.
const (
	evSyn = 0x00
	evAbs = 0x03

	synReport  = 0
	synDropped = 3

	absMTSlot       = 0x2f
	absMTPositionX  = 0x35
	absMTPositionY  = 0x36
	absMTTrackingID = 0x39
)

// feed takes one kernel event and appends what a finished frame changed.
func (d *mtDecoder) feed(typ, code uint16, value int32, t time.Duration, out []panelEvent) []panelEvent {
	switch typ {
	case evAbs:
		if d.dropping {
			return out
		}
		switch code {
		case absMTSlot:
			d.slot = int(value)
		case absMTTrackingID:
			if d.slot < 0 || d.slot >= mtSlots {
				return out
			}
			s := &d.slots[d.slot]
			if value < 0 {
				if s.live {
					s.ended = true
				}
				return out
			}
			*s = mtSlot{id: value, x: s.x, y: s.y, live: true, began: true}
		case absMTPositionX, absMTPositionY:
			if d.slot < 0 || d.slot >= mtSlots {
				return out
			}
			s := &d.slots[d.slot]
			if code == absMTPositionX {
				s.x = value
			} else {
				s.y = value
			}
			s.moved = true
		}
	case evSyn:
		switch code {
		case synDropped:
			// Events were lost. Every contact is lifted where it was and
			// the next frame starts afresh, which is the documented
			// recovery short of reading the slots back.
			d.dropping = true
		case synReport:
			if d.dropping {
				d.dropping = false
				for i := range d.slots {
					if s := &d.slots[i]; s.live {
						out = append(out, d.event(panelUp, s, t))
						*s = mtSlot{}
					}
				}
				return out
			}
			for i := range d.slots {
				s := &d.slots[i]
				switch {
				case s.began:
					out = append(out, d.event(panelDown, s, t))
				case s.ended:
					out = append(out, d.event(panelUp, s, t))
					*s = mtSlot{}
					continue
				case s.moved && s.live:
					out = append(out, d.event(panelMove, s, t))
				}
				if s.ended {
					// Down and up in one frame: a touch shorter than a
					// sample.
					out = append(out, d.event(panelUp, s, t))
					*s = mtSlot{}
					continue
				}
				s.began, s.moved = false, false
			}
		}
	}
	return out
}

func (d *mtDecoder) event(k panelKind, s *mtSlot, t time.Duration) panelEvent {
	return panelEvent{kind: k, id: s.id, x: norm(s.x, d.minX, d.maxX), y: norm(s.y, d.minY, d.maxY), t: t}
}

func norm(v, lo, hi int32) float64 {
	if hi <= lo {
		return 0
	}
	return float64(v-lo) / float64(hi-lo)
}

// The resampling constants of Android's InputConsumer.
const (
	// resampleLatency is how far behind the frame the finger is drawn.
	resampleLatency = 5 * time.Millisecond
	// resampleMinDelta and resampleMaxDelta bound the distance of two
	// samples that are used to extrapolate: closer is noise, further is a
	// finger that stopped.
	resampleMinDelta = 2 * time.Millisecond
	resampleMaxDelta = 20 * time.Millisecond
	// resampleMaxPrediction caps an extrapolation, which is further capped
	// at half the distance of the two samples.
	resampleMaxPrediction = 8 * time.Millisecond
	// panelLinger is how long after a finger lifted the display server's
	// emulated mouse is still the same finger.
	panelLinger = 500 * time.Millisecond
	// slowPanel is the sample period above which the latency follows the
	// panel, and maxLatency caps it; see [inputBridge.latency].
	slowPanel  = 12 * time.Millisecond
	maxLatency = 30 * time.Millisecond
)

type sample struct {
	t    time.Duration
	x, y float64
}

// contact is one finger on the panel, as the bridge resamples it.
type contact struct {
	id   int32
	live bool
	hist [8]sample
	n    int
	// sent is the time of the sample last handed to gift.
	sent time.Duration
	// up is set when the finger lifted this tick, at the last sample.
	up bool
}

func (c *contact) add(s sample) {
	if c.n == len(c.hist) {
		copy(c.hist[:], c.hist[1:])
		c.n--
	}
	c.hist[c.n] = s
	c.n++
}

func (c *contact) last() sample { return c.hist[c.n-1] }

// at is the resampled position of the contact for a frame at t: between the
// two samples around t, or extrapolated from the last two by at most
// [resampleMaxPrediction] and half their distance. It reports false when the
// samples do not reach t yet.
func (c *contact) at(t time.Duration) (sample, bool) {
	if c.n == 0 {
		return sample{}, false
	}
	l := c.last()
	if t >= l.t {
		if c.n < 2 {
			return l, true
		}
		p := c.hist[c.n-2]
		dt := l.t - p.t
		if dt < resampleMinDelta || dt > resampleMaxDelta {
			return l, true
		}
		pred := min(t-l.t, resampleMaxPrediction, dt/2)
		f := float64(pred) / float64(dt)
		return sample{t: l.t + pred, x: l.x + (l.x-p.x)*f, y: l.y + (l.y-p.y)*f}, true
	}
	for i := c.n - 2; i >= 0; i-- {
		a, b := c.hist[i], c.hist[i+1]
		if a.t > t {
			continue
		}
		if b.t-a.t < resampleMinDelta {
			return b, true
		}
		f := float64(t-a.t) / float64(b.t-a.t)
		return sample{t: t, x: a.x + (b.x-a.x)*f, y: a.y + (b.y-a.y)*f}, true
	}
	return sample{}, false
}

// panelID is the pointer identifier of a panel contact: far from anything
// Ebitengine hands out.
func panelID(id int32) gift.PointerID { return gift.PointerID(1<<40 | int64(uint32(id))) }

// panelMap converts normalised panel coordinates into the logical pixels of
// the window. It is computed once per tick.
type panelMap struct {
	w, h, ox, oy, scale float64
}

func (m panelMap) pos(b *inputBridge, x, y float64) geom.Point {
	return b.logical((x*m.w-m.ox)*m.scale, (y*m.h-m.oy)*m.scale)
}

// pollPanel hands the panel's contacts to gift: presses and releases with
// their own times, moves resampled to the frame. now is the tick time.
func (b *inputBridge) pollPanel(now time.Duration, m panelMap) {
	b.panelEvs = b.panel.drain(b.panelEvs[:0])
	for _, e := range b.panelEvs {
		s := sample{t: e.t, x: e.x, y: e.y}
		switch e.kind {
		case panelDown:
			c := b.contact(e.id, true)
			if c == nil {
				continue
			}
			*c = contact{id: e.id, live: true}
			c.add(s)
			c.sent = e.t
			b.app.PointerDownAt(panelID(e.id), gift.PointerTouch, m.pos(b, s.x, s.y), e.t)
		case panelMove:
			if c := b.contact(e.id, false); c != nil {
				b.notePeriod(e.t - c.last().t)
				c.add(s)
			}
		case panelUp:
			if c := b.contact(e.id, false); c != nil {
				c.add(s)
				c.up = true
			}
		}
	}
	frame := now - b.latency()
	for i := range b.contacts {
		c := &b.contacts[i]
		if !c.live {
			continue
		}
		if c.up {
			// The release goes where and when the finger lifted, after a
			// move to the same place, so that the last samples reach the
			// velocity of whatever was being flung.
			l := c.last()
			p := m.pos(b, l.x, l.y)
			b.app.PointerMoveAt(panelID(c.id), gift.PointerTouch, p, l.t)
			b.app.PointerUpAt(panelID(c.id), gift.PointerTouch, p, l.t)
			*c = contact{}
			continue
		}
		s, ok := c.at(frame)
		if !ok || s.t <= c.sent {
			continue
		}
		c.sent = s.t
		b.app.PointerMoveAt(panelID(c.id), gift.PointerTouch, m.pos(b, s.x, s.y), s.t)
	}
}

// contact finds the contact with the kernel identifier id, or a free one for
// a press.
func (b *inputBridge) contact(id int32, press bool) *contact {
	for i := range b.contacts {
		if c := &b.contacts[i]; c.live && c.id == id {
			return c
		}
	}
	if !press {
		return nil
	}
	for i := range b.contacts {
		if c := &b.contacts[i]; !c.live {
			return c
		}
	}
	return nil
}

// attachPanel starts reading the touch panel cfg names; see
// [Config.Touchscreen].
func (b *inputBridge) attachPanel(cfg string, logger *slog.Logger, wake func()) {
	if v, ok := os.LookupEnv("GIFT_TOUCHSCREEN"); ok {
		cfg = v
	}
	if cfg == "off" {
		return
	}
	b.panel = openPanel(cfg, b.start, logger, wake)
	b.screen = ebitenScreen
}

func (b *inputBridge) detachPanel() {
	if b.panel != nil {
		b.panel.close()
	}
}

// ebitenScreen maps the panel onto the monitor the window is on: a touch
// screen covers its display, and the window sits somewhere on it unless it is
// full screen.
func ebitenScreen() panelMap {
	m := eb.Monitor()
	if m == nil {
		return panelMap{}
	}
	w, h := m.Size()
	var ox, oy int
	if !eb.IsFullscreen() {
		ox, oy = eb.WindowPosition()
	}
	return panelMap{w: float64(w), h: float64(h), ox: float64(ox), oy: float64(oy), scale: m.DeviceScaleFactor()}
}

// notePeriod folds the distance of two samples of one finger into the
// estimate of the panel's sample period: an exponential average over about
// sixteen samples of the distances a panel can have, so that a pause of the
// finger is not taken for a slow panel.
func (b *inputBridge) notePeriod(d time.Duration) {
	if d < 3*time.Millisecond || d > 60*time.Millisecond {
		return
	}
	if b.panelPeriod == 0 {
		b.panelPeriod = d
		return
	}
	b.panelPeriod += (d - b.panelPeriod) / 16
}

// latency is how far behind the frame a finger is drawn.
//
// Android's 5 ms assume a digitiser of 120 Hz or more, which leaves a sample
// after the moment that is drawn in most frames, so that position is
// interpolated. The panels of a Raspberry Pi are polled by their drivers every
// 17 ms, which a kernel ticking at 250 Hz makes 20: about 50 samples a second,
// with a drifting phase, against 60 frames. At 5 ms behind the frame most
// frames would have to extrapolate, from two samples whose times jitter by a
// tick, and the judder resampling exists to remove would come back. So a
// panel slower than [slowPanel] is drawn one sample period and a little more
// behind the frame — always between two samples — at the price of that much
// latency, capped at [maxLatency]. A fast panel keeps Android's 5 ms.
func (b *inputBridge) latency() time.Duration {
	if b.panelPeriod <= slowPanel {
		return resampleLatency
	}
	return min(b.panelPeriod+3*time.Millisecond, maxLatency)
}
