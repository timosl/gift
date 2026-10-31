package maps

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geo"
)

// XYZOptions describes a tile service with a URL template.
type XYZOptions struct {
	// Name identifies the provider in cache keys; empty takes the template.
	Name string
	// Kind is Raster for pictures, the default, or Vector for Mapbox Vector
	// Tiles, which then need Schema and a style for it.
	Kind   TileKind
	Schema string
	// MinZoom and MaxZoom are the levels the service has; zero MaxZoom
	// selects 19, the last level of the OpenStreetMap Foundation's.
	MinZoom, MaxZoom int
	// Attribution is the notice the service requires. It is shown on the
	// map and is not optional: an empty one is refused.
	Attribution string
	// Prefetch permits fetching the tiles around the view before they are
	// seen. Leave it false for the OSMF's servers, whose policy forbids it.
	Prefetch bool
	// Subdomains are put in for {s} in turn, as some services spread their
	// load.
	Subdomains []string
	// Header is sent with every request: an API key, say. A User-Agent in it
	// replaces [asset.DefaultUserAgent], which a service that publishes a
	// usage policy wants to name the application.
	Header http.Header
	// Revision is part of every cache key; change it to drop the tiles
	// cached of an older release of the service.
	Revision string
	// Client is the HTTP client of vector tiles; raster tiles are fetched
	// by the image pipeline.
	Client *http.Client
}

// XYZ returns a provider for a tile service with a URL template, the most
// common form there is: {z}, {x} and {y} are replaced by the tile, {s} by a
// subdomain.
//
//	maps.XYZ("https://tile.openstreetmap.org/{z}/{x}/{y}.png", maps.XYZOptions{
//	    Attribution: "© OpenStreetMap",
//	    Header:      http.Header{"User-Agent": {"Kiosk/1.4 (+https://example.org)"}},
//	})
//
// Raster tiles are fetched by the image pipeline as [asset.HTTP] sources, so
// they are cached on disk by the HTTP caching headers of the service and
// revalidated with its ETags, which is what the usage policy of the
// OpenStreetMap Foundation asks for. The OSMF's servers are for developing
// and trying out; see the project plan, section 37, for what a product uses.
func XYZ(template string, o XYZOptions) (Provider, error) {
	if !strings.Contains(template, "{z}") || !strings.Contains(template, "{x}") || !strings.Contains(template, "{y}") {
		return nil, fmt.Errorf("maps: the template %q lacks {z}, {x} or {y}", template)
	}
	if strings.TrimSpace(o.Attribution) == "" {
		return nil, errors.New("maps: a tile service needs its attribution on the map")
	}
	if o.MaxZoom <= 0 {
		o.MaxZoom = 19
	}
	if o.Name == "" {
		o.Name = "xyz:" + template
	}
	if o.Client == nil {
		o.Client = http.DefaultClient
	}
	if o.Kind == Vector && o.Schema == "" {
		return nil, errors.New("maps: a vector tile service needs its schema")
	}
	return &xyz{tmpl: template, o: o}, nil
}

type xyz struct {
	tmpl string
	o    XYZOptions
}

func (p *xyz) Info() ProviderInfo {
	return ProviderInfo{
		Name: p.o.Name, Kind: p.o.Kind, Schema: p.o.Schema,
		MinZoom: p.o.MinZoom, MaxZoom: p.o.MaxZoom, Bounds: geo.World,
		Attribution: p.o.Attribution, Prefetch: p.o.Prefetch, Revision: p.o.Revision,
	}
}

// url is the address of a tile.
func (p *xyz) url(t TileID) string {
	r := strings.NewReplacer("{z}", strconv.Itoa(t.Z), "{x}", strconv.Itoa(t.X), "{y}", strconv.Itoa(t.Y))
	u := r.Replace(p.tmpl)
	if len(p.o.Subdomains) > 0 {
		u = strings.ReplaceAll(u, "{s}", p.o.Subdomains[(t.X+t.Y)%len(p.o.Subdomains)])
	}
	return u
}

// TileSource makes a raster tile an HTTP source of the image pipeline; see
// [SourceProvider].
func (p *xyz) TileSource(t TileID) asset.Source {
	src := asset.HTTP(p.url(t))
	for k, vs := range p.o.Header {
		for _, v := range vs {
			src = src.WithHeader(k, v)
		}
	}
	return src
}

// Tile fetches a tile itself, for vector tiles and for a caller outside the
// map.
func (p *xyz) Tile(ctx context.Context, t TileID) (Tile, error) {
	if !t.Valid() || t.Z < p.o.MinZoom || t.Z > p.o.MaxZoom {
		return Tile{}, ErrNoTile
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url(t), nil)
	if err != nil {
		return Tile{}, err
	}
	for k, vs := range p.o.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", asset.DefaultUserAgent)
	}
	resp, err := p.o.Client.Do(req)
	if err != nil {
		return Tile{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent:
		return Tile{}, ErrNoTile
	case resp.StatusCode != http.StatusOK:
		return Tile{}, &asset.StatusError{Status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Tile{}, err
	}
	// A vector tile is often stored gzipped and served without saying so.
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			if d, err := io.ReadAll(zr); err == nil {
				data = d
			}
		}
	}
	return Tile{Data: data, Format: sniffFormat(data, p.o.Kind)}, nil
}

func sniffFormat(b []byte, kind TileKind) TileFormat {
	switch {
	case kind == Vector:
		return FormatMVT
	case bytes.HasPrefix(b, []byte("\x89PNG")):
		return FormatPNG
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8}):
		return FormatJPEG
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return FormatWebP
	}
	return FormatUnknown
}

// SourceProvider is an optional extension of [Provider]: a provider whose
// raster tiles the image pipeline fetches itself, as [asset.Source]s of its
// own — [asset.HTTP] for a tile service, so that the pipeline's HTTP cache
// with its revalidation applies, rather than the bytes of [Provider.Tile]
// under the provider's revision.
type SourceProvider interface {
	Provider
	TileSource(t TileID) asset.Source
}
