package ui

import (
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var pageControlType = gift.RegisterType("ui.PageControl")

// Metrics of a page control: the ones of UIPageControl.
const (
	pageDot     = float32(7)
	pagePitch   = float32(16)
	pageHeight  = float32(26)
	pagePadding = float32(10)
)

// PageControlView shows which of a row of pages is shown. It is created by
// [PageControl].
type PageControlView struct {
	base
	count, current int
	onChange       func(int)
	tint           Color
	hasTint        bool
	capsule        bool
	name           string
}

// PageControl returns the dots of UIPageControl for count pages with current
// shown, the counterpart of the index of SwiftUI's paged TabView.
//
//	page := ctx.State("page", 0)
//	ui.PageControl(5, ctx.Read(page), page.Set)
//
// A tap on either side of the current dot goes one page that way, and a
// finger that slides along the dots goes to the page under it, as on iOS
// since 14; the arrow keys page when it has the focus. A control with one
// page or none draws nothing, as UIPageControl does with hidesForSinglePage.
func PageControl(count, current int, onChange func(int)) PageControlView {
	count = max(count, 0)
	return PageControlView{count: count, current: max(0, min(current, count-1)), onChange: onChange}
}

// Tint sets the colour of the current dot; the others are a third of it. The
// default is [ColorLabel].
func (v PageControlView) Tint(c Color) PageControlView { v.tint, v.hasTint = c, true; return v }

// Capsule puts the dots on a dark translucent capsule, for a page control
// over a picture: UIPageControl's prominent background style. The dots are
// then white unless [PageControlView.Tint] says otherwise.
func (v PageControlView) Capsule(b bool) PageControlView { v.capsule = b; return v }

// Label sets the accessible name.
func (v PageControlView) Label(s string) PageControlView { v.name = s; return v }

// Key sets the reconciliation key.
func (v PageControlView) Key(s string) PageControlView { v.setKey(s); return v }

// Flex sets the flex factor in a stack.
func (v PageControlView) Flex(f float32) PageControlView { v.setFlex(f); return v }

// ViewType implements gift.View.
func (v PageControlView) ViewType() gift.TypeID { return pageControlType }

// Build implements gift.View.
func (v PageControlView) Build(*gift.BuildContext) gift.Element {
	tint := Color(ColorLabel)
	if v.capsule {
		tint = RGB(255, 255, 255)
	}
	if v.hasTint {
		tint = v.tint
	}
	n := &pageControlNode{v: v, on: ResolveColor(tint), off: ResolveColor(Fade(tint, 0.3)),
		capsule: RGBA(0, 0, 0, 64), focusRing: resolveBorder(defaultFieldFocusRing)}
	if v.capsule {
		n.off = ResolveColor(Fade(tint, 0.45))
	}
	return gift.Element{
		Key: v.key, Flex: v.flex, Label: v.name,
		Layouter: n, Painter: n, Interactor: n,
		Focusable: v.count > 1,
	}
}

type pageControlNode struct {
	v                PageControlView
	on, off, capsule Color
	focusRing        Border
}

// pageControlState is the gesture under way; it survives the build every
// page change causes.
type pageControlState struct {
	from      geom.Point
	scrubbing bool
}

func (n *pageControlNode) width() float32 {
	return float32(n.v.count)*pagePitch + 2*pagePadding - (pagePitch - pageDot)
}

func (n *pageControlNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	if n.v.count <= 1 {
		return c.Constrain(geom.Size{})
	}
	return c.Constrain(geom.Sz(n.width(), pageHeight))
}

// dotsLeft is the x of the left edge of the first dot within bounds b: the
// dots are centred in whatever width the control was given.
func (n *pageControlNode) dotsLeft(b geom.Rect) float32 {
	return b.Min.X + (b.Width()-(float32(n.v.count-1)*pagePitch+pageDot))/2
}

func (n *pageControlNode) Paint(ctx *gift.PaintContext) {
	if n.v.count <= 1 {
		return
	}
	b := ctx.Bounds()
	if n.v.capsule {
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: n.capsule, CornerRadius: b.Height() / 2})
	}
	x := n.dotsLeft(b)
	cy := (b.Min.Y + b.Max.Y) / 2
	for i := range n.v.count {
		c := n.off
		if i == n.v.current {
			c = n.on
		}
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x, cy-pageDot/2, x+pageDot, cy+pageDot/2), Color: c, CornerRadius: pageDot / 2})
		x += pagePitch
	}
	if ia := ctx.Interaction(); ia.FocusVisible {
		paintBorder(ctx, styleSpec{border: n.focusRing, radius: b.Height() / 2}, b)
	}
}

// AccessibleValue implements [gift.ValueReporter]: "page 2 of 5".
func (n *pageControlNode) AccessibleValue(gift.ControlState) string {
	return "page " + strconv.Itoa(n.v.current+1) + " of " + strconv.Itoa(n.v.count)
}

// turnTo reports page i, kept inside the pages, and takes it as current for
// the rest of this event.
func (n *pageControlNode) turnTo(i int) {
	i = max(0, min(i, n.v.count-1))
	if i != n.v.current && n.v.onChange != nil {
		n.v.onChange(i)
	}
	n.v.current = i
}

// dotAt is the page whose dot is nearest to the device x.
func (n *pageControlNode) dotAt(ctx *gift.EventContext, x float32) int {
	b := ctx.DeviceBounds()
	left := n.dotsLeft(b) + pageDot/2
	return int((x-left)/pagePitch + 0.5)
}

func (n *pageControlNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.v.count <= 1 {
		return false
	}
	st := ctx.Retained[pageControlState]()
	switch e.Kind {
	case gift.EventPointerDown:
		st.from, st.scrubbing = e.Pos, false
		return true
	case gift.EventPointerMove:
		if !e.Dragged {
			return false
		}
		if !st.scrubbing {
			// Sideways along the dots it scrubs; a stroke more up or down
			// belongs to the container, a list the control sits in.
			if absf(e.Pos.Y-st.from.Y) > absf(e.Pos.X-st.from.X) {
				return false
			}
			st.scrubbing = true
			ctx.StealPointer()
		}
		n.turnTo(n.dotAt(ctx, e.Pos.X))
		ctx.Repaint()
		return true
	case gift.EventPointerUp:
		if st.scrubbing {
			st.scrubbing = false
			return true
		}
		if e.Inside && !e.Dragged {
			b := ctx.DeviceBounds()
			cur := n.dotsLeft(b) + float32(n.v.current)*pagePitch + pageDot/2
			if e.Pos.X < cur {
				n.turnTo(n.v.current - 1)
			} else {
				n.turnTo(n.v.current + 1)
			}
			ctx.Repaint()
		}
		return true
	case gift.EventPointerCancel:
		st.scrubbing = false
		return true
	case gift.EventKeyDown:
		switch e.Key {
		case gift.KeyLeft:
			n.turnTo(n.v.current - 1)
		case gift.KeyRight:
			n.turnTo(n.v.current + 1)
		case gift.KeyHome:
			n.turnTo(0)
		case gift.KeyEnd:
			n.turnTo(n.v.count - 1)
		default:
			return false
		}
		ctx.Repaint()
		return true
	}
	return false
}
