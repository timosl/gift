---
title: Toast
---

A toast shows a short message above everything for a moment: a capsule with an optional symbol, a title and an
optional subtitle that slides in at the top of the window, stays for two seconds and goes again, as iOS shows
"Copied" or a connected headset.

{{< gallery-shot alt="A white capsule at the top of the window with a link symbol, the title Link copied and the subtitle gift.dev/docs, above a Copy link button" >}}

{{< gallery-code >}}

## Usage

```go
copied := ctx.State("copied", false)
return ui.Toast(screen, "Link copied").
	Icon(outline.Link).
	Presented(ctx.Read(copied)).
	OnDismiss(func() { copied.Set(false) })
```

`ui.Toast` wraps a view, usually the screen, and presents the message while `Presented` is true. When the toast's
time is up it calls `OnDismiss`, whose job is to stop presenting it. `Duration` sets the time, two seconds by
default; zero keeps the toast until it is swiped away or no longer presented. A new title is a new toast, which
slides in afresh.

The toast lives in the top layer above the whole window, like a [Popover](../popover/), but it takes no input outside
its capsule: a tap beside it reaches whatever is underneath.

## Interaction

- A **swipe** towards the edge dismisses the toast early. A pull the other way gives like a rubber band and springs
  back.
- A finger that **holds** the toast keeps it; its time starts again when the finger lifts.
- A **tap** calls `OnTap` and dismisses the toast.

## Options

| Modifier            | Effect                                                   |
|---------------------|----------------------------------------------------------|
| `Subtitle(s)`       | a second, smaller line under the title                   |
| `Icon(symbol)`      | a symbol in front of the text                            |
| `Duration(d)`       | how long the toast stays; zero until dismissed           |
| `Edge(ui.ToastBottom)` | at the bottom of the window instead of the top        |
| `OnTap(fn)`         | called when the toast is tapped, before it is dismissed  |

## API

{{< api "ToastView" >}}

## Related

- [Popover](../popover/), for a message that belongs to one view
- [Alert](../alert/), for a message that needs an answer
