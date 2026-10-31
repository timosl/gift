package imageviewer_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/imageviewer"
)

// TestAZoomableInAScrollingPageLeavesTheScrollingToThePage: not zoomed, a
// finger drag scrolls the page; pinched, the picture zooms and then pans.
func TestAZoomableInAScrollingPageLeavesTheScrollingToThePage(t *testing.T) {
	items, srcs := photos(t, 1)
	ch := make(chan func(), 64)
	pipe := asset.NewPipeline(asset.Config{Deliver: func(fn func()) { ch <- fn }, Sizes: []int{256, 512, 1024}, Workers: 1})
	t.Cleanup(func() { pipe.Close() })
	ui.SetImagePipeline(pipe)
	t.Cleanup(func() { ui.SetImagePipeline(nil) })
	src := srcs[items[0].ID]
	h := gifttest.New(t, gifttest.Options{
		View: ui.VScroll(
			imageviewer.Zoomable(src).Frame(600, 400).Key("photo"),
			ui.Box().Frame(600, 900),
		).Key("page"),
		Size: geom.Sz(600, 480),
	})
	h.App().SetImages(&images{})
	h.Settle()
	scrolled := func() float64 {
		info, _ := h.App().ScrollInfo(h.Find(gifttest.ByKey("page")).Ref())
		return info.Offset
	}
	h.Fling(geom.Pt(300, 300), geom.Pt(300, 150), 300*time.Millisecond)
	h.AdvanceTicks(60, gifttest.FrameInterval)
	if scrolled() < 100 {
		t.Fatalf("a drag over an unzoomed picture scrolled the page by %v, want it to scroll", scrolled())
	}
	// Back to the top, where the picture is, for the pinch.
	h.App().ScrollTo(h.Find(gifttest.ByKey("page")).Ref(), 0)
	h.Settle()
	before := scrolled()
	h.Pinch(geom.Pt(300, 200), 80, 240, 250*time.Millisecond)
	h.AdvanceTicks(90, gifttest.FrameInterval)
	if d := scrolled() - before; d < -1 || d > 1 {
		t.Errorf("a pinch scrolled the page by %v", d)
	}
	// The picture arrives and is drawn zoomed.
	deadline := time.Now().Add(10 * time.Second)
	widest := float32(0)
	for widest == 0 && time.Now().Before(deadline) {
		select {
		case fn := <-ch:
			fn()
		default:
			time.Sleep(time.Millisecond)
		}
		h.Settle()
		for _, op := range h.Ops() {
			if op.Kind == render.OpImage {
				widest = max(widest, op.Bounds.Width())
			}
		}
	}
	if widest < 600*2.5 {
		t.Errorf("after a pinch of three the picture is drawn %v wide, want about three times 600", widest)
	}
}
