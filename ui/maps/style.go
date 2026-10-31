package maps

import (
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/worldiety/gift/ui"
)

// Style draws the vector tiles of one schema: which features of which layer
// are filled or stroked in what colour and width, and which are labelled.
//
// A style is written for one schema, because vector providers share
// OpenStreetMap as their source but not the names of their layers and kinds;
// see [ProviderInfo.Schema] and the project plan, section 37. It is
// immutable once a map uses it: the tiles are drawn on the image pipeline's
// goroutines.
type Style struct {
	// Schema is the layer schema the style is written for.
	Schema string
	// Name distinguishes the styles of one schema in cache keys, so that a
	// tile drawn light is not shown dark. Change it with the style.
	Name string
	// Background is the colour where no layer draws. In the Protomaps schema
	// it is the sea, and the land is the "earth" layer above it.
	Background ui.Color
	// Placeholder is the colour of the map before its tiles arrive and
	// outside the provider's coverage: the land, in a map of land.
	Placeholder ui.Color
	// Layers are drawn in order, each over the ones before.
	Layers []Layer
	// Labels are the rules of the labels, which are not drawn into the tiles
	// but set on the map, upright at any heading.
	Labels []LabelRule

	once sync.Once
	reg  uint64
	srcs map[string]bool
}

// Layer is one drawing rule: the features of one source layer, chosen by
// their kind, filled, stroked, or both.
type Layer struct {
	// Source is the layer of the tile, "roads" say.
	Source string
	// Kinds and Details choose features by their "kind" and "kind_detail"
	// properties; an empty list chooses all.
	Kinds, Details []string
	// MinZoom and MaxZoom are the tile levels the rule applies at, MaxZoom
	// exclusive and zero for no limit.
	MinZoom, MaxZoom float64
	// Fill fills polygons; transparent fills nothing.
	Fill ui.Color
	// Line strokes lines and, when Fill is transparent, the outlines of
	// polygons, Width wide; transparent strokes nothing.
	Line  ui.Color
	Width Stops
}

// LabelRule chooses features to label with their "name".
type LabelRule struct {
	Source           string
	Kinds            []string
	MinZoom, MaxZoom float64
	// Size is the font size by zoom.
	Size Stops
	// Color and Halo are the glyphs and the outline that keeps them legible.
	Color, Halo ui.Color
	// Rank orders the rules: a lower rank is placed first and wins a
	// collision. Within a rule, a bigger place wins.
	Rank int
	// Along places a line's label along it, turned with it, kept upright.
	Along bool
}

// Stop is a value at a zoom.
type Stop struct{ Zoom, Value float64 }

// Stops is a value that changes with the zoom: linear between the stops,
// constant before the first and after the last.
type Stops []Stop

// At returns the value at zoom z.
func (s Stops) At(z float64) float64 {
	if len(s) == 0 {
		return 0
	}
	if z <= s[0].Zoom {
		return s[0].Value
	}
	for i := 1; i < len(s); i++ {
		if z <= s[i].Zoom {
			a, b := s[i-1], s[i]
			return a.Value + (b.Value-a.Value)*(z-a.Zoom)/(b.Zoom-a.Zoom)
		}
	}
	return s[len(s)-1].Value
}

func (s *Style) background() ui.Color { return s.Placeholder }

// id names the style in cache keys.
func (s *Style) id() string { return s.Schema + ":" + s.Name }

var (
	styleIDs  atomic.Uint64
	styleRegs sync.Map // uint64 → *Style
)

// register gives the style the number the vector tiles of the pipeline name
// it by, and notes the source layers it reads.
func (s *Style) register() uint64 {
	s.once.Do(func() {
		s.reg = styleIDs.Add(1)
		s.srcs = make(map[string]bool)
		for _, l := range s.Layers {
			s.srcs[l.Source] = true
		}
		styleRegs.Store(s.reg, s)
	})
	return s.reg
}

func styleByID(id uint64) (*Style, bool) {
	v, ok := styleRegs.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*Style), true
}

func (l *Layer) applies(z float64) bool {
	return z >= l.MinZoom && (l.MaxZoom == 0 || z < l.MaxZoom)
}

func (r *LabelRule) applies(z float64) bool {
	return z >= r.MinZoom && (r.MaxZoom == 0 || z < r.MaxZoom)
}

// matches reports whether a feature with kind and detail is chosen by the
// lists; an empty list chooses all.
func matches(kinds, details []string, kind, detail string) bool {
	return (len(kinds) == 0 || slices.Contains(kinds, kind)) &&
		(len(details) == 0 || slices.Contains(details, detail))
}

// palette is the colours of a Protomaps style.
type palette struct {
	sea, land, park, forest, farm, urban, industry, school, hospital ui.Color
	building, buildingLine                                           ui.Color
	highwayCase, highway, majorCase, major, minorCase, minor         ui.Color
	path, rail, boundary                                             ui.Color
	label, labelMinor, halo, water                                   ui.Color
}

// ProtomapsLight is a light style for the Protomaps basemap, schema version
// 4, after the colours of Apple Maps: pale land, blue water, green parks,
// white streets and yellow motorways.
func ProtomapsLight() *Style {
	return protomaps("light-1", palette{
		sea: ui.RGB(166, 205, 237), land: ui.RGB(245, 243, 238),
		park: ui.RGB(206, 232, 196), forest: ui.RGB(191, 223, 180), farm: ui.RGB(238, 241, 226),
		urban: ui.RGB(240, 237, 233), industry: ui.RGB(234, 230, 229),
		school: ui.RGB(248, 240, 218), hospital: ui.RGB(250, 228, 228),
		building: ui.RGB(226, 222, 215), buildingLine: ui.RGB(212, 206, 198),
		highwayCase: ui.RGB(226, 160, 72), highway: ui.RGB(255, 207, 120),
		majorCase: ui.RGB(212, 205, 196), major: ui.RGB(255, 255, 255),
		minorCase: ui.RGB(222, 216, 208), minor: ui.RGB(255, 255, 255),
		path: ui.RGB(208, 202, 194), rail: ui.RGB(186, 182, 178), boundary: ui.RGB(160, 150, 178),
		label: ui.RGB(36, 40, 48), labelMinor: ui.RGB(104, 106, 116), halo: ui.RGBA(255, 255, 255, 230),
		water: ui.RGB(46, 106, 166),
	})
}

// ProtomapsDark is the dark counterpart of [ProtomapsLight].
func ProtomapsDark() *Style {
	return protomaps("dark-1", palette{
		sea: ui.RGB(31, 50, 72), land: ui.RGB(38, 40, 44),
		park: ui.RGB(36, 55, 43), forest: ui.RGB(33, 58, 41), farm: ui.RGB(41, 45, 42),
		urban: ui.RGB(44, 45, 49), industry: ui.RGB(47, 46, 50),
		school: ui.RGB(52, 49, 42), hospital: ui.RGB(56, 42, 44),
		building: ui.RGB(56, 58, 64), buildingLine: ui.RGB(68, 70, 77),
		highwayCase: ui.RGB(120, 88, 40), highway: ui.RGB(176, 128, 56),
		majorCase: ui.RGB(30, 32, 36), major: ui.RGB(88, 92, 101),
		minorCase: ui.RGB(32, 34, 38), minor: ui.RGB(72, 76, 84),
		path: ui.RGB(74, 78, 86), rail: ui.RGB(86, 86, 92), boundary: ui.RGB(118, 108, 140),
		label: ui.RGB(232, 234, 238), labelMinor: ui.RGB(170, 172, 180), halo: ui.RGBA(18, 20, 24, 220),
		water: ui.RGB(128, 172, 214),
	})
}

func protomaps(name string, p palette) *Style {
	none := ui.RGBA(0, 0, 0, 0)
	fill := func(src string, c ui.Color, minZ float64, kinds ...string) Layer {
		return Layer{Source: src, Kinds: kinds, Fill: c, MinZoom: minZ, Line: none}
	}
	line := func(src string, c ui.Color, w Stops, minZ float64, kinds []string, details ...string) Layer {
		return Layer{Source: src, Kinds: kinds, Details: details, Line: c, Width: w, MinZoom: minZ, Fill: none}
	}
	// Widths are logical pixels at the tile's level.
	var (
		highwayCase = Stops{{5, 1.1}, {10, 2.4}, {14, 5.5}, {18, 18}}
		highway     = Stops{{5, 0.7}, {10, 1.6}, {14, 4.2}, {18, 15}}
		majorCase   = Stops{{8, 0.8}, {12, 2.6}, {15, 6.5}, {18, 17}}
		major       = Stops{{8, 0.5}, {12, 1.7}, {15, 5}, {18, 14}}
		minorCase   = Stops{{12, 0.8}, {14, 2.2}, {16, 5.5}, {18, 12}}
		minor       = Stops{{12, 0.5}, {14, 1.5}, {16, 4.2}, {18, 10}}
		path        = Stops{{14, 0.6}, {16, 1.2}, {18, 2.6}}
		rail        = Stops{{10, 0.6}, {14, 1.2}, {18, 2.5}}
		river       = Stops{{9, 0.5}, {12, 1.2}, {15, 3}, {18, 7}}
		boundary    = Stops{{2, 0.6}, {8, 1}, {12, 1.5}}
	)
	roads := func(k ...string) []string { return k }
	return &Style{
		Schema:      "protomaps-v4",
		Name:        name,
		Background:  p.sea,
		Placeholder: p.land,
		Layers: []Layer{
			fill("earth", p.land, 0),
			fill("landcover", p.forest, 0, "forest"),
			fill("landcover", p.farm, 0, "farmland", "grassland"),
			fill("landcover", p.urban, 0, "urban_area"),
			fill("landuse", p.farm, 10, "farmland", "meadow", "grassland", "allotments", "orchard", "vineyard"),
			fill("landuse", p.urban, 12, "residential", "commercial", "retail", "pedestrian", "railway"),
			fill("landuse", p.industry, 12, "industrial", "military"),
			fill("landuse", p.park, 10, "park", "garden", "grass", "nature_reserve", "national_park",
				"recreation_ground", "playground", "pitch", "golf_course", "cemetery", "zoo", "village_green"),
			fill("landuse", p.forest, 8, "forest", "wood", "scrub", "wetland"),
			fill("landuse", p.school, 13, "school", "kindergarten", "university", "college"),
			fill("landuse", p.hospital, 13, "hospital"),
			fill("water", p.sea, 0),
			line("water", p.sea, river, 9, roads("river", "canal", "stream", "ditch", "drain")),
			line("boundaries", p.boundary, boundary, 0, roads("country", "region")),
			fill("buildings", p.building, 13, "building", "building_part"),
			{Source: "buildings", Kinds: []string{"building"}, Line: p.buildingLine, Fill: none,
				Width: Stops{{15, 0.5}, {18, 1}}, MinZoom: 15},
			line("roads", p.rail, rail, 10, roads("rail")),
			line("roads", p.path, path, 14, roads("path")),
			line("roads", p.minorCase, minorCase, 12, roads("minor_road", "other")),
			line("roads", p.majorCase, majorCase, 8, roads("major_road")),
			line("roads", p.highwayCase, highwayCase, 5, roads("highway")),
			line("roads", p.minor, minor, 12, roads("minor_road", "other")),
			line("roads", p.major, major, 8, roads("major_road")),
			line("roads", p.highway, highway, 5, roads("highway")),
		},
		Labels: []LabelRule{
			{Source: "places", Kinds: []string{"country"}, Size: Stops{{2, 12}, {6, 16}},
				Color: p.label, Halo: p.halo, Rank: 0},
			{Source: "places", Kinds: []string{"region"}, Size: Stops{{4, 11}, {8, 14}},
				Color: p.labelMinor, Halo: p.halo, Rank: 1, MaxZoom: 10},
			{Source: "places", Kinds: []string{"locality"}, Size: Stops{{6, 11}, {10, 15}, {14, 19}},
				Color: p.label, Halo: p.halo, Rank: 2},
			{Source: "places", Kinds: []string{"macrohood", "neighbourhood"}, Size: Stops{{12, 11}, {16, 14}},
				Color: p.labelMinor, Halo: p.halo, Rank: 4, MinZoom: 12},
			{Source: "water", Size: Stops{{10, 11}, {16, 14}}, Color: p.water, Halo: p.halo, Rank: 5, MinZoom: 11},
			{Source: "roads", Kinds: []string{"highway", "major_road"}, Size: Stops{{13, 10.5}, {17, 13}},
				Color: p.label, Halo: p.halo, Rank: 3, MinZoom: 13, Along: true},
			{Source: "roads", Kinds: []string{"minor_road", "other"}, Size: Stops{{15, 10}, {18, 12.5}},
				Color: p.labelMinor, Halo: p.halo, Rank: 6, MinZoom: 15, Along: true},
			{Source: "pois", Size: Stops{{15, 10}, {18, 12}}, Color: p.labelMinor, Halo: p.halo, Rank: 7, MinZoom: 16},
		},
	}
}

// String names the style.
func (s *Style) String() string { return s.id() + "#" + strconv.FormatUint(s.reg, 10) }
