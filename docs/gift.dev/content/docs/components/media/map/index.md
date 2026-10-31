---
title: Map
---

A map after MapKit: tiles of OpenStreetMap from a provider you choose, a camera that pans, glides, zooms about the
fingers and turns, labels that stay upright at any heading, and points — places of interest, or photographs that
merge into clusters as you zoom out and come apart as you zoom in, the way the map of the iOS Photos app does.
Package [`ui/maps`](https://github.com/worldiety/gift/tree/main/ui/maps), with its coordinates in package
[`geo`](https://github.com/worldiety/gift/tree/main/geo).

{{< gallery-shot alt="A map of Oldenburg with clusters of photographs" >}}

{{< gallery-code >}}

## Usage

A `maps.Map` is long lived and owned by the application, like a `ui.Gallery`; `maps.View` declares it in a build.
The map needs a [provider](../../../concepts/maps/#providers) of its tiles and, for vector tiles, a style for the
provider's schema:

```go
tiles, err := maps.PMTiles(filepath.Join(cacheDir, "gift/maps/oldenburg.pmtiles"))
m, err := maps.New(maps.Config{
	Tiles:     tiles,
	Style:     maps.ProtomapsLight(),
	DarkStyle: maps.ProtomapsDark(), // follows the theme
	Camera:    maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 13},
})

maps.View(m).Flex(1)
```

The tiles go through the [image pipeline](../../../concepts/images/), so a program that shows a map sets one up,
as for any picture. Its ladder should hold 256 times the display's density, 256 and 512 for most screens.

## The camera

`Camera` is a centre, a zoom and a heading. `m.SetCamera(c, true)` flies there on the path of van Wijk and Nuij —
out, across and in again — as MapKit and Mapbox do; `m.ShowRect(r, true)` fits a rectangle of coordinates.
`m.Camera()` reads it, and `m.OnCameraChange` hears of it when it comes to rest after a gesture or a flight.

| Input                    | What it does                                                         |
|--------------------------|----------------------------------------------------------------------|
| One finger, the mouse    | pans; flicked, it glides on                                          |
| Two fingers              | zoom about their centre, rubber banding past the limits; turn the map when rotation is on |
| Double tap               | zooms in a level about the tap                                       |
| Mouse wheel              | zooms half a level per notch about the cursor                        |
| Trackpad                 | pans; with control or command held, zooms                            |
| Arrow keys, plus, minus  | pan, zoom about the middle                                           |

A turned map shows a compass in its top right corner; a tap on it turns the map back to north. Rotation is on by
default for vector tiles, whose labels the map sets upright itself, and off for raster tiles, whose lettering
would turn with them; `Config.Rotation` decides otherwise.

## Points and clusters

`maps.NewPoints` takes a slice of points and builds their cluster index on a goroutine of its own; a hundred
thousand photographs take about a quarter of a second on a Raspberry Pi 5. A frame asks the index only for what
touches the map, so what is drawn does not grow with the catalogue.

```go
pts, err := maps.LocatePhotos(ctx, sources) // where and when, from EXIF
photos := maps.NewPoints(pts, maps.Clustering{Radius: 80})

maps.View(m).
	Points(photos, maps.PhotoMarkers()).
	OnSelect(func(p maps.Point) { open(p.ID) }).
	OnCluster(func(c maps.Cluster) { showGrid(c.Points()) })
```

A cluster shows the photograph with the highest `Score`, then the newest, and its count on a badge. When the zoom
crosses a level the markers come apart from the cluster that held them, or fly into it. Without `OnCluster` a tap
on a cluster zooms in until it comes apart. `maps.PinMarkers(colour)` draws places as pins with their titles.

## Nothing outside

Tiles, labels and markers are chosen by what touches the map — a turned map's polygon, not its bounding box — and
nothing else is drawn, requested or uploaded; a marker whose picture reaches in from outside is drawn, one just
outside is not. `m.Stats()` counts what the last frame drew.

## Overlays and weather

`MapView.Overlay` adds something drawn above the tiles and below the labels: the [weather](../../../concepts/maps/#weather)
of package `weather`, or an overlay of your own, which gets the camera of every frame as a `maps.Projection`.

## Attribution

The provider's attribution is always on the map, in the bottom right corner, and cannot be switched off: the
licence of OpenStreetMap, and of every map derived from it, requires it.

## API

- [`maps.New`](https://pkg.go.dev/github.com/worldiety/gift/ui/maps#New)
- [`maps.View`](https://pkg.go.dev/github.com/worldiety/gift/ui/maps#View)
- [`maps.Points`](https://pkg.go.dev/github.com/worldiety/gift/ui/maps#Points)
- [`maps.Provider`](https://pkg.go.dev/github.com/worldiety/gift/ui/maps#Provider)
- [`maps.Style`](https://pkg.go.dev/github.com/worldiety/gift/ui/maps#Style)
- [`geo`](https://pkg.go.dev/github.com/worldiety/gift/geo)
