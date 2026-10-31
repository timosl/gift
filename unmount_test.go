package gift_test

import (
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
)

// TestOnUnmountRunsOnceWhenTheComponentGoes pins Context.OnUnmount: replaced
// by name on every build, run once when the instance leaves the tree, with
// its state still readable, and not at all while it stays.
func TestOnUnmountRunsOnceWhenTheComponentGoes(t *testing.T) {
	var closed []int
	var show *gift.State[bool]
	var bump func()
	child := func(ctx *gift.Context) gift.View {
		n := ctx.State("n", 0)
		bump = func() { n.Set(n.Get() + 1) }
		v := ctx.Read(n)
		ctx.OnUnmount("player", func() { closed = append(closed, v*10+n.Get()) })
		return newLeaf("x", 10, 10)
	}
	root := func(ctx *gift.Context) gift.View {
		show = ctx.State("show", true)
		if ctx.Read(show) {
			return gift.Component("child", child)
		}
		return newLeaf("x", 10, 10)
	}
	h := gifttest.New(t, gifttest.Options{Root: root, Size: geom.Sz(100, 100)})
	h.Settle()
	h.App().BeginInput(h.Now())
	bump()
	h.Settle()
	h.App().BeginInput(h.Now())
	bump()
	h.Settle()
	if len(closed) != 0 {
		t.Fatalf("rebuilds ran the unmount function: %v", closed)
	}
	h.App().BeginInput(h.Now())
	show.Set(false)
	h.Settle()
	if len(closed) != 1 || closed[0] != 22 {
		t.Fatalf("unmounting ran %v, want once, the function of the last build reading its state, 22", closed)
	}
}
