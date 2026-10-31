// Package geo is positions on the earth and the Web Mercator projection
// gift's maps draw them in.
//
// It has no user interface and no dependency beyond the standard library:
// package ui/maps builds on it as package ui builds on geom, and so do the
// weather providers, which never draw anything. See the project plan,
// section 37.
//
// # Two kinds of position
//
// A [Coordinate] is latitude and longitude in degrees, the form a photograph's
// EXIF, a weather model and a person all use. A [Point] is the same position
// projected into Web Mercator (EPSG:3857) and normalised to the unit square,
// the form every map tile is cut from: X grows east from the antimeridian, Y
// grows south from the northern edge of the projection, and one tile of zoom z
// is 1/2^z wide.
//
// Points are float64 on purpose. At zoom 20 the world is 2^28 pixels wide,
// and float32 has 24 bits of mantissa; a map that computed in float32 would
// have its markers jitter by whole pixels when zoomed in. Conversion to the
// float32 of the display list happens only relative to the camera, after the
// large common part has cancelled; see ui/maps.
package geo

import (
	"fmt"
	"math"
)

// MaxLatitude is the northern edge of Web Mercator, the latitude at which the
// projection is square: atan(sinh(π)) in degrees. Coordinates further north
// or south are clamped to it when projected.
const MaxLatitude = 85.05112877980659

// EarthRadius is the mean radius of the earth in metres, the radius
// [Distance] uses.
const EarthRadius = 6371008.8

// Coordinate is a position on the earth in degrees of latitude, positive
// north, and longitude, positive east.
type Coordinate struct {
	Lat, Lon float64
}

// LatLon returns the coordinate at lat and lon degrees.
func LatLon(lat, lon float64) Coordinate { return Coordinate{Lat: lat, Lon: lon} }

// Valid reports whether c is a position on the earth: both numbers finite,
// the latitude within ±90 and the longitude within ±180.
func (c Coordinate) Valid() bool {
	return !math.IsNaN(c.Lat) && !math.IsNaN(c.Lon) &&
		c.Lat >= -90 && c.Lat <= 90 && c.Lon >= -180 && c.Lon <= 180
}

// String formats c as "53.14350, 8.21460", latitude first, as maps and
// photographs show it.
func (c Coordinate) String() string { return fmt.Sprintf("%.5f, %.5f", c.Lat, c.Lon) }

// Point is a position in Web Mercator, normalised to the unit square: X from 0
// at 180° west to 1 at 180° east, Y from 0 at [MaxLatitude] north to 1 at
// the same latitude south. X outside [0, 1) is the same place in a
// neighbouring copy of the world, which is how a map that is panned across
// the antimeridian keeps its arithmetic continuous.
type Point struct {
	X, Y float64
}

// Project returns the Web Mercator point of c. Latitudes beyond
// [MaxLatitude] are clamped to it.
func Project(c Coordinate) Point {
	lat := math.Max(-MaxLatitude, math.Min(MaxLatitude, c.Lat)) * math.Pi / 180
	return Point{
		X: (c.Lon + 180) / 360,
		Y: (1 - math.Log(math.Tan(lat)+1/math.Cos(lat))/math.Pi) / 2,
	}
}

// Coordinate returns the coordinate of p, the inverse of [Project]. The
// longitude is wrapped into ±180, so a point in a neighbouring copy of the
// world names the same place as its original.
func (p Point) Coordinate() Coordinate {
	lon := p.Wrapped().X*360 - 180
	n := math.Pi - 2*math.Pi*p.Y
	lat := 180 / math.Pi * math.Atan(math.Sinh(n))
	return Coordinate{Lat: lat, Lon: lon}
}

// Wrapped returns p with X brought into [0, 1).
func (p Point) Wrapped() Point {
	x := p.X - math.Floor(p.X)
	return Point{X: x, Y: p.Y}
}

// Add returns p moved by q.
func (p Point) Add(q Point) Point { return Point{X: p.X + q.X, Y: p.Y + q.Y} }

// Sub returns the vector from q to p.
func (p Point) Sub(q Point) Point { return Point{X: p.X - q.X, Y: p.Y - q.Y} }

// Scale returns p multiplied by f.
func (p Point) Scale(f float64) Point { return Point{X: p.X * f, Y: p.Y * f} }

// Distance returns the great circle distance between a and b in metres, by
// the haversine formula on a sphere of [EarthRadius]. It is off from the
// ellipsoid by up to half a percent, which is far below anything a map shows.
func Distance(a, b Coordinate) float64 {
	const rad = math.Pi / 180
	dlat := (b.Lat - a.Lat) * rad
	dlon := (b.Lon - a.Lon) * rad
	s := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * EarthRadius * math.Asin(math.Min(1, math.Sqrt(s)))
}

// MetresPerUnit is the length on the ground of one unit of Web Mercator at
// latitude lat: the circumference of the parallel. A map multiplies it by its
// scale to say how many metres a point covers.
func MetresPerUnit(lat float64) float64 {
	return 2 * math.Pi * EarthRadius * math.Cos(lat*math.Pi/180)
}
