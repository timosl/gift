---
title: Circular Slider
---

`ui.CircularSlider` picks a number from a range on an open ring whose knob a finger drags round it: the circular form
of the [Slider](../slider/), for a thermostat, a volume dial or a timer.

{{< gallery-shot alt="An open ring with an orange part from half past seven to just before twelve, a white knob there, and 21.5 degrees Living room in the middle" >}}

{{< gallery-code >}}

## Usage

```go
t := ctx.State("t", 21.0)
ui.ZStack(
	ui.CircularSlider(t.Get(), t.Set).Range(16, 28).Step(0.5),
	ui.Text(fmt.Sprintf("%.1f°", ctx.Read(t))),
).Align(geom.Center)
```

Like the slider it is controlled: it shows the value you give it and reports a new one to `onChange`. `Range` sets
the smallest and the largest value, 0 and 1 by default, and `Step` the grid the value keeps to; zero is continuous.
The slider shows no number; put one in its middle with a `ZStack`, as above.

## Interaction

- A press on the ring, or close to it, takes the knob there, and a drag moves it round.
- A drag into the gap at the bottom holds the knob at the nearer end; it never jumps across the gap from one end to
  the other.
- A press in the middle is left to whatever stands there.
- Focused, the arrow keys step, page up and down take ten steps, and home and end go to the ends. Without a `Step`, a
  step is a hundredth of the range. The wheel steps too.

## Look

The ring runs from half past seven to half past four, 140 points across and 10 wide by default; `Size` and
`LineWidth` change that. The part up to the value is `ui.ColorAccent` unless `Tint` says otherwise, the rest the
track of the slider, and the knob is the slider's knob.

## API

{{< api "CircularSliderView" >}}

## Related

- [Slider](../slider/), the straight form
- [Gauge](../gauge/), the same ring for a value nobody sets
- [Stepper](../stepper/), for a number a step at a time
