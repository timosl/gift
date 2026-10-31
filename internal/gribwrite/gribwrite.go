// Package gribwrite writes GRIB edition 2 messages of the one kind package
// weather/grib2 reads — a regular latitude and longitude grid with simple
// packing and an optional bitmap — for the tests of the decoder and of the
// weather providers, which need fields nobody has to download.
package gribwrite

import (
	"encoding/binary"
	"math"
	"time"
)

// Field is one field to encode. Values run row by row from (Lat0, Lon0);
// SouthFirst scans from the south, as the Deutscher Wetterdienst does. A NaN
// value is left out and marked missing in the bitmap.
type Field struct {
	Reference              time.Time
	Forecast               time.Duration
	Category, Number       uint8
	Surface                uint8
	Level                  int32
	Ni, Nj                 int
	Lat0, Lon0, DLat, DLon float64
	SouthFirst             bool
	Values                 []float32
	DecimalScale           int
	Bits                   int
}

// Encode returns the message of f.
func Encode(f Field) []byte {
	if f.Bits == 0 {
		f.Bits = 16
	}
	be := binary.BigEndian
	section := func(num byte, body []byte) []byte {
		out := be.AppendUint32(nil, uint32(5+len(body)))
		out = append(out, num)
		return append(out, body...)
	}
	sm32 := func(v int64) uint32 {
		if v < 0 {
			return uint32(-v) | 0x80000000
		}
		return uint32(v)
	}
	// Section 1: identification.
	s1 := []byte{0, 78, 0, 0, 2, 1, 1}
	s1 = be.AppendUint16(s1, uint16(f.Reference.Year()))
	s1 = append(s1, byte(f.Reference.Month()), byte(f.Reference.Day()), byte(f.Reference.Hour()),
		byte(f.Reference.Minute()), byte(f.Reference.Second()), 0, 1)
	// Section 3: grid definition template 3.0.
	s3 := []byte{0}
	s3 = be.AppendUint32(s3, uint32(f.Ni*f.Nj))
	s3 = append(s3, 0, 0)
	s3 = be.AppendUint16(s3, 0)
	s3 = append(s3, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	s3 = be.AppendUint32(s3, uint32(f.Ni))
	s3 = be.AppendUint32(s3, uint32(f.Nj))
	s3 = be.AppendUint32(s3, 0)
	s3 = be.AppendUint32(s3, 0xffffffff)
	la1, la2 := f.Lat0, f.Lat0-float64(f.Nj-1)*f.DLat
	scan := byte(0)
	if f.SouthFirst {
		la1, la2, scan = la2, la1, 0x40
	}
	lo2 := f.Lon0 + float64(f.Ni-1)*f.DLon
	e6 := func(v float64) uint32 { return sm32(int64(math.Round(v * 1e6))) }
	s3 = be.AppendUint32(s3, e6(la1))
	s3 = be.AppendUint32(s3, e6(f.Lon0))
	s3 = append(s3, 48)
	s3 = be.AppendUint32(s3, e6(la2))
	s3 = be.AppendUint32(s3, e6(lo2))
	s3 = be.AppendUint32(s3, uint32(math.Round(f.DLon*1e6)))
	s3 = be.AppendUint32(s3, uint32(math.Round(f.DLat*1e6)))
	s3 = append(s3, scan)
	// Section 4: product definition template 4.0.
	s4 := be.AppendUint16(nil, 0)
	s4 = be.AppendUint16(s4, 0)
	s4 = append(s4, f.Category, f.Number, 2, 0, 0, 0, 0, 0, 1)
	s4 = be.AppendUint32(s4, uint32(f.Forecast/time.Hour))
	s4 = append(s4, f.Surface, 0)
	s4 = be.AppendUint32(s4, sm32(int64(f.Level)))
	s4 = append(s4, 255, 0, 0, 0, 0, 0)
	// The values in scanning order, and the bitmap.
	vals := f.Values
	if f.SouthFirst {
		vals = make([]float32, len(f.Values))
		for r := range f.Nj {
			copy(vals[r*f.Ni:(r+1)*f.Ni], f.Values[(f.Nj-1-r)*f.Ni:(f.Nj-r)*f.Ni])
		}
	}
	var present []float64
	bitmap := make([]byte, (len(vals)+7)/8)
	missing := false
	dec := math.Pow(10, float64(f.DecimalScale))
	for i, v := range vals {
		if math.IsNaN(float64(v)) {
			missing = true
			continue
		}
		bitmap[i/8] |= 0x80 >> (i % 8)
		present = append(present, float64(v)*dec)
	}
	ref := math.Inf(1)
	hi := math.Inf(-1)
	for _, v := range present {
		ref, hi = math.Min(ref, v), math.Max(hi, v)
	}
	if len(present) == 0 {
		ref, hi = 0, 0
	}
	// A binary scale that fits the range into the bits.
	e := 0
	for hi-ref > 0 && (hi-ref)/math.Pow(2, float64(e)) > float64(uint64(1)<<f.Bits-1) {
		e++
	}
	for e > -30 && hi-ref > 0 && (hi-ref)/math.Pow(2, float64(e-1)) <= float64(uint64(1)<<f.Bits-1) {
		e--
	}
	s5 := be.AppendUint32(nil, uint32(len(present)))
	s5 = be.AppendUint16(s5, 0)
	s5 = be.AppendUint32(s5, math.Float32bits(float32(ref)))
	sm16 := func(v int) uint16 {
		if v < 0 {
			return uint16(-v) | 0x8000
		}
		return uint16(v)
	}
	s5 = be.AppendUint16(s5, sm16(e))
	s5 = be.AppendUint16(s5, sm16(f.DecimalScale))
	s5 = append(s5, byte(f.Bits), 0)
	var s6 []byte
	if missing {
		s6 = append([]byte{0}, bitmap...)
	} else {
		s6 = []byte{255}
	}
	var packed []byte
	var acc uint64
	have := 0
	scale := math.Pow(2, float64(e))
	for _, v := range present {
		x := uint64(math.Round((v - float32ref(ref)) / scale))
		acc = acc<<uint(f.Bits) | x
		have += f.Bits
		for have >= 8 {
			packed = append(packed, byte(acc>>uint(have-8)))
			have -= 8
		}
	}
	if have > 0 {
		packed = append(packed, byte(acc<<uint(8-have)))
	}
	body := section(1, s1)
	body = append(body, section(3, s3)...)
	body = append(body, section(4, s4)...)
	body = append(body, section(5, s5)...)
	body = append(body, section(6, s6)...)
	body = append(body, section(7, packed)...)
	body = append(body, "7777"...)
	head := []byte{'G', 'R', 'I', 'B', 0, 0, 0, 2}
	head = be.AppendUint64(head, uint64(16+len(body)))
	return append(head, body...)
}

// float32ref is the reference value as the decoder sees it, rounded to a
// float32.
func float32ref(v float64) float64 { return float64(float32(v)) }
