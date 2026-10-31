---
title: Pull to Refresh
---

`Refreshable` gives a vertical [Scroll View](../../layout/scroll-view/) the pull-to-refresh of iOS: content pulled
down past its top far enough starts a refresh, and a spinner turns in the room above the content until the refresh is
done.

{{< gallery-shot alt="A list of items pulled down below the top of the window, with a turning spinner of eight spokes above it" >}}

{{< gallery-code >}}

## Usage

```go
busy := ctx.State("busy", false)
ui.VScroll(ui.List(rows...)).Refreshable(ctx.Read(busy), func() {
	busy.Set(true)
	go reload(func() { app.Post(func() { busy.Set(false) }) })
})
```

`Refreshable(refreshing, onRefresh)` takes two things: whether a refresh is running, and the function that starts
one. `onRefresh` is called once per pull; it sets the state that says a refresh runs and starts the work, usually on
another goroutine. When the work is done, clear the state on the UI goroutine, with `App.Post` or a `gift.Token`
from `ctx.Request()` as the demo does.

## Interaction

- As the content is pulled past its top, the eight spokes of the spinner appear one by one.
- At 70 points the refresh starts, while the finger is still pulling, as on iOS.
- Let go, the content springs back to 56 points below its top and rests there while `refreshing` is true, with the
  spinner turning above it. A finger can take hold of it and scroll as usual.
- When `refreshing` turns false, the content springs back to its top.

A short pull only stretches the rubber band. Pull-to-refresh needs the rubber band, so it does nothing on a scroll
view with `Bounce(false)`, and nothing on a horizontal one.

## API

`Refreshable` is a modifier of `ui.ScrollView`:

{{< api "ScrollView" >}}

## Related

- [Scroll View](../../layout/scroll-view/), which it belongs to
- [Swipe Actions](../swipe-actions/), the other gesture of an iOS list
