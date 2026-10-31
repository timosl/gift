---
title: Wheel Picker
---

A wheel picker chooses one of many values with a turning wheel, as iOS's `UIPickerView` does: the rows sit on a
cylinder, shrink and fade towards the top and the bottom, and the chosen one rests in a band across the middle.
Several wheels stand side by side, one per part of the choice. It is the natural picker for a finger: a wheel is
turned, flicked and stopped where the finger lifts, and nothing has to be typed.

{{< gallery-shot alt="Two wheels side by side, sizes XS to XXL and colours, with M and White in a grey band across the middle" >}}

{{< gallery-code >}}

## Usage

`ui.WheelPicker(wheels...)` stands wheels side by side. `ui.Wheel(count, title)` is one wheel of `count` rows, row
`i` reading `title(i)`:

```go
size := ctx.State("size", 2)
ui.WheelPicker(
	ui.Wheel(len(sizes), func(i int) string { return sizes[i] }).Bind(size.Binding()),
)
```

`Bind` reads and writes the chosen row; `Selected` and `OnChange` do the same in two halves. `Cyclic(true)` turns a
wheel round, the last row followed by the first, as the hours and minutes of iOS do. `Width` fixes a wheel's width;
wheels without one share what is left. `Align` lines the rows up, centred by default.

The picker is `VisibleRows` rows tall (five) of `RowHeight` (36). The rows fade into `Background`, which is
`ColorSurface` by default: set it to whatever the picker sits on, or the fading shows as a band.

## Turning

- **Finger**: the wheel follows; a flick runs on and decelerates as a `UIScrollView` does, onto the row Apple's
  projection of the flick lands on, and stops there without overshooting. Past the first and the last row the wheel
  rubber bands. A finger on a turning wheel stops it.
- **Tap**: a tap on a row above or below the band turns the wheel to it.
- **Mouse**: a wheel notch steps one row; a trackpad's fractions of a notch add up.
- **Keys**: a focused wheel steps with the up and down arrows, pages with page up and down, and goes to the ends with
  home and end.

`OnChange` is called when the finger lifts, with the row the wheel will come to rest on, so the application knows the
choice while the wheel is still turning. A value the application sets turns the wheel to it, the short way round.

## API

- [`ui.WheelPicker`](https://pkg.go.dev/github.com/worldiety/gift/ui#WheelPicker)
- [`ui.Wheel`](https://pkg.go.dev/github.com/worldiety/gift/ui#Wheel)
