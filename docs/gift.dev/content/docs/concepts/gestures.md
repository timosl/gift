---
title: Gestures
weight: 9
---

Package [`ui/gesture`](https://pkg.go.dev/github.com/worldiety/gift/ui/gesture) recognizes taps, long presses,
drags, pinches, rotations, hovering and the wheel, and hands each recognized gesture to a function, in the manner of
SwiftUI's gestures. It is what the [image viewer](../../components/media/image-viewer/) is built from, and you can
attach the same recognizers to any view.

## Attaching gestures

`gesture.On(content, gestures...)` wraps a view. The wrapper takes the size of its content and draws nothing:

```go
gesture.On(card,
	gesture.Tap().Count(2).OnEnded(func(v gesture.TapValue) { like(id) }),
	gesture.Drag().Axis(gesture.Horizontal).
		OnChanged(func(v gesture.DragValue) { offset.Set(v.Translation.X) }).
		OnEnded(func(v gesture.DragValue) { settle(v.PredictedEndTranslation.X) }),
	gesture.Magnify().OnChanged(func(v gesture.MagnifyValue) { scale.Set(v.Magnification) }),
)
```

| Recognizer             | Value                                                                                     |
|------------------------|-------------------------------------------------------------------------------------------|
| `Tap().Count(n)`       | `Location`, `Count`                                                                        |
| `LongPress()`          | `Location`, after `MinimumDuration` (500 ms)                                               |
| `Drag()`               | `Location`, `StartLocation`, `Translation`, `Velocity`, `PredictedEndTranslation`, `Fingers` |
| `Magnify()`            | `Magnification`, `Velocity`, `StartAnchor`, `Location`                                     |
| `Rotate()`             | `Rotation` in radians, `Velocity`, `StartAnchor`, `Location`                               |
| `Hover()`              | `Location`, `Inside`, for a mouse that moves                                               |
| `Wheel()`              | `Location`, `Delta`; the function reports whether it used the event                        |

Every value also carries `Time`, `Bounds` (the recognizing view in window coordinates), `Device` and `Mods`, and
`Local` converts a position into the view's own coordinates. Continuous gestures report a `Phase`: `Began` on the
first change, `Changed` afterwards, and `OnEnded` and `OnCancelled` at the end.

## How recognizers get along

The recognizers of one view run side by side and see every event. Three rules keep them from contradicting each
other:

- A tap waits for a tap of a higher count to fail. A single tap on a view that also zooms on a double tap therefore
  fires after the double tap interval (300 ms), as with UIKit's require-to-fail.
- A long press that fires takes the press from every tap.
- A tap fails once its finger has moved `gesture.Slop` (10 points, the iOS value), which is exactly the distance at
  which a drag begins. A drag with an `Axis` decides at that distance and fails in the other direction, so a
  horizontal and a vertical drag on one view are exclusive without further arrangement.

`Drag().ShouldBegin(fn)` is UIKit's `gestureRecognizerShouldBegin`: a drag that declines fails, and the events go on
to the ancestors. A zoomable picture inside a scrolling page declines while it is not zoomed, and the page scrolls.
A drag follows the centroid of all fingers, so a second finger joins a drag rather than ending it, and the
translation does not jump when a finger arrives or lifts.

A gesture that begins takes the pointer from whatever captured it, with `EventContext.StealPointer`: a button under
the finger stops being pressed. Events no recognizer wants are reported as unused and bubble on to the ancestors.

## In a view of your own

A custom interactor uses the same engine with `gesture.Handle`. The recognition state lives in the node, so the
slice of gestures is built once per build and passed on every event:

```go
func (n *myNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	return gesture.Handle(ctx, e, n.gestures) || n.keys(ctx, e)
}
```

Set `gift.Element.MultiTouch` to `gesture.NeedsMultiTouch(n.gestures)`. A node keeps its own state across builds
with `ctx.Retained[T]()`, one value per type; the engine keeps its fingers there too.

The physics a gesture leads into — the rubber band past an edge, the deceleration of a flick, springs — are package
[`ui/motion`](https://pkg.go.dev/github.com/worldiety/gift/ui/motion): closed forms of time, evaluated on the clock
painters read, with UIKit's constants.

## Multitouch

gift delivers one finger at a time unless a view asks for more, as UIKit does with `isMultipleTouchEnabled`. A view
that sets `Element.MultiTouch` — every view with a `Magnify` or `Rotate` recognizer does — receives further fingers
that land on it or inside it, up to `gift.MaxTouches`, each with its own `Event.Pointer`. Any other further finger is
dropped and counted in `Diagnostics.DiscardedTouches`, which keeps a hand resting on a kiosk's glass from pressing a
second button.

## Touch screens on Linux

On Raspberry Pi OS, under X11 and XWayland, a touch screen reaches the window as an emulated mouse: one finger, no
time stamps, one position per tick. The Ebitengine backend therefore reads the panel directly from the kernel, as
libinput does for the compositor, without taking it from anybody:

- It finds the first direct multitouch device in `/dev/input`, which the default user may read as a member of the
  group `input`, and keeps looking every two seconds for one that is plugged in later.
  `backend.Config.Touchscreen` names a device or turns it `"off"`, and so does the environment variable
  `GIFT_TOUCHSCREEN`.
- Every finger arrives, so a pinch works, and the emulated mouse is ignored while a finger is down and until the
  cursor really moves.
- Every sample keeps the time the kernel stamped it with, on the monotonic clock. `Event.Time` is that time, and the
  velocity of a flick is computed from it rather than from the frame that happened to deliver the sample.
- Positions are **resampled to the frame**, the way Android's `InputConsumer` does: each frame shows the finger
  where it was a fixed latency before the frame, interpolated between the two samples around that moment or
  extrapolated a little from the last two. Without that, a panel sampling at 50 Hz read at 60 frames a second moves
  a steady finger by uneven steps, which is the judder of a dragged list on a cheap panel. Android's latency of 5 ms
  assumes a digitizer of 120 Hz or more; the drivers of the Raspberry Pi displays poll every 17 to 20 ms, so for
  such a panel the latency follows the measured sample period.
- A finger on an idle application raises the tick rate at once rather than at the next idle tick.

`cmd/gift-touchinject` creates a virtual multitouch screen through `uinput` and plays taps, swipes and pinches on
it, for testing all of this on a machine without a touch screen.

## API

- [`gesture.On`](https://pkg.go.dev/github.com/worldiety/gift/ui/gesture#On),
  [`gesture.Handle`](https://pkg.go.dev/github.com/worldiety/gift/ui/gesture#Handle)
- [`gift.Element.MultiTouch`](https://pkg.go.dev/github.com/worldiety/gift#Element),
  [`gift.EventContext.Retained`](https://pkg.go.dev/github.com/worldiety/gift#EventContext.Retained),
  [`gift.EventContext.After`](https://pkg.go.dev/github.com/worldiety/gift#EventContext.After),
  [`gift.App.PointerMoveAt`](https://pkg.go.dev/github.com/worldiety/gift#App.PointerMoveAt)
- [`motion`](https://pkg.go.dev/github.com/worldiety/gift/ui/motion)

## Related

- [Input and Focus](../input-and-focus/)
- [Kiosks](../kiosks/)
- [Image Viewer](../../components/media/image-viewer/)
