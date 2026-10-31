package gift

// Retained returns the value of type T the receiving node keeps across builds,
// creating a zero T on first use.
//
// # What it is for
//
// [ControlState] is the retained state of a control, and it is deliberately a
// small fixed struct: one grab, one animation. A recogniser that follows two
// fingers, keeps the last hundred milliseconds of samples for a velocity and
// runs a spring after the release does not fit into it, and widening it for
// one caller is what its own documentation argues against. This is the open
// ended sibling: one value of any type per node, owned by the node and not by
// the view, so it survives every rebuild for the reason ControlState does — a
// rebuild replaces the view and the node object, not the node.
//
// It is the identity rule of SwiftUI's state, applied to a node: the value
// lives as long as the node at this position with this type and key, and a
// node that is unmounted takes it along. A fresh node in a recycled slot
// starts from a zero T; see [nodeData.release].
//
// # Rules
//
// One value per type. A node may keep several, one for each type it asks for,
// so that a recogniser that keeps its fingers here and the view that uses it,
// which keeps its zoom here, do not have to know of each other. A type that is
// unexported in its package cannot collide with anybody else's.
//
// Writing the value never builds, lays out or repaints anything by itself.
// A handler that changed what is drawn says so with [EventContext.Repaint] or
// [EventContext.Animate], exactly as with its own fields.
//
// The value is a pointer into the node's payload, valid while the node is
// mounted. Keeping it beyond that is the same mistake as keeping a
// [PaintContext]: it then belongs to nobody.
func (c *EventContext) Retained[T any]() *T { return retainedOf[T](c.nd) }

// Retained is [EventContext.Retained] for the node being laid out.
func (l *LayoutContext) Retained[T any]() *T { return retainedOf[T](l.nd) }

// Retained is [EventContext.Retained] for the node being painted.
//
// A painter may create the value — a zero T is what it would have read anyway
// — but must not change it: painting is free of side effects, and a painter
// that wrote here would make the picture depend on the order nodes are
// visited in.
func (p *PaintContext) Retained[T any]() *T { return retainedOf[T](p.nd) }

func retainedOf[T any](nd *nodeData) *T {
	for _, v := range nd.retained {
		if t, ok := v.(*T); ok {
			return t
		}
	}
	v := new(T)
	nd.retained = append(nd.retained, v)
	return v
}
