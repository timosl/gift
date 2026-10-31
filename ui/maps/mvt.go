package maps

import (
	"encoding/binary"
	"errors"
	"math"
	"strconv"
)

// The Mapbox Vector Tile format, version 2: a protocol buffer of layers, each
// with its features, its keys and its values. Three message types are all it
// takes, and a decoder for them is shorter than the dependency a protobuf
// library would be.
//
// The decoder keeps the tile's bytes and points into them: names and
// geometries are slices of the input, never copies, because a tile of a
// city centre holds thousands of features of which a style draws most and
// labels a few.

// geomType is a feature's geometry type.
type geomType uint8

const (
	geomUnknown geomType = iota
	geomPoint
	geomLine
	geomPolygon
)

type mvtTile struct {
	layers []mvtLayer
}

type mvtLayer struct {
	name     string
	extent   int
	keys     []string
	values   []mvtValue
	features []mvtFeature
}

// mvtValue is a property value: a string, or a number, or a bool as 0 or 1.
type mvtValue struct {
	s     string
	n     float64
	isStr bool
}

func (v mvtValue) String() string {
	if v.isStr {
		return v.s
	}
	return strconv.FormatFloat(v.n, 'g', -1, 64)
}

type mvtFeature struct {
	typ  geomType
	tags []uint32
	geom []uint32
}

var errMVT = errors.New("maps: malformed vector tile")

// decodeMVT decodes the layers of a tile. Layers not in want are skipped
// without decoding their features; a nil want decodes all.
func decodeMVT(b []byte, want func(string) bool) (*mvtTile, error) {
	t := &mvtTile{}
	for i := 0; i < len(b); {
		f, wt, v, n, err := field(b, i)
		if err != nil {
			return nil, err
		}
		i = n
		if f == 3 && wt == 2 {
			l, err := decodeLayer(b[v.start:v.end], want)
			if err != nil {
				return nil, err
			}
			if l != nil {
				t.layers = append(t.layers, *l)
			}
		}
	}
	return t, nil
}

func (t *mvtTile) layer(name string) *mvtLayer {
	for i := range t.layers {
		if t.layers[i].name == name {
			return &t.layers[i]
		}
	}
	return nil
}

// span is a field's payload: a varint value, or the range of a length
// delimited one.
type span struct {
	u          uint64
	start, end int
}

// field reads the field at i and returns its number, wire type, payload and
// the index after it.
func field(b []byte, i int) (num int, wire int, v span, next int, err error) {
	key, n := binary.Uvarint(b[i:])
	if n <= 0 {
		return 0, 0, v, 0, errMVT
	}
	i += n
	num, wire = int(key>>3), int(key&7)
	switch wire {
	case 0:
		u, n := binary.Uvarint(b[i:])
		if n <= 0 {
			return 0, 0, v, 0, errMVT
		}
		v.u = u
		i += n
	case 1:
		if i+8 > len(b) {
			return 0, 0, v, 0, errMVT
		}
		v.u = binary.LittleEndian.Uint64(b[i:])
		i += 8
	case 2:
		l, n := binary.Uvarint(b[i:])
		if n <= 0 || i+n+int(l) > len(b) {
			return 0, 0, v, 0, errMVT
		}
		v.start, v.end = i+n, i+n+int(l)
		i = v.end
	case 5:
		if i+4 > len(b) {
			return 0, 0, v, 0, errMVT
		}
		v.u = uint64(binary.LittleEndian.Uint32(b[i:]))
		i += 4
	default:
		return 0, 0, v, 0, errMVT
	}
	return num, wire, v, i, nil
}

func decodeLayer(b []byte, want func(string) bool) (*mvtLayer, error) {
	l := &mvtLayer{extent: 4096}
	// The name comes first in every encoder, but the format does not
	// promise it; the features are collected and decoded after the loop.
	var feats []span
	for i := 0; i < len(b); {
		f, wt, v, n, err := field(b, i)
		if err != nil {
			return nil, err
		}
		i = n
		switch {
		case f == 1 && wt == 2:
			l.name = string(b[v.start:v.end])
		case f == 2 && wt == 2:
			feats = append(feats, v)
		case f == 3 && wt == 2:
			l.keys = append(l.keys, string(b[v.start:v.end]))
		case f == 4 && wt == 2:
			val, err := decodeValue(b[v.start:v.end])
			if err != nil {
				return nil, err
			}
			l.values = append(l.values, val)
		case f == 5 && wt == 0:
			l.extent = int(v.u)
		}
	}
	if want != nil && !want(l.name) {
		return nil, nil
	}
	l.features = make([]mvtFeature, 0, len(feats))
	for _, fs := range feats {
		ft, err := decodeFeature(b[fs.start:fs.end])
		if err != nil {
			return nil, err
		}
		l.features = append(l.features, ft)
	}
	return l, nil
}

func decodeValue(b []byte) (mvtValue, error) {
	var out mvtValue
	for i := 0; i < len(b); {
		f, wt, v, n, err := field(b, i)
		if err != nil {
			return out, err
		}
		i = n
		switch f {
		case 1:
			if wt == 2 {
				out.s, out.isStr = string(b[v.start:v.end]), true
			}
		case 2:
			out.n = float64(math.Float32frombits(uint32(v.u)))
		case 3:
			out.n = math.Float64frombits(v.u)
		case 4:
			out.n = float64(int64(v.u))
		case 5:
			out.n = float64(v.u)
		case 6:
			out.n = float64(int64(v.u>>1) ^ -int64(v.u&1))
		case 7:
			out.n = float64(v.u)
		}
	}
	return out, nil
}

func decodeFeature(b []byte) (mvtFeature, error) {
	var ft mvtFeature
	for i := 0; i < len(b); {
		f, wt, v, n, err := field(b, i)
		if err != nil {
			return ft, err
		}
		i = n
		switch {
		case f == 2 && wt == 2:
			ft.tags = packed(b[v.start:v.end], ft.tags)
		case f == 3 && wt == 0:
			ft.typ = geomType(v.u)
		case f == 4 && wt == 2:
			ft.geom = packed(b[v.start:v.end], ft.geom)
		}
	}
	return ft, nil
}

func packed(b []byte, dst []uint32) []uint32 {
	for i := 0; i < len(b); {
		u, n := binary.Uvarint(b[i:])
		if n <= 0 {
			return dst
		}
		dst = append(dst, uint32(u))
		i += n
	}
	return dst
}

// prop returns the value of the property key of f, and whether it has one.
func (l *mvtLayer) prop(f *mvtFeature, key string) (mvtValue, bool) {
	for i := 0; i+1 < len(f.tags); i += 2 {
		k, v := int(f.tags[i]), int(f.tags[i+1])
		if k < len(l.keys) && v < len(l.values) && l.keys[k] == key {
			return l.values[v], true
		}
	}
	return mvtValue{}, false
}

func (l *mvtLayer) str(f *mvtFeature, key string) string {
	v, _ := l.prop(f, key)
	if v.isStr {
		return v.s
	}
	return ""
}

func (l *mvtLayer) num(f *mvtFeature, key string, def float64) float64 {
	if v, ok := l.prop(f, key); ok && !v.isStr {
		return v.n
	}
	return def
}

// path walks a feature's geometry: start is called at every MoveTo, line at
// every LineTo and close at every ClosePath, with positions in tile units.
func (f *mvtFeature) path(start, line func(x, y float64), close func()) {
	var x, y int32
	g := f.geom
	for i := 0; i < len(g); {
		cmd, count := g[i]&7, int(g[i]>>3)
		i++
		switch cmd {
		case 1, 2:
			for range count {
				if i+1 >= len(g) {
					return
				}
				x += zigzag(g[i])
				y += zigzag(g[i+1])
				i += 2
				if cmd == 1 {
					start(float64(x), float64(y))
				} else {
					line(float64(x), float64(y))
				}
			}
		case 7:
			close()
		default:
			return
		}
	}
}

func zigzag(u uint32) int32 { return int32(u>>1) ^ -int32(u&1) }
