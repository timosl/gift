package kiosk

import (
	"context"

	"github.com/worldiety/gift"
)

// loader holds what a page reads from the system: a value that a goroutine
// fetches, because every call of the system may block, and that arrives on
// the UI goroutine through the app. It lives in the state of the page, so
// that a rebuild does not fetch it again, and rebuilds the page when the
// value arrives.
type loader[T any] struct {
	k     *Kiosk
	fn    func(context.Context) (T, error)
	rev   *gift.State[int]
	gen   int
	alive bool

	val  T
	err  error
	done bool
	busy bool
}

// use returns the loader called key of the page being built, starting it
// the first time.
func use[T any](ctx *gift.Context, k *Kiosk, key string, fn func(context.Context) (T, error)) *loader[T] {
	st := ctx.State(key, (*loader[T])(nil))
	rev := ctx.State(key+"#rev", 0)
	ctx.Read(rev)
	l := st.Get()
	if l == nil {
		l = &loader[T]{k: k, rev: rev, alive: true}
		st.Set(l)
		ctx.OnUnmount(key, func() { l.alive = false })
		l.fn = fn
		l.reload()
	}
	l.fn = fn
	return l
}

// reload fetches the value again; the value of an earlier fetch that
// arrives later is dropped.
func (l *loader[T]) reload() {
	l.gen++
	gen, fn := l.gen, l.fn
	l.busy = true
	go func() {
		v, err := fn(context.Background())
		l.k.post(func() {
			if !l.alive || gen != l.gen {
				return
			}
			l.val, l.err, l.done, l.busy = v, err, true, false
			l.rev.Set(l.rev.Get() + 1)
		})
	}()
}

// set replaces the value, as a change made here already knows it.
func (l *loader[T]) set(v T) {
	l.gen++
	l.val, l.err, l.done, l.busy = v, nil, true, false
	l.rev.Set(l.rev.Get() + 1)
}
