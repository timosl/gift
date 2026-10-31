---
title: Date and Time Wheels
---

`ui.DateWheel` chooses a date on a day, a month and a year wheel, and `ui.TimeWheel` a time of day on an hour and a
minute wheel: the wheel style of SwiftUI's `DatePicker`. They are built from the [Wheel Picker](../wheel-picker/) and
turn the same way. For typing a date or a time with a keyboard, use [Text Field](../text-field/)'s siblings
`ui.DateField` and `ui.TimeField` instead.

{{< gallery-shot alt="A date wheel showing 31 October 2026 and a time wheel showing 14:30, each in a band across the middle" >}}

{{< gallery-code >}}

## Date

```go
day := ctx.State("day", ui.DateOf(2026, time.October, 31))
ui.DateWheel().
	Bind(day.Binding()).
	Range(ui.DateOf(2020, time.January, 1), ui.DateOf(2030, time.December, 31)).
	Order(ui.DayMonthYear).
	MonthNames(ui.GermanMonths)
```

The day and the month wheels turn round; the year wheel ends at the `Range`, which is 1900 to 2100 by default. A day
that does not exist in the month chosen — the 31st of April — becomes the last day of that month, and a date outside
the range becomes its nearest end, to which the wheels then turn. `Order` puts the wheels in the order of a locale:
`DayMonthYear` (the default), `MonthDayYear` or `YearMonthDay`. `MonthNames` replaces the English names.

## Time

```go
at := ctx.State("at", ui.TimeOf(14, 30))
ui.TimeWheel().Bind(at.Binding()).MinuteInterval(5)
```

Both wheels turn round, on the 24 hour clock. `MinuteInterval` makes the minute wheel step by 5, 10, 15 or any other
divisor of 60, as `UIDatePicker` does.

An empty value shows today or the current time without choosing it, so a form can tell "not chosen yet" from a choice.

## API

- [`ui.DateWheel`](https://pkg.go.dev/github.com/worldiety/gift/ui#DateWheel)
- [`ui.TimeWheel`](https://pkg.go.dev/github.com/worldiety/gift/ui#TimeWheel)
