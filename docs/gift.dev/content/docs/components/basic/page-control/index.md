---
title: Page Control
---

`ui.PageControl` shows a row of dots, one per page, with the current one solid: the counterpart of `UIPageControl`
and of the index of SwiftUI's paged `TabView`.

{{< gallery-shot alt="Five grey dots with the second one black, and below them a picture with four white dots on a dark capsule" >}}

{{< gallery-code >}}

## Usage

```go
page := ctx.State("page", 0)
ui.PageControl(5, ctx.Read(page), page.Set)
```

Put it under the pages it counts, for example an [Image Viewer](../../media/image-viewer/) or a pager, and set the
same state from both.

## Interaction

A tap left of the current dot goes one page back, a tap right of it one page on. A finger that slides along the
dots goes to the page under it, as on iOS since version 14; a stroke that goes more up or down than sideways is left
to the scroll view around the control. Focused, the arrow keys turn the pages and home and end go to the first and
the last.

## Look

The dots are 7 points across on a pitch of 16, in `ui.ColorLabel` for the current page and a third of it for the
others. `Tint` sets another colour. `Capsule(true)` puts the dots on a dark translucent capsule and makes them white,
for a page control over a picture, which is `UIPageControl`'s prominent background style.

A control with one page or none draws nothing and takes no room.

## API

{{< api "PageControlView" >}}
