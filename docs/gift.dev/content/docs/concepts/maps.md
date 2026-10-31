---
title: Maps and Weather
weight: 17
---

The [map](../../components/media/map/) of package `ui/maps` draws OpenStreetMap from a provider you choose, and the
overlay of package `weather` draws the weather of a model over it. Both depend on data from outside, and both hide
where it comes from behind a seam, so that the source can change without the map noticing. This page is about
those sources: which to use, how to host them, and what they cost.

## Providers

A `maps.Provider` delivers the tiles of a map and says what they are: raster or vector, the schema of vector tiles,
the zoom levels, the area covered, the attribution, and whether tiles may be fetched before they are seen. The map
follows what it says.

| Provider                                   | Constructor          | For                                       |
|--------------------------------------------|----------------------|-------------------------------------------|
| A PMTiles archive in a file                | `maps.PMTiles`       | a kiosk that must work offline            |
| A PMTiles archive on your web server       | `maps.PMTilesURL`    | self hosting without a tile server        |
| A tile service with a URL template         | `maps.XYZ`           | a commercial service, or trying things out |
| Synthetic tiles that print their numbers   | `maps.DebugTiles`    | tests, and seeing the pyramid at work     |

### Self hosted: PMTiles

A [PMTiles](https://github.com/protomaps/PMTiles) archive is a whole map in one file, read with range requests,
without a tile server. Protomaps publishes a daily build of OpenStreetMap as one; a region is cut out of it with
their `pmtiles` tool, without downloading the 140 GB of the planet:

```bash
go run github.com/protomaps/go-pmtiles@v1.31.2 extract \
    https://build.protomaps.com/20261031.pmtiles oldenburg.pmtiles \
    --bbox=8.05,53.05,8.35,53.23
```

A city is a few ten megabytes. The builds are kept for a while only; the current dates are listed at
[docs.protomaps.com](https://docs.protomaps.com/basemaps/downloads). Copy the extract to the device and open it
with `maps.PMTiles`, or put it on any static server that answers range requests — nginx, Caddy, an S3 compatible
store — and open its URL with `maps.PMTilesURL`.

Its tiles are vector tiles of the Protomaps basemap, schema `protomaps-v4`, which `maps.ProtomapsLight` and
`maps.ProtomapsDark` draw. They are drawn on the processor, on the image pipeline's goroutines: the densest tile of
Oldenburg takes 11 ms on a Raspberry Pi 5. Labels are not drawn into the tiles but set on the map in every frame,
upright at any heading and out of each other's way.

### Tile services

`maps.XYZ` takes a URL template and the service's terms: its attribution, a header with an API key or the
`User-Agent` its usage policy wants, and whether it permits prefetching. Its raster tiles are fetched as
`asset.HTTP` sources, so the pipeline's disk cache keeps them by the service's caching headers.

The tile servers of the OpenStreetMap Foundation, `tile.openstreetmap.org`, are for development and trying out,
not for a product: their [usage policy](https://operations.osmfoundation.org/policies/tiles/) wants a `User-Agent`
that names the application, caching, no prefetching and no bulk downloads, and gives no guarantee of service.
gift sends `asset.DefaultUserAgent` unless a source sets its own; Go's own is blocked.

### Styles and schemas

Vector providers share OpenStreetMap as their source, but not the names of their layers. A `maps.Style` is written
for one schema, and `maps.New` refuses a style for another: switching between raster services costs nothing,
switching to a vector service of another schema costs a style.

## Weather

A `weather.Provider` delivers the fields of a weather model, normalised: cloud cover of the low, middle and high
layer as a fraction, precipitation in mm/h, the wind near the ground and aloft in m/s, the temperature. A
`weather.Sky` keeps the latest run of one over a region, on a goroutine of its own, and `maps.Weather` draws it:

```go
icon := dwd.ICOND2(dwd.Options{CacheDir: cache + "/dwd", UserAgent: "Kiosk/1.4 (+https://example.org)"})
global := gfs.Global(gfs.Options{CacheDir: cache + "/gfs", UserAgent: "Kiosk/1.4 (+https://example.org)"})
sky := weather.NewSky(weather.Combine(icon, global), weather.SkyConfig{Region: region})
sky.Start()

maps.View(m).Overlay(maps.Weather(sky, maps.WeatherOptions{Quality: maps.WeatherReduced}))
```

| Provider        | Model    | Coverage                       | Spacing | Runs, horizon       | Use                         |
|-----------------|----------|--------------------------------|---------|---------------------|-----------------------------|
| `dwd.ICOND2`    | ICON-D2  | Germany and its neighbours     | 2.2 km  | every 3 h, 48 h     | free, with the source named |
| `gfs.Global`    | GFS      | the earth                      | 28 km   | every 6 h, 120 h    | public domain               |

`weather.Combine` takes the first where it covers the region and the second beyond it, blended at the edge. The
Deutscher Wetterdienst serves every field of every hour as a file of the whole domain, 0.7 MB compressed, so a sky
fetches three hours ahead by default and the provider keeps the files in its cache directory; the GFS comes cut to
the region through NOAA's filter, a few kilobytes an hour.

### What is drawn

The clouds of each layer are drawn into textures on a goroutine when the run, the zoom or the region change, and
in a frame they are pictures: moved by the wind aloft, enlarged about the middle of the map by the parallax of
their height, so that high clouds slide faster than the ground when the map is panned, and cast as shadows away
from the sun of the hour and the place. The wind near the ground and the falling rain — snow where it freezes —
are particles, drawn as triangles. The precipitation is also drawn in the colours of a radar picture.
`WeatherOptions.Layers` chooses among them, and `WeatherOptions.Time` shows another hour of the forecast.

## On a Raspberry Pi

The [kiosk](../kiosks/) settings matter for a map: draw on demand and straight to the screen, run full screen,
keep the texture upload budget at one or two tiles a frame, and give the image pipeline two workers, so that the
frame loop keeps a core of its own while tiles are drawn. `cmd/example-maps` uses them. With them a Raspberry Pi 5
pans and zooms the vector map of a city at 1920 by 1080 with a frame interval p95 of 17.4 ms, and the map of a
camera roll at zoom 13 just as well; an overcast deck of weather over it costs a few milliseconds more.
