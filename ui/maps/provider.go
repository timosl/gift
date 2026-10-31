package maps

import (
	"context"
	"fmt"

	"github.com/worldiety/gift/geo"
)

// TileSize is the edge of a map tile in logical pixels at a whole zoom: at
// zoom z the world is TileSize·2^z points wide. Providers whose tiles have
// another pixel size are drawn at this size all the same; the pixel size only
// says how sharp they are.
const TileSize = 256

// TileID names a tile of the XYZ scheme: zoom level Z, column X from the
// antimeridian eastward and row Y from the north.
type TileID struct {
	Z, X, Y int
}

// String formats t as "z/x/y", the form of tile URLs.
func (t TileID) String() string { return fmt.Sprintf("%d/%d/%d", t.Z, t.X, t.Y) }

// Parent returns the tile one level up that contains t.
func (t TileID) Parent() TileID { return TileID{Z: t.Z - 1, X: t.X >> 1, Y: t.Y >> 1} }

// Ancestor returns the tile at level z that contains t; z must not be below
// zero or above t.Z.
func (t TileID) Ancestor(z int) TileID {
	d := t.Z - z
	return TileID{Z: z, X: t.X >> d, Y: t.Y >> d}
}

// Valid reports whether t exists: a level of zero or more and a column and row
// inside the 2^Z by 2^Z grid.
func (t TileID) Valid() bool {
	n := 1 << t.Z
	return t.Z >= 0 && t.Z < 31 && t.X >= 0 && t.X < n && t.Y >= 0 && t.Y < n
}

// TileKind says what a provider's tiles contain.
type TileKind uint8

const (
	// Raster tiles are pictures, drawn as they come.
	Raster TileKind = iota
	// Vector tiles are geometry, drawn by gift with a [Style].
	Vector
)

func (k TileKind) String() string {
	if k == Vector {
		return "vector"
	}
	return "raster"
}

// TileFormat is the encoding of a tile's bytes.
type TileFormat uint8

const (
	FormatUnknown TileFormat = iota
	FormatPNG
	FormatJPEG
	FormatWebP
	// FormatMVT is a Mapbox Vector Tile, a protocol buffer, unpacked.
	FormatMVT
	// FormatPixels is premultiplied RGBA in gift's own container, for a
	// provider that draws its tiles itself; see [EncodePixels].
	FormatPixels
)

// ProviderInfo is everything that makes a map provider what it is, beyond
// the tiles themselves: the map follows it instead of assumptions. It is the
// first half of the provider seam of the project plan, section 37.
type ProviderInfo struct {
	// Name identifies the provider in cache keys: two providers whose tiles
	// differ must have different names. A file provider uses its path, a
	// URL provider its template.
	Name string
	// Kind says whether the tiles are pictures or geometry.
	Kind TileKind
	// Schema is the layer schema of vector tiles, "protomaps-v4" or
	// "openmaptiles" say. A [Style] is written for one schema, and the map
	// refuses a style for another; see [New].
	Schema string
	// MinZoom and MaxZoom are the levels the provider has. Above MaxZoom the
	// map enlarges the last level: a raster tile is drawn bigger, a vector
	// tile is drawn again from its geometry, sharp at any zoom.
	MinZoom, MaxZoom int
	// Bounds is the area the provider covers; nothing outside it is asked
	// for. The zero value means the whole world.
	Bounds geo.Rect
	// Attribution is the notice the map must show, such as
	// "© OpenStreetMap contributors". It is drawn on the map and cannot be
	// switched off: the licences of every OSM derived source require it.
	Attribution string
	// Prefetch says whether the provider permits tiles to be fetched before
	// they are seen. The OpenStreetMap Foundation's servers forbid it.
	Prefetch bool
	// Revision changes when the tiles change, an archive's modification time
	// or a release number. It is part of every cache key, so a new revision
	// is never mixed up with an old one.
	Revision string
}

// Tile is one tile as a provider delivers it.
type Tile struct {
	Data   []byte
	Format TileFormat
}

// Provider delivers the tiles of one map. It is the provider seam of the
// project plan, section 37: a self hosted PMTiles archive, a raster service
// with a URL template and a synthetic source for tests all implement it, and
// switching between them is a change of configuration.
//
// Tile is called from the image pipeline's goroutines, several at once, and
// must be safe for that. It returns [ErrNoTile] for a tile that does not exist
// — outside the coverage, or open sea an archive left out — and any other
// error for a failure worth trying again later.
type Provider interface {
	Info() ProviderInfo
	Tile(ctx context.Context, t TileID) (Tile, error)
}

// ErrNoTile is the error of a tile that does not exist. It is permanent: the
// map draws the coarser tile that covers the place instead and does not ask
// again.
var ErrNoTile error = noTile{}

type noTile struct{}

func (noTile) Error() string   { return "maps: no such tile" }
func (noTile) Temporary() bool { return false }
