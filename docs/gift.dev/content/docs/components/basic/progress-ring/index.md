---
title: Progress Ring
---

`ui.ProgressRing` shows how far a task has come as a ring that fills clockwise from twelve o'clock, the circular
counterpart of the [Progress Bar](../progress-bar/). `Indeterminate` turns it into the spinner of iOS for a task whose
end nobody knows.

{{< gallery-shot alt="Three rings a quarter, three fifths and nine tenths full, and two spinners of eight spokes" >}}

{{< gallery-code >}}

## Usage

```go
ui.ProgressRing(done / total)
ui.ProgressRing(0).Indeterminate()
```

The fraction works as for the progress bar: above 1 is full, a negative one panics. A new fraction is not jumped to:
the ring eases there in about a third of a second, so a value that arrives in steps still moves smoothly.

## Look

The ring is 28 points across with a line a seventh of that wide, in `ui.ColorAccent` over a track of a fifth of it.
`Size` and `LineWidth` change the two, `Tint` the colour. The spinner has the eight spokes of
`UIActivityIndicatorView` in `ui.ColorSecondaryLabel` and turns as long as it is on screen; `Size` scales it.

Rings and arcs are drawn by the shape shader from a distance field, like every rounded rectangle, so they cost one
quad each in the same draw call as everything else and are sharp at every size and density.

## API

{{< api "ProgressRingView" >}}

## Related

- [Progress Bar](../progress-bar/), the straight form
- [Gauge](../gauge/), a value within a range on a ring
- [Pull to Refresh](../../lists/pull-to-refresh/), which shows the same spinner
