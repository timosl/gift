package ui

import (
	"strconv"
	"time"

	"github.com/worldiety/gift"
)

var (
	dateWheelType = gift.RegisterType("ui.DateWheel")
	timeWheelType = gift.RegisterType("ui.TimeWheel")
)

// DateOrder is the order of the wheels of a [DateWheel].
type DateOrder uint8

const (
	// DayMonthYear is the order of most of Europe.
	DayMonthYear DateOrder = iota
	// MonthDayYear is the order of the United States.
	MonthDayYear
	// YearMonthDay is the order of ISO 8601 and of East Asia.
	YearMonthDay
)

// EnglishMonths are the default names of the month wheel.
var EnglishMonths = [12]string{
	"January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

// GermanMonths are the German names, for [DateWheelView.MonthNames].
var GermanMonths = [12]string{
	"Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember",
}

// DateWheelView chooses a date on three wheels. It is created by [DateWheel].
type DateWheelView struct {
	value      Date
	bind       gift.Binding[Date]
	onChange   func(Date)
	min, max   Date
	order      DateOrder
	months     [12]string
	background Color
	hasBg      bool
	key, label string
	flex       float32
}

// DateWheel returns a date picker of a day, a month and a year wheel, the
// counterpart of SwiftUI's DatePicker with the wheel style. The day and the
// month wheels turn round, the year wheel ends at the range; a day that does
// not exist in the month chosen — the 31st of April — becomes the last day of
// that month, and a date outside the range becomes its nearest end, to which
// the wheels then turn.
//
// An empty value shows today without choosing it.
func DateWheel() DateWheelView {
	return DateWheelView{min: DateOf(1900, time.January, 1), max: DateOf(2100, time.December, 31), months: EnglishMonths}
}

// Value sets the date shown.
func (v DateWheelView) Value(d Date) DateWheelView { v.value = d; return v }

// Bind reads the date from b and writes the choice back.
func (v DateWheelView) Bind(b gift.Binding[Date]) DateWheelView { v.bind = b; return v }

// OnChange sets the function called with the date chosen.
func (v DateWheelView) OnChange(fn func(Date)) DateWheelView { v.onChange = fn; return v }

// Range limits the dates that can be chosen. The default is 1900 to 2100.
func (v DateWheelView) Range(min, max Date) DateWheelView {
	if !min.IsZero() && !max.IsZero() && !before(max, min) {
		v.min, v.max = min, max
	}
	return v
}

// Order sets the order of the wheels; see [DateOrder].
func (v DateWheelView) Order(o DateOrder) DateWheelView { v.order = o; return v }

// MonthNames sets the names of the month wheel, January first; see
// [GermanMonths].
func (v DateWheelView) MonthNames(names [12]string) DateWheelView { v.months = names; return v }

// Background sets the colour behind the wheels; see
// [WheelPickerView.Background].
func (v DateWheelView) Background(c Color) DateWheelView { v.background, v.hasBg = c, true; return v }

// Key sets the reconciliation key.
func (v DateWheelView) Key(s string) DateWheelView { v.key = s; return v }

// Label sets the accessible name.
func (v DateWheelView) Label(s string) DateWheelView { v.label = s; return v }

// Flex sets the flex factor in a stack.
func (v DateWheelView) Flex(f float32) DateWheelView { v.flex = f; return v }

// ViewType implements gift.View.
func (DateWheelView) ViewType() gift.TypeID { return dateWheelType }

// monthLength is the number of days of month in year.
func monthLength(year int, month time.Month) int {
	return int(daysIn(int32(month), int32(year)))
}

// before reports whether a is earlier than b.
func before(a, b Date) bool {
	if a.Year() != b.Year() {
		return a.Year() < b.Year()
	}
	if a.Month() != b.Month() {
		return a.Month() < b.Month()
	}
	return a.Day() < b.Day()
}

// clampDate is d inside [min, max], with a day that exists.
func clampDate(year int, month time.Month, day int, min, max Date) Date {
	day = minInt(day, monthLength(year, month))
	d := DateOf(year, month, day)
	switch {
	case before(d, min):
		return min
	case before(max, d):
		return max
	}
	return d
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Build implements gift.View.
func (v DateWheelView) Build(*gift.BuildContext) gift.Element {
	d := v.value
	if !v.bind.IsZero() {
		d = v.bind.Get()
	}
	if d.IsZero() {
		d = DateFrom(time.Now())
	}
	y, m, day := d.Year(), d.Month(), d.Day()
	set := func(year int, month time.Month, dd int) {
		nd := clampDate(year, month, dd, v.min, v.max)
		if !v.bind.IsZero() {
			v.bind.Set(nd)
		}
		if v.onChange != nil {
			v.onChange(nd)
		}
	}
	years := v.max.Year() - v.min.Year() + 1
	dayCol := Wheel(monthLength(y, m), func(i int) string { return strconv.Itoa(i + 1) }).
		Selected(day - 1).Cyclic(true).Width(64).Key("day").Label("Day").
		OnChange(func(i int) { set(y, m, i+1) })
	monthCol := Wheel(12, func(i int) string { return v.months[i] }).
		Selected(int(m) - 1).Cyclic(true).Width(150).Key("month").Label("Month").
		OnChange(func(i int) { set(y, time.Month(i+1), day) })
	yearCol := Wheel(years, func(i int) string { return strconv.Itoa(v.min.Year() + i) }).
		Selected(y - v.min.Year()).Width(88).Key("year").Label("Year").
		OnChange(func(i int) { set(v.min.Year()+i, m, day) })
	cols := []WheelColumn{dayCol, monthCol, yearCol}
	switch v.order {
	case MonthDayYear:
		cols = []WheelColumn{monthCol, dayCol, yearCol}
	case YearMonthDay:
		cols = []WheelColumn{yearCol, monthCol, dayCol}
	}
	p := WheelPicker(cols...).Label(v.label)
	if v.hasBg {
		p = p.Background(v.background)
	}
	return gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{p}, Flex: v.flex}
}

// TimeWheelView chooses a time of day on an hour and a minute wheel. It is
// created by [TimeWheel].
type TimeWheelView struct {
	value      TimeOfDay
	bind       gift.Binding[TimeOfDay]
	onChange   func(TimeOfDay)
	interval   int
	background Color
	hasBg      bool
	key, label string
	flex       float32
}

// TimeWheel returns a time picker of an hour and a minute wheel on the 24 hour
// clock, both turning round. An empty value shows the current time without
// choosing it.
func TimeWheel() TimeWheelView { return TimeWheelView{interval: 1} }

// Value sets the time shown.
func (v TimeWheelView) Value(t TimeOfDay) TimeWheelView { v.value = t; return v }

// Bind reads the time from b and writes the choice back.
func (v TimeWheelView) Bind(b gift.Binding[TimeOfDay]) TimeWheelView { v.bind = b; return v }

// OnChange sets the function called with the time chosen.
func (v TimeWheelView) OnChange(fn func(TimeOfDay)) TimeWheelView { v.onChange = fn; return v }

// MinuteInterval makes the minute wheel step by n minutes, as UIDatePicker's
// minuteInterval does; n divides 60. A time between two steps shows the step
// before it.
func (v TimeWheelView) MinuteInterval(n int) TimeWheelView {
	if n >= 1 && n <= 30 && 60%n == 0 {
		v.interval = n
	}
	return v
}

// Background sets the colour behind the wheels; see
// [WheelPickerView.Background].
func (v TimeWheelView) Background(c Color) TimeWheelView { v.background, v.hasBg = c, true; return v }

// Key sets the reconciliation key.
func (v TimeWheelView) Key(s string) TimeWheelView { v.key = s; return v }

// Label sets the accessible name.
func (v TimeWheelView) Label(s string) TimeWheelView { v.label = s; return v }

// Flex sets the flex factor in a stack.
func (v TimeWheelView) Flex(f float32) TimeWheelView { v.flex = f; return v }

// ViewType implements gift.View.
func (TimeWheelView) ViewType() gift.TypeID { return timeWheelType }

// Build implements gift.View.
func (v TimeWheelView) Build(*gift.BuildContext) gift.Element {
	t := v.value
	if !v.bind.IsZero() {
		t = v.bind.Get()
	}
	if t.IsZero() {
		t = TimeOfDayFrom(time.Now())
	}
	h, m := t.Hour(), t.Minute()
	set := func(hour, minute int) {
		nt := TimeOf(hour, minute)
		if !v.bind.IsZero() {
			v.bind.Set(nt)
		}
		if v.onChange != nil {
			v.onChange(nt)
		}
	}
	two := func(i int) string {
		if i < 10 {
			return "0" + strconv.Itoa(i)
		}
		return strconv.Itoa(i)
	}
	step := v.interval
	hourCol := Wheel(24, two).Selected(h).Cyclic(true).Width(80).Key("hour").Label("Hour").
		OnChange(func(i int) { set(i, m) })
	minuteCol := Wheel(60/step, func(i int) string { return two(i * step) }).
		Selected(m / step).Cyclic(true).Width(80).Key("minute").Label("Minute").
		OnChange(func(i int) { set(h, i*step) })
	p := WheelPicker(hourCol, minuteCol).Label(v.label)
	if v.hasBg {
		p = p.Background(v.background)
	}
	return gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{p}, Flex: v.flex}
}
