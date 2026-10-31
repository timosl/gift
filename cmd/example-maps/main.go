// Command example-maps shows a map of ui/maps: a self hosted PMTiles archive
// of OpenStreetMap, or the debug tiles, in a window of its own.
//
//	go run ./cmd/example-maps                       # the Oldenburg extract, see PLAN.md section 37
//	go run ./cmd/example-maps -pmtiles city.pmtiles
//	go run ./cmd/example-maps -url https://example.org/city.pmtiles
//	go run ./cmd/example-maps -debug -heading 30
//	go run ./cmd/example-maps -fullscreen            # a kiosk
//
// Pan with a finger or the mouse, zoom with two fingers, the wheel or plus
// and minus, turn with two fingers; a double tap zooms in. -dark draws the
// dark style.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"

	"github.com/worldiety/gift/asset"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/maps"
)

func main() {
	def := ""
	if dir, err := os.UserCacheDir(); err == nil {
		def = filepath.Join(dir, "gift", "maps", "oldenburg.pmtiles")
	}
	path := flag.String("pmtiles", def, "a PMTiles archive")
	url := flag.String("url", "", "a PMTiles archive on an HTTP server, read with range requests")
	debug := flag.Bool("debug", false, "draw the debug tiles instead of a map")
	dark := flag.Bool("dark", false, "the dark style")
	width := flag.Int("width", 1180, "window width")
	height := flag.Int("height", 800, "window height")
	lat := flag.Float64("lat", 53.1435, "latitude of the centre")
	lon := flag.Float64("lon", 8.2146, "longitude of the centre")
	zoom := flag.Float64("zoom", 13, "zoom")
	heading := flag.Float64("heading", 0, "heading in degrees, clockwise from north")
	noLabels := flag.Bool("nolabels", false, "draw the map without its labels, for measuring")
	fullscreen := flag.Bool("fullscreen", false, "start full screen, as a kiosk does")
	upload := flag.Int("upload", 512, "texture upload budget per frame in KiB: on a Raspberry Pi 5 two tiles per frame keep a pan at 60 Hz")
	flag.Parse()
	if *fullscreen {
		eb.SetFullscreen(true)
	}
	cam := maps.Camera{Center: geo.LatLon(*lat, *lon), Zoom: *zoom, Heading: *heading}
	if err := run(*path, *url, *debug, *dark, *noLabels, *width, *height, *upload, cam); err != nil {
		fmt.Fprintln(os.Stderr, "example-maps:", err)
		os.Exit(1)
	}
}

func run(path, url string, debug, dark, noLabels bool, width, height, upload int, cam maps.Camera) error {
	if err := example.LoadFont(); err != nil {
		return err
	}
	var tiles maps.Provider
	var err error
	switch {
	case debug:
		tiles = maps.DebugTiles()
	case url != "":
		tiles, err = maps.PMTilesURL(url)
	default:
		tiles, err = maps.PMTiles(path)
	}
	if err != nil {
		return err
	}
	light, darkStyle := maps.ProtomapsLight(), maps.ProtomapsDark()
	if noLabels {
		light.Labels, darkStyle.Labels = nil, nil
	}
	m, err := maps.New(maps.Config{
		Tiles: tiles, Style: light, DarkStyle: darkStyle,
		Camera: cam, Rotation: maps.RotationOn,
	})
	if err != nil {
		return err
	}
	app := gift.New(gift.Options{Root: func(*gift.Context) gift.View {
		return ui.Window(maps.View(m).Flex(1).Key("map"))
	}})
	if dark {
		ui.SetTheme(app, ui.DarkTheme())
	}
	pipe := asset.NewPipeline(asset.Config{Deliver: app.Post})
	defer func() {
		pipe.Close()
		app.DrainPosts()
	}()
	ui.SetImagePipeline(pipe)
	cfg := backend.Config{
		Title: "gift maps", Width: width, Height: height,
		// The settings of a kiosk on a Raspberry Pi: draw only what changed,
		// straight into the window, and tick slowly while nothing does.
		DrawOnDemand: true, DirectToScreen: true, IdleTPS: 10,
		AssetStats: ui.ImagePipelineStats,
	}
	if upload > 0 {
		cfg.Textures.UploadBytesPerFrame = int64(upload) << 10
	}
	return backend.Run(app, cfg)
}
