package gift_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var alarmType = gift.RegisterType("test.Alarm")

// alarm arms a timer on every press and counts what fires.
type alarm struct {
	delay time.Duration
	fired *[]time.Duration
}

func (alarm) ViewType() gift.TypeID { return alarmType }

func (v alarm) Build(*gift.BuildContext) gift.Element {
	n := &alarmNode{v: v}
	return gift.Element{Key: "alarm", Layouter: n, Interactor: n}
}

type alarmNode struct{ v alarm }

func (n *alarmNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size { return geom.Sz(50, 50) }

func (n *alarmNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.After(n.v.delay)
		return true
	case gift.EventTimer:
		*n.v.fired = append(*n.v.fired, e.Time)
		return true
	}
	return false
}

func TestAfterDeliversOneTimerAndReArmingReplacesIt(t *testing.T) {
	var fired []time.Duration
	root := func(*gift.Context) gift.View {
		return frameView{w: 100, h: 100, offsets: []geom.Point{at(0, 0)},
			children: []gift.View{alarm{delay: 300 * time.Millisecond, fired: &fired}}}
	}
	a := newInputApp(t, root)
	press := func(now time.Duration) {
		a.BeginInput(now)
		a.PointerDown(gift.MousePointer, gift.PointerMouse, at(10, 10))
		a.PointerUp(gift.MousePointer, gift.PointerMouse, at(10, 10))
		mustUpdate(t, a)
	}
	press(0)
	// A second press before the first timer fired replaces it.
	press(200 * time.Millisecond)
	for now := 216 * time.Millisecond; now < time.Second; now += 16 * time.Millisecond {
		a.BeginInput(now)
		if len(fired) == 0 && !a.NeedsPaint() {
			t.Fatalf("at %v a timer is pending and nothing keeps the backend ticking", now)
		}
		mustUpdate(t, a)
	}
	if len(fired) != 1 || fired[0] < 500*time.Millisecond || fired[0] > 520*time.Millisecond {
		t.Fatalf("fired at %v, want once, 300 ms after the second press", fired)
	}
}
