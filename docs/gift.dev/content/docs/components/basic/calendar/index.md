---
title: Calendar
---

`ui.Calendar` chooses a date on a month grid: the graphical style of SwiftUI's `DatePicker` and `UICalendarView`. For
a date on wheels use the [Date and Time Wheels](../date-wheel/), for typing one `ui.DateField`.

{{< gallery-shot alt="A calendar on a white card showing October 2026, the 31st in a filled blue circle and a dot under every Saturday" >}}

{{< gallery-code >}}

## Choosing a date

```go
day := ctx.State("day", ui.DateOf(2026, time.October, 31))
ui.Calendar().
	Bind(day.Binding()).
	Range(ui.DateOf(2026, time.January, 1), ui.DateOf(2027, time.December, 31)).
	FirstWeekday(time.Monday).
	MonthNames(ui.GermanMonths).
	WeekdayNames(ui.GermanWeekdays)
```

A tap on a day chooses it. The chosen day sits in a filled circle of the accent colour, today is written in it. Days
outside the `Range` (1900 to 2100 by default) are grey and refuse the tap. `FirstWeekday` sets the day the weeks begin
with, Monday by default; `MonthNames` and `WeekdayNames` replace the English names.

The grid always has six rows, so the calendar keeps its height from month to month.

## Changing the month

The month shown is the calendar's own state and changing it chooses nothing:

- **Swipe** sideways. The months follow the finger; a flick or a drag past half way turns to the next or the previous
  one, a shorter drag springs back. At the ends of the range the months give way like a rubber band.
- **Arrows** beside the title, the **wheel**, and **page up** and **page down** slide the next month in from its side.
- A **tap on the title** turns the grid into a month and a year wheel, and a second one turns it back.

A focused calendar moves the chosen day with the **arrow keys**, by a day sideways and by a week up and down, and the
month shown follows it.

## Marking days

```go
ui.Calendar().Bind(day.Binding()).Marked(func(d ui.Date) bool { return appointments[d] > 0 })
```

`Marked` puts a dot under every day it reports true for: the days with appointments, the days a shop is open. It is
asked for the days shown, so keep it cheap.

## Appearance

`Background` puts the calendar on a rounded card, which the month and year wheels fade into as well; without it the
calendar draws no background and the wheels fade into the window's colour. `Width` fixes the width; without it the
calendar fills what it is given up to 480 points.

## API

- [`ui.Calendar`](https://pkg.go.dev/github.com/worldiety/gift/ui#Calendar)
- [`ui.CalendarView`](https://pkg.go.dev/github.com/worldiety/gift/ui#CalendarView)
