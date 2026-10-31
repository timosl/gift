package gesture

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var viewType = gift.RegisterType("gesture.View")

// View is content with gestures attached; see [On].
type View struct {
	content     gift.View
	gs          []Gesture
	key, label  string
	onKey       func(gift.Event) bool
	keyFallback bool
	flex        float32
}

// On attaches gestures to content, the counterpart of SwiftUI's .gesture
// modifier:
//
//	gesture.On(ui.Image(src),
//		gesture.Magnify().OnChanged(func(v gesture.MagnifyValue) { ... }),
//		gesture.Tap().Count(2).OnEnded(func(gesture.TapValue) { ... }),
//	)
//
// The view takes the size of its content and draws nothing of its own. The
// gestures see every pointer event that reaches it, including the ones that
// bubble up from interactive content, and run side by side by the rules of
// [Handle]. A set with [Magnify] or [Rotate] accepts further fingers; see
// [gift.Element.MultiTouch].
func On(content gift.View, gs ...Gesture) View {
	return View{content: content, gs: gs}
}

// Key sets the reconciliation key.
func (v View) Key(s string) View { v.key = s; return v }

// Label sets the accessible name.
func (v View) Label(s string) View { v.label = s; return v }

// Flex sets the flex factor in a stack.
func (v View) Flex(f float32) View { v.flex = f; return v }

// OnKey makes the view focusable and hands it key and character events. The
// function reports whether it used the event; one it did not use bubbles on.
func (v View) OnKey(fn func(gift.Event) bool) View { v.onKey = fn; return v }

// KeyFallback makes the view receive keys while nothing holds the focus; see
// [gift.Element.KeyFallback]. A full screen view that is driven by the arrow
// keys on a device without a pointer wants it.
func (v View) KeyFallback(b bool) View { v.keyFallback = b; return v }

// ViewType implements gift.View.
func (View) ViewType() gift.TypeID { return viewType }

// Build implements gift.View.
func (v View) Build(*gift.BuildContext) gift.Element {
	n := &node{gs: v.gs, onKey: v.onKey}
	var kids []gift.View
	if v.content != nil {
		kids = []gift.View{v.content}
	}
	return gift.Element{
		Key:         v.key,
		Label:       v.label,
		Layouter:    n,
		Interactor:  n,
		Children:    kids,
		Flex:        v.flex,
		Focusable:   v.onKey != nil,
		KeyFallback: v.keyFallback,
		MultiTouch:  NeedsMultiTouch(v.gs),
	}
}

type node struct {
	gs    []Gesture
	onKey func(gift.Event) bool
}

// Layout gives the content the constraints and takes its size.
func (n *node) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if ctx.ChildCount() == 0 {
		return c.Constrain(geom.Size{})
	}
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

func (n *node) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventKeyDown, gift.EventKeyUp, gift.EventRune:
		return n.onKey != nil && n.onKey(e)
	case gift.EventPointerDown:
		if n.onKey != nil {
			ctx.RequestFocus()
		}
	}
	return Handle(ctx, e, n.gs)
}
