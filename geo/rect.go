package geo

import "math"

// Rect is the area between two parallels and two meridians: Min is its south
// west corner and Max its north east corner. A rectangle that crosses the
// antimeridian is not representable; providers and weather models that cover
// the whole world use [World].
type Rect struct {
	Min, Max Coordinate
}

// World is the whole earth.
var World = Rect{Min: LatLon(-90, -180), Max: LatLon(90, 180)}

// RectOf returns the smallest rectangle that contains every coordinate, or the
// zero rectangle for none.
func RectOf(cs ...Coordinate) Rect {
	if len(cs) == 0 {
		return Rect{}
	}
	r := Rect{Min: cs[0], Max: cs[0]}
	for _, c := range cs[1:] {
		r = r.Extend(c)
	}
	return r
}

// Extend returns r grown to contain c.
func (r Rect) Extend(c Coordinate) Rect {
	r.Min.Lat = math.Min(r.Min.Lat, c.Lat)
	r.Min.Lon = math.Min(r.Min.Lon, c.Lon)
	r.Max.Lat = math.Max(r.Max.Lat, c.Lat)
	r.Max.Lon = math.Max(r.Max.Lon, c.Lon)
	return r
}

// IsEmpty reports whether r covers no area.
func (r Rect) IsEmpty() bool { return r.Max.Lat <= r.Min.Lat || r.Max.Lon <= r.Min.Lon }

// Contains reports whether c lies inside r or on its edge.
func (r Rect) Contains(c Coordinate) bool {
	return c.Lat >= r.Min.Lat && c.Lat <= r.Max.Lat && c.Lon >= r.Min.Lon && c.Lon <= r.Max.Lon
}

// Intersects reports whether r and o share area.
func (r Rect) Intersects(o Rect) bool {
	return r.Min.Lat < o.Max.Lat && o.Min.Lat < r.Max.Lat && r.Min.Lon < o.Max.Lon && o.Min.Lon < r.Max.Lon
}

// Intersect returns the area r and o share, or the zero rectangle.
func (r Rect) Intersect(o Rect) Rect {
	if !r.Intersects(o) {
		return Rect{}
	}
	return Rect{
		Min: LatLon(math.Max(r.Min.Lat, o.Min.Lat), math.Max(r.Min.Lon, o.Min.Lon)),
		Max: LatLon(math.Min(r.Max.Lat, o.Max.Lat), math.Min(r.Max.Lon, o.Max.Lon)),
	}
}

// Center returns the coordinate halfway between the corners.
func (r Rect) Center() Coordinate {
	return LatLon((r.Min.Lat+r.Max.Lat)/2, (r.Min.Lon+r.Max.Lon)/2)
}

// Projected returns the Web Mercator rectangle of r as its north west and
// south east points, the order in which Y grows.
func (r Rect) Projected() (nw, se Point) {
	return Project(LatLon(r.Max.Lat, r.Min.Lon)), Project(LatLon(r.Min.Lat, r.Max.Lon))
}

// Region is an area given by its centre and its extent in degrees, the way
// MapKit's MKCoordinateRegion gives one: what a map is asked to show when the
// application knows where something is and roughly how much around it.
type Region struct {
	Center Coordinate
	// LatSpan and LonSpan are the full height and width in degrees.
	LatSpan, LonSpan float64
}

// Rect returns the rectangle of the region.
func (g Region) Rect() Rect {
	return Rect{
		Min: LatLon(g.Center.Lat-g.LatSpan/2, g.Center.Lon-g.LonSpan/2),
		Max: LatLon(g.Center.Lat+g.LatSpan/2, g.Center.Lon+g.LonSpan/2),
	}
}

// RegionOf returns the region of r.
func RegionOf(r Rect) Region {
	return Region{Center: r.Center(), LatSpan: r.Max.Lat - r.Min.Lat, LonSpan: r.Max.Lon - r.Min.Lon}
}
