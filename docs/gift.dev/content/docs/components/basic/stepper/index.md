---
title: Stepper
---

`ui.Stepper` changes a number by a step at a time with a minus and a plus button side by side: the counterpart of
`UIStepper` and of the control of SwiftUI's `Stepper`. It shows no number itself; put the value next to it.

{{< gallery-shot alt="Three rows of a label and a stepper; the last stepper is at its maximum and its plus half is grey" >}}

{{< gallery-code >}}

## Usage

```go
tickets := ctx.State("tickets", 1.0)
ui.HStack(
	ui.Text(fmt.Sprintf("Tickets: %v", ctx.Read(tickets))).Flex(1),
	ui.Stepper(tickets.Get(), tickets.Set).Range(1, 10),
)
```

The stepper is controlled, like the [Slider](../slider/): it shows the value you give it and reports the next one to
`onChange`. `Range` sets the smallest and the largest value, 0 and 100 by default, and `Step` the size of a step, 1
by default. Steps land on the grid of the step from the minimum, so ten steps of 0.1 from zero reach exactly 1.

At either end of the range that half of the stepper is grey and does nothing. `Wraps(true)` makes a step past one
end go to the other instead, as `UIStepper.wraps` does.

## Interaction

A tap steps once, as soon as the finger is down. Held, a half steps again after half a second, ten times a second,
and twenty times a second after two seconds. Sliding off the stepper pauses the repeat, and sliding back resumes it.

Focused, left and down step down, right and up step up, and home and end go to the ends of the range.

The stepper is 94 by 32 points.

## API

{{< api "StepperView" >}}

## Related

- [Slider](../slider/), for a number from a range in one gesture
- [Wheel Picker](../wheel-picker/), for one value out of a list
