// Package exifwrite writes JPEG files with the EXIF of a photograph: where
// and when it was taken. It exists for the tests of package asset and for
// the sample photographs of the map demo, which need a catalogue with
// locations that nobody has to check in.
package exifwrite

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"math"
	"time"
)

// JPEG encodes img and inserts an APP1 segment with a GPS IFD for lat, lon
// and DateTimeOriginal for taken. A zero taken writes no date.
func JPEG(img image.Image, lat, lon float64, taken time.Time) ([]byte, error) {
	var enc bytes.Buffer
	if err := jpeg.Encode(&enc, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	b := enc.Bytes()
	tiff := TIFF(lat, lon, taken)
	seg := make([]byte, 0, 10+len(tiff))
	seg = append(seg, 0xFF, 0xE1)
	seg = binary.BigEndian.AppendUint16(seg, uint16(2+6+len(tiff)))
	seg = append(seg, "Exif\x00\x00"...)
	seg = append(seg, tiff...)
	out := make([]byte, 0, len(b)+len(seg))
	out = append(out, b[:2]...)
	out = append(out, seg...)
	return append(out, b[2:]...), nil
}

// TIFF returns the TIFF structure of an EXIF block, little endian: IFD0
// with the pointers to the EXIF sub-IFD and the GPS IFD, then those two.
func TIFF(lat, lon float64, taken time.Time) []byte {
	le := binary.LittleEndian
	type entry struct {
		tag, typ uint16
		count    uint32
		value    []byte // inline when four bytes or fewer
	}
	var buf []byte
	buf = append(buf, 'I', 'I', 42, 0, 8, 0, 0, 0)
	// Layout: IFD0 at 8 with two entries, the EXIF IFD, the GPS IFD, then
	// the data the entries point to.
	const ifd0, ifd0N = 8, 2
	exifOff := ifd0 + 2 + 12*ifd0N + 4
	exifN := 1
	gpsOff := exifOff + 2 + 12*exifN + 4
	gpsN := 4
	dataOff := gpsOff + 2 + 12*gpsN + 4

	var data []byte
	put := func(v []byte) uint32 {
		off := uint32(dataOff + len(data))
		data = append(data, v...)
		if len(data)%2 == 1 {
			data = append(data, 0)
		}
		return off
	}
	u32 := func(v uint32) []byte { return le.AppendUint32(nil, v) }
	rational := func(d float64) []byte {
		d = math.Abs(d)
		deg := math.Floor(d)
		minF := (d - deg) * 60
		min := math.Floor(minF)
		sec := (minF - min) * 60
		var b []byte
		for _, v := range [][2]uint32{{uint32(deg), 1}, {uint32(min), 1}, {uint32(math.Round(sec * 10000)), 10000}} {
			b = le.AppendUint32(b, v[0])
			b = le.AppendUint32(b, v[1])
		}
		return b
	}
	writeIFD := func(es []entry) []byte {
		var b []byte
		b = le.AppendUint16(b, uint16(len(es)))
		for _, e := range es {
			b = le.AppendUint16(b, e.tag)
			b = le.AppendUint16(b, e.typ)
			b = le.AppendUint32(b, e.count)
			v := make([]byte, 4)
			copy(v, e.value)
			b = append(b, v...)
		}
		return le.AppendUint32(b, 0)
	}
	date := []byte(taken.Format("2006:01:02 15:04:05") + "\x00")
	var exifEntries []entry
	if !taken.IsZero() {
		exifEntries = []entry{{0x9003, 2, uint32(len(date)), u32(put(date))}}
	} else {
		exifEntries = []entry{{0x9000, 7, 4, []byte("0232")}}
	}
	latRef, lonRef := "N\x00", "E\x00"
	if lat < 0 {
		latRef = "S\x00"
	}
	if lon < 0 {
		lonRef = "W\x00"
	}
	gpsEntries := []entry{
		{1, 2, 2, []byte(latRef)},
		{2, 5, 3, u32(put(rational(lat)))},
		{3, 2, 2, []byte(lonRef)},
		{4, 5, 3, u32(put(rational(lon)))},
	}
	buf = append(buf, writeIFD([]entry{
		{0x8769, 4, 1, u32(uint32(exifOff))},
		{0x8825, 4, 1, u32(uint32(gpsOff))},
	})...)
	buf = append(buf, writeIFD(exifEntries)...)
	buf = append(buf, writeIFD(gpsEntries)...)
	return append(buf, data...)
}
