package showcase

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/ui/maps"
)

func init() {
	register(Demo{Name: "map", Group: "media", Title: "Map",
		Summary: "A map after MapKit, from self hosted tiles, with photographs that cluster.",
		Width:   720, Height: 480, Settle: 60}, mapDemo)
}

// The map of the demo is made once and kept, as an application keeps its
// own: its camera and its tiles outlive a build.
var (
	demoMapOnce sync.Once
	demoMap     *maps.Map
	demoPhotos  *maps.Points
)

// demoMapProvider is the Oldenburg extract of the project plan, section 37,
// when it has been cut on this machine, and the debug tiles otherwise, so
// that the demo runs without data and without a network.
func demoMapProvider() (maps.Provider, *maps.Style) {
	if dir, err := os.UserCacheDir(); err == nil {
		if p, err := maps.PMTiles(filepath.Join(dir, "gift", "maps", "oldenburg.pmtiles")); err == nil {
			return p, maps.ProtomapsLight()
		}
	}
	return maps.DebugTiles(), nil
}

func mapDemo(ctx *gift.Context) gift.View {
	demoMapOnce.Do(func() {
		tiles, style := demoMapProvider()
		pts, _ := maps.LocatePhotos(context.Background(), mapPhotos)
		demoPhotos = maps.NewPoints(pts, maps.Clustering{})
		demoMap, _ = maps.New(maps.Config{
			Tiles: tiles, Style: style, DarkStyle: maps.ProtomapsDark(),
			Camera: maps.Camera{Center: geo.LatLon(53.142, 8.21), Zoom: 13},
		})
	})
	// snippet:begin
	return maps.View(demoMap).
		Points(demoPhotos, maps.PhotoMarkers()).
		Frame(680, 440).
		Label("Photographs around Oldenburg")
	// snippet:end
}
