// Package grib2 decodes the GRIB edition 2 messages weather models are
// published in, as far as the providers of package weather need: a regular
// latitude and longitude grid (grid definition template 3.0), simple packing
// (data representation template 5.0) and a bitmap of missing points. That
// is what the Deutscher Wetterdienst serves ICON-D2 in and what NOAA's NOMADS
// filter repacks GFS into, both checked on 31 October 2026; other templates
// are refused with an error that names them.
package grib2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"
)

// Message is one decoded field.
type Message struct {
	// Discipline, Category and Number name the parameter in the WMO's code
	// tables: discipline 0 is meteorology, category 6 number 22 is the
	// cover of the low clouds, say.
	Discipline, Category, Number uint8
	// ProductTemplate is the template of the product definition; 0 is an
	// instant, 8 a statistic over an interval.
	ProductTemplate uint16
	// Reference is the time of the run; Forecast is how far after it the
	// field is valid, at the end of the interval of a statistic.
	Reference time.Time
	Forecast  time.Duration
	// Surface is the type of the first fixed surface and Level its value:
	// 103 at 10 for ten metres above the ground, 100 at 50000 for 500 hPa.
	Surface uint8
	Level   float64
	end     time.Time
	// Grid is the grid, normalised: row 0 is the northernmost, column 0 the
	// westernmost, longitudes in ±180.
	Grid Grid
	// Values holds Ni×Nj values row by row; a missing one is NaN.
	Values []float32
}

// Grid is a regular latitude and longitude grid.
type Grid struct {
	Ni, Nj     int     // columns and rows
	Lat0, Lon0 float64 // the north west point
	DLat, DLon float64 // positive steps, southward and eastward
}

// Decode decodes every message in b.
func Decode(b []byte) ([]Message, error) {
	var out []Message
	for len(b) >= 16 {
		if string(b[:4]) != "GRIB" {
			return out, errors.New("grib2: no GRIB message")
		}
		if b[7] != 2 {
			return out, fmt.Errorf("grib2: edition %d, only 2 is supported", b[7])
		}
		n := binary.BigEndian.Uint64(b[8:16])
		if n < 16 || n > uint64(len(b)) {
			return out, errors.New("grib2: message longer than the data")
		}
		m, err := decodeMessage(b[:n])
		if err != nil {
			return out, err
		}
		out = append(out, m)
		b = b[n:]
	}
	return out, nil
}

type sections struct {
	s1, s3, s4, s5, s6, s7 []byte
}

func decodeMessage(b []byte) (Message, error) {
	m := Message{Discipline: b[6]}
	var s sections
	for i := 16; i+5 <= len(b); {
		if string(b[i:i+4]) == "7777" {
			break
		}
		l := int(binary.BigEndian.Uint32(b[i:]))
		if l < 5 || i+l > len(b) {
			return m, errors.New("grib2: section longer than the message")
		}
		sec := b[i : i+l]
		switch sec[4] {
		case 1:
			s.s1 = sec
		case 3:
			s.s3 = sec
		case 4:
			s.s4 = sec
		case 5:
			s.s5 = sec
		case 6:
			s.s6 = sec
		case 7:
			s.s7 = sec
		}
		i += l
	}
	if s.s1 == nil || s.s3 == nil || s.s4 == nil || s.s5 == nil || s.s7 == nil {
		return m, errors.New("grib2: a section is missing")
	}
	if len(s.s1) < 19 {
		return m, errors.New("grib2: short identification section")
	}
	m.Reference = time.Date(int(binary.BigEndian.Uint16(s.s1[12:])), time.Month(s.s1[14]), int(s.s1[15]),
		int(s.s1[16]), int(s.s1[17]), int(s.s1[18]), 0, time.UTC)
	g, scan, err := grid(s.s3)
	if err != nil {
		return m, err
	}
	if err := product(&m, s.s4); err != nil {
		return m, err
	}
	if !m.end.IsZero() {
		m.Forecast = m.end.Sub(m.Reference)
	}
	vals, err := unpack(s.s5, s.s6, s.s7, g.Ni*g.Nj)
	if err != nil {
		return m, err
	}
	m.Grid, m.Values = g, normalise(vals, g, scan)
	return m, nil
}

// sm reads a sign and magnitude integer: GRIB2 does not use two's
// complement.
func sm32(b []byte) int64 {
	v := binary.BigEndian.Uint32(b)
	if v&0x80000000 != 0 {
		return -int64(v & 0x7fffffff)
	}
	return int64(v)
}

func sm16(b []byte) int {
	v := binary.BigEndian.Uint16(b)
	if v&0x8000 != 0 {
		return -int(v & 0x7fff)
	}
	return int(v)
}

// grid reads grid definition template 3.0 and returns the grid as scanned
// and its scanning mode.
func grid(s3 []byte) (Grid, uint8, error) {
	if len(s3) < 14 {
		return Grid{}, 0, errors.New("grib2: short grid section")
	}
	if t := binary.BigEndian.Uint16(s3[12:]); t != 0 {
		return Grid{}, 0, fmt.Errorf("grib2: grid definition template 3.%d, only 3.0 is supported", t)
	}
	if len(s3) < 72 {
		return Grid{}, 0, errors.New("grib2: short grid template")
	}
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(s3[o:])) }
	ni, nj := u32(30), u32(34)
	basic, sub := u32(38), u32(42)
	unit := 1e-6
	if basic != 0 && basic != 0xffffffff && sub != 0 && sub != 0xffffffff {
		unit = float64(basic) / float64(sub)
	}
	la1 := float64(sm32(s3[46:])) * unit
	lo1 := float64(sm32(s3[50:])) * unit
	la2 := float64(sm32(s3[55:])) * unit
	lo2 := float64(sm32(s3[59:])) * unit
	di := float64(u32(63)) * unit
	dj := float64(u32(67)) * unit
	scan := s3[71]
	if ni <= 0 || nj <= 0 || ni*nj > 1<<26 {
		return Grid{}, 0, fmt.Errorf("grib2: grid of %d by %d", ni, nj)
	}
	north, west := math.Max(la1, la2), lo1
	if scan&0x80 != 0 {
		west = lo2
	}
	if west > 180 {
		west -= 360
	}
	return Grid{Ni: ni, Nj: nj, Lat0: north, Lon0: west, DLat: dj, DLon: di}, scan, nil
}

// product reads the parameter, the forecast time and the surface of product
// definition templates 4.0 and 4.8.
func product(m *Message, s4 []byte) error {
	if len(s4) < 34 {
		return errors.New("grib2: short product section")
	}
	m.ProductTemplate = binary.BigEndian.Uint16(s4[7:])
	if m.ProductTemplate != 0 && m.ProductTemplate != 8 {
		return fmt.Errorf("grib2: product definition template 4.%d, only 4.0 and 4.8 are supported", m.ProductTemplate)
	}
	m.Category, m.Number = s4[9], s4[10]
	unit := timeUnit(s4[17])
	m.Forecast = time.Duration(binary.BigEndian.Uint32(s4[18:])) * unit
	m.Surface = s4[22]
	scale := int(int8(s4[23]))
	m.Level = float64(sm32(s4[24:])) / math.Pow(10, float64(scale))
	if m.ProductTemplate == 8 && len(s4) >= 41 {
		// A statistic is valid at the end of its interval, which the
		// template states as a time, octets 35 to 41.
		end := time.Date(int(binary.BigEndian.Uint16(s4[34:])), time.Month(s4[36]), int(s4[37]),
			int(s4[38]), int(s4[39]), int(s4[40]), 0, time.UTC)
		m.end = end
	}
	return nil
}

func timeUnit(code uint8) time.Duration {
	switch code {
	case 0:
		return time.Minute
	case 1:
		return time.Hour
	case 2:
		return 24 * time.Hour
	case 10:
		return 3 * time.Hour
	case 11:
		return 6 * time.Hour
	case 12:
		return 12 * time.Hour
	case 13:
		return time.Second
	}
	return time.Hour
}

// unpack decodes simple packing, template 5.0, and spreads the values over
// the points by the bitmap.
func unpack(s5, s6, s7 []byte, points int) ([]float32, error) {
	if len(s5) < 21 {
		return nil, errors.New("grib2: short data representation section")
	}
	if t := binary.BigEndian.Uint16(s5[9:]); t != 0 {
		return nil, fmt.Errorf("grib2: data representation template 5.%d, only 5.0 is supported", t)
	}
	n := int(binary.BigEndian.Uint32(s5[5:]))
	ref := float64(math.Float32frombits(binary.BigEndian.Uint32(s5[11:])))
	e := sm16(s5[15:])
	d := sm16(s5[17:])
	bits := int(s5[19])
	scale := math.Pow(2, float64(e))
	dec := math.Pow(10, float64(-d))
	data := s7[5:]
	vals := make([]float32, n)
	if bits == 0 {
		for i := range vals {
			vals[i] = float32(ref * dec)
		}
	} else {
		if (n*bits+7)/8 > len(data) {
			return nil, errors.New("grib2: packed data shorter than its values")
		}
		var acc uint64
		have, pos := 0, 0
		for i := range vals {
			for have < bits {
				acc = acc<<8 | uint64(data[pos])
				pos++
				have += 8
			}
			x := (acc >> uint(have-bits)) & (1<<uint(bits) - 1)
			have -= bits
			vals[i] = float32((ref + float64(x)*scale) * dec)
		}
	}
	if s6 == nil || len(s6) < 6 || s6[5] == 255 {
		if n != points {
			return nil, fmt.Errorf("grib2: %d values for %d points and no bitmap", n, points)
		}
		return vals, nil
	}
	if s6[5] != 0 {
		return nil, fmt.Errorf("grib2: bitmap indicator %d is not supported", s6[5])
	}
	bm := s6[6:]
	if (points+7)/8 > len(bm) {
		return nil, errors.New("grib2: bitmap shorter than the grid")
	}
	out := make([]float32, points)
	nan := float32(math.NaN())
	k := 0
	for i := range out {
		if bm[i/8]&(0x80>>(i%8)) != 0 && k < len(vals) {
			out[i] = vals[k]
			k++
		} else {
			out[i] = nan
		}
	}
	return out, nil
}

// normalise reorders values scanned in any of the row-wise orders into rows
// from the north, columns from the west.
func normalise(v []float32, g Grid, scan uint8) []float32 {
	if scan&0x20 != 0 {
		// Columns consecutive: not used by the providers, read as rows.
		return v
	}
	westward := scan&0x80 != 0
	northward := scan&0x40 != 0
	if !westward && !northward {
		return v
	}
	out := make([]float32, len(v))
	for r := range g.Nj {
		src := r
		if northward {
			src = g.Nj - 1 - r
		}
		for c := range g.Ni {
			sc := c
			if westward {
				sc = g.Ni - 1 - c
			}
			out[r*g.Ni+c] = v[src*g.Ni+sc]
		}
	}
	return out
}
