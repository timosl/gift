package ui

import (
	"fmt"
	"strconv"
	"time"
)

// TimeOfDay is a time on a 24 hour clock, to the minute, or no time at all.
// It is the value of a [TimeFieldView].
//
// The zero value is *no* time — an empty field — and not midnight; midnight
// is TimeOf(0, 0). A form needs both, and a field that could not be empty
// would have to show a time nobody entered.
//
// It is a small comparable value, so it can be kept in a [gift.State] and
// compared with ==. Its fields are unexported so that an hour of 25 cannot be
// constructed: every TimeOfDay is either empty or a real time.
type TimeOfDay struct {
	hour, minute int8
	set          bool
}

// TimeOf returns the time hour:minute. It panics on an hour outside 0 to 23
// or a minute outside 0 to 59; a value that would have to be corrected is a
// mistake at the call site and not something to round.
func TimeOf(hour, minute int) TimeOfDay {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		panic(fmt.Sprintf("gift/ui: TimeOf(%d, %d) is not a time on a 24 hour clock", hour, minute))
	}
	return TimeOfDay{hour: int8(hour), minute: int8(minute), set: true}
}

// TimeOfDayFrom returns the time d after midnight, truncated to the minute.
// It panics on a duration outside [0, 24h), for the reason [TimeOf] does.
func TimeOfDayFrom(d time.Duration) TimeOfDay {
	if d < 0 || d >= 24*time.Hour {
		panic(fmt.Sprintf("gift/ui: TimeOfDayFrom(%v) is not within one day", d))
	}
	m := int(d / time.Minute)
	return TimeOf(m/60, m%60)
}

// IsZero reports whether t is no time at all.
func (t TimeOfDay) IsZero() bool { return !t.set }

// Hour returns the hour, 0 to 23, and 0 for the zero value.
func (t TimeOfDay) Hour() int { return int(t.hour) }

// Minute returns the minute, 0 to 59, and 0 for the zero value.
func (t TimeOfDay) Minute() int { return int(t.minute) }

// Minutes returns the number of minutes since midnight.
func (t TimeOfDay) Minutes() int { return int(t.hour)*60 + int(t.minute) }

// Duration returns the time since midnight.
func (t TimeOfDay) Duration() time.Duration { return time.Duration(t.Minutes()) * time.Minute }

// String formats t as "hh:mm", and the zero value as "".
func (t TimeOfDay) String() string {
	if !t.set {
		return ""
	}
	return pad2(int(t.hour)) + ":" + pad2(int(t.minute))
}

// ParseTimeOfDay reads a time the way a [TimeFieldView] accepts one from the
// clipboard: "9:30", "09:30", "9.30", "930", "0930", or an hour on its own,
// "9" or "09". Surrounding white space is ignored. The empty string is the
// zero TimeOfDay and no error.
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	vals, ok, empty := parseClock(s)
	if empty {
		return TimeOfDay{}, nil
	}
	if !ok {
		return TimeOfDay{}, fmt.Errorf("gift/ui: %q is not a time of day", s)
	}
	return TimeOf(int(vals[0]), int(vals[1])), nil
}

// Date is a day in the proleptic Gregorian calendar, without a time and
// without a location, or no day at all. It is the value of a [DateFieldView].
//
// It is to a [time.Time] what [TimeOfDay] is to a [time.Duration]: the zero
// value is *no* date and not the first of January of year one, it is
// comparable, and every Date that is not zero is a real day.
type Date struct {
	year  int16
	month int8
	day   int8
}

// DateOf returns the given day. It panics on a year outside 1 to 9999, a month
// outside January to December, or a day that the month does not have.
func DateOf(year int, month time.Month, day int) Date {
	if year < 1 || year > 9999 || month < time.January || month > time.December ||
		day < 1 || day > int(daysIn(int32(month), int32(year))) {
		panic(fmt.Sprintf("gift/ui: DateOf(%d, %d, %d) is not a day", year, month, day))
	}
	return Date{year: int16(year), month: int8(month), day: int8(day)}
}

// DateFrom returns the day t falls on in t's own location.
func DateFrom(t time.Time) Date {
	y, m, d := t.Date()
	return DateOf(y, m, d)
}

// IsZero reports whether d is no day at all.
func (d Date) IsZero() bool { return d.month == 0 }

// Year, Month and Day return the parts of the date, all zero for the zero
// value.
func (d Date) Year() int         { return int(d.year) }
func (d Date) Month() time.Month { return time.Month(d.month) }
func (d Date) Day() int          { return int(d.day) }

// Time returns midnight at the start of d in loc. The zero Date returns the
// zero [time.Time].
func (d Date) Time(loc *time.Location) time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	return time.Date(int(d.year), time.Month(d.month), int(d.day), 0, 0, 0, 0, loc)
}

// String formats d as "YYYY-MM-DD", the order that sorts, and the zero value
// as "". A field shows it as "DD.MM.YYYY"; see [DateFieldView].
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	y := strconv.Itoa(int(d.year))
	for len(y) < 4 {
		y = "0" + y
	}
	return y + "-" + pad2(int(d.month)) + "-" + pad2(int(d.day))
}

// ParseDate reads a date the way a [DateFieldView] accepts one from the
// clipboard: "26.10.2026", "26.10.26", "2026-10-26", "26/10/2026", "26102026"
// or "261026". A two digit year is in the twenty-first century. Surrounding
// white space is ignored. The empty string is the zero Date and no error.
func ParseDate(s string) (Date, error) {
	vals, ok, empty := parseCalendar(s)
	if empty {
		return Date{}, nil
	}
	if !ok {
		return Date{}, fmt.Errorf("gift/ui: %q is not a date", s)
	}
	return DateOf(int(vals[2]), time.Month(vals[1]), int(vals[0])), nil
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

// daysIn is the length of a month. A year of zero is taken as a leap year, so
// that the 29th of February can be typed before the year is.
func daysIn(month, year int32) int32 {
	if year == 0 {
		year = 2000
	}
	return int32(time.Date(int(year), time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day())
}
