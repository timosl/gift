package grib2_test

import (
	"math"
	"testing"
	"time"

	"github.com/worldiety/gift/internal/gribwrite"
	"github.com/worldiety/gift/weather/grib2"
)

func TestAFieldIsDecodedAsItWasWritten(t *testing.T) {
	ref := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	for _, south := range []bool{false, true} {
		f := gribwrite.Field{
			Reference: ref, Forecast: 3 * time.Hour, Category: 6, Number: 22, Surface: 100, Level: 80000,
			Ni: 5, Nj: 4, Lat0: 54, Lon0: 7, DLat: 0.5, DLon: 0.25, SouthFirst: south, DecimalScale: 1,
		}
		for i := range f.Ni * f.Nj {
			f.Values = append(f.Values, float32(i)*2.5)
		}
		f.Values[7] = float32(math.NaN())
		ms, err := grib2.Decode(gribwrite.Encode(f))
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) != 1 {
			t.Fatalf("%d messages", len(ms))
		}
		m := ms[0]
		if m.Category != 6 || m.Number != 22 || m.Surface != 100 || m.Level != 80000 || m.Forecast != 3*time.Hour || !m.Reference.Equal(ref) {
			t.Errorf("header %+v", m)
		}
		if m.Grid.Ni != 5 || m.Grid.Nj != 4 || m.Grid.Lat0 != 54 || m.Grid.Lon0 != 7 || m.Grid.DLat != 0.5 || m.Grid.DLon != 0.25 {
			t.Errorf("south first %v: grid %+v", south, m.Grid)
		}
		for i, v := range m.Values {
			want := f.Values[i]
			if math.IsNaN(float64(want)) {
				if !math.IsNaN(float64(v)) {
					t.Errorf("south first %v: value %d is %v, want missing", south, i, v)
				}
				continue
			}
			if math.Abs(float64(v-want)) > 0.06 {
				t.Errorf("south first %v: value %d is %v, want %v", south, i, v, want)
			}
		}
	}
}

func TestAnotherTemplateIsRefusedByName(t *testing.T) {
	b := gribwrite.Encode(gribwrite.Field{Reference: time.Now(), Ni: 2, Nj: 2, DLat: 1, DLon: 1, Values: []float32{1, 2, 3, 4}})
	// Grid definition template number, octets 13 and 14 of section 3.
	i := 16 + 21
	b[i+12], b[i+13] = 0, 40
	if _, err := grib2.Decode(b); err == nil || !contains(err.Error(), "3.40") {
		t.Fatalf("error %v, want one naming template 3.40", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
