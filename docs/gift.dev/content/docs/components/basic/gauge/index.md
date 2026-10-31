---
title: Gauge
---

`ui.Gauge` shows a value within a range on a ring, the counterpart of SwiftUI's `Gauge` in its circular styles: an
open ring like a speedometer, or a closed one for how full something is.

{{< gallery-shot alt="Three gauges: an open blue ring at 21.5 degrees between 10 and 30, an orange ring 62 percent full, and an open red ring at 84 of 120" >}}

{{< gallery-code >}}

## Usage

```go
ui.Gauge(21.5, 10, 30).CurrentValueLabel("21.5°").MinMaxLabels("10", "30")
ui.Gauge(0.62, 0, 1).Capacity(true).CurrentValueLabel("62 %")
```

`Gauge(value, min, max)` places the value in the range; a value outside it shows at the nearer end. By default the
ring is open at the bottom, from half past seven to half past four, filled from its left end up to the value, with the
labels of `MinMaxLabels` under its two ends. `Capacity(true)` closes the ring and fills it clockwise from twelve
o'clock, as SwiftUI's `accessoryCircularCapacity` style does. `CurrentValueLabel` stands in the middle.

A new value is eased to, like the [Progress Ring](../progress-ring/)'s.

## Look

The gauge is 72 points across with a line a tenth of that wide; `Size` and `LineWidth` change them. The filled part
is `ui.ColorAccent` unless `Tint` says otherwise, and the track a quarter of it. The labels scale with the size.

## API

{{< api "GaugeView" >}}

## Related

- [Progress Ring](../progress-ring/), for how far a task has come
- [Circular Slider](../circular-slider/), the same ring for a value the user sets
