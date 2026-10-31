package gesture

import (
	"time"

	"github.com/worldiety/gift/geom"
)

// Velocity estimation constants. They are Flutter's and Android's, which
// agree: fit over the last 100 ms, and call a pointer that has not moved for
// 40 ms before it lifted a pointer that stopped.
const (
	velocityHorizon = 100 * time.Millisecond
	velocityStopped = 40 * time.Millisecond
	velocitySamples = 20
)

// tracker estimates the velocity of a position from its recent samples, by a
// least squares line through those within [velocityHorizon]. A fit rather than
// the last two samples, because touch panels report at their own rate and in
// bursts, and the difference of two samples one millisecond apart is noise.
//
// It is a fixed ring and allocates nothing.
type tracker struct {
	t    [velocitySamples]time.Duration
	p    [velocitySamples]geom.Point
	n, i int
}

func (k *tracker) reset() { k.n, k.i = 0, 0 }

// add records a sample. A sample that follows the previous one by more than
// [velocityStopped] starts the history afresh, as Android's VelocityTracker
// does: the finger stopped in between, and what it did before says nothing
// about how fast it moves now.
func (k *tracker) add(t time.Duration, p geom.Point) {
	if k.n > 0 && t-k.t[(k.i-1+velocitySamples)%velocitySamples] > velocityStopped {
		k.reset()
	}
	k.t[k.i], k.p[k.i] = t, p
	k.i = (k.i + 1) % velocitySamples
	if k.n < velocitySamples {
		k.n++
	}
}

// velocity is the estimate at now, in units per second.
func (k *tracker) velocity(now time.Duration) geom.Point {
	if k.n < 2 {
		return geom.Point{}
	}
	last := (k.i - 1 + velocitySamples) % velocitySamples
	if now-k.t[last] > velocityStopped {
		return geom.Point{}
	}
	var st, sx, sy float64
	m := 0
	for j := range k.n {
		idx := (last - j + velocitySamples) % velocitySamples
		if k.t[last]-k.t[idx] > velocityHorizon {
			break
		}
		st += k.t[idx].Seconds()
		sx += float64(k.p[idx].X)
		sy += float64(k.p[idx].Y)
		m++
	}
	if m < 2 {
		return geom.Point{}
	}
	mt, mx, my := st/float64(m), sx/float64(m), sy/float64(m)
	var stt, stx, sty float64
	for j := range m {
		idx := (last - j + velocitySamples) % velocitySamples
		dt := k.t[idx].Seconds() - mt
		stt += dt * dt
		stx += dt * (float64(k.p[idx].X) - mx)
		sty += dt * (float64(k.p[idx].Y) - my)
	}
	if stt < 1e-12 {
		return geom.Point{}
	}
	return geom.Pt(float32(stx/stt), float32(sty/stt))
}
