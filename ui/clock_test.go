package ui_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/worldiety/gift/ui"
)

func TestATimeAndADateSurviveJSON(t *testing.T) {
	type record struct {
		Day   ui.Date
		Start ui.TimeOfDay
		End   ui.TimeOfDay
	}
	in := record{Day: ui.DateOf(2026, time.October, 26), Start: ui.TimeOf(0, 0)}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"Day":"2026-10-26","Start":"00:00","End":""}`; got != want {
		t.Fatalf("json.Marshal = %s, want %s", got, want)
	}
	var out record
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("the round trip gave %+v, want %+v", out, in)
	}
}

func TestUnmarshalTextTakesOnlyWhatMarshalTextWrites(t *testing.T) {
	for _, s := range []string{"9:30", "0930", "24:00", "09:60", "09:3x", " 09:30"} {
		var v ui.TimeOfDay
		if err := v.UnmarshalText([]byte(s)); err == nil {
			t.Errorf("TimeOfDay.UnmarshalText(%q) did not fail", s)
		}
	}
	for _, s := range []string{"26.10.2026", "2026-1-26", "2026-02-29", "0000-01-01", "2026-13-01"} {
		var d ui.Date
		if err := d.UnmarshalText([]byte(s)); err == nil {
			t.Errorf("Date.UnmarshalText(%q) did not fail", s)
		}
	}
	var d ui.Date
	if err := d.UnmarshalText([]byte("2024-02-29")); err != nil || d != ui.DateOf(2024, time.February, 29) {
		t.Errorf("Date.UnmarshalText(2024-02-29) = %v, %v", d, err)
	}
}

func TestTimeOfDayFromAndDateAt(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no time zone database:", err)
	}
	now := time.Date(2026, time.October, 26, 14, 37, 59, 0, berlin)
	if got := ui.TimeOfDayFrom(now); got != ui.TimeOf(14, 37) {
		t.Errorf("TimeOfDayFrom = %v, want 14:37", got)
	}
	if got := ui.TimeOfDayFrom(now.UTC()); got != ui.TimeOf(13, 37) {
		t.Errorf("TimeOfDayFrom in UTC = %v, want 13:37", got)
	}
	if got := ui.TimeOfDayAfter(90 * time.Minute); got != ui.TimeOf(1, 30) {
		t.Errorf("TimeOfDayAfter(90m) = %v, want 01:30", got)
	}

	day := ui.DateFrom(now)
	if got, want := day.At(ui.TimeOf(14, 37), berlin), now.Truncate(time.Minute); !got.Equal(want) {
		t.Errorf("Date.At = %v, want %v", got, want)
	}
	if got := day.At(ui.TimeOfDay{}, berlin); !got.Equal(day.Time(berlin)) {
		t.Errorf("Date.At(no time) = %v, want midnight", got)
	}
	if got := (ui.Date{}).At(ui.TimeOf(9, 0), berlin); !got.IsZero() {
		t.Errorf("no date At 09:00 = %v, want the zero time", got)
	}
}
