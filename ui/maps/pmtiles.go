package maps

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geo"
)

// PMTilesOptions adjusts what [PMTiles] and [PMTilesURL] report about an
// archive. The zero value takes everything from the archive.
type PMTilesOptions struct {
	// Schema overrides the schema read from the metadata; see
	// [ProviderInfo.Schema].
	Schema string
	// Attribution overrides the attribution read from the metadata.
	Attribution string
	// Client is the HTTP client of [PMTilesURL]; nil selects
	// http.DefaultClient.
	Client *http.Client
	// UserAgent is sent with every request of [PMTilesURL]. Empty selects
	// [asset.DefaultUserAgent].
	UserAgent string
}

// PMTiles opens a PMTiles archive (version 3) in a local file: a whole
// map, or a region of one, in a single file and without a tile server.
// Protomaps publishes daily builds of OpenStreetMap as such archives, and a
// region is cut from one with its pmtiles tool; see the project plan,
// section 37.
//
// The archive is read on demand: the header and the root directory now,
// further directories and the tiles when they are asked for. The file stays
// open for the life of the provider.
func PMTiles(path string, opts ...PMTilesOptions) (Provider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	rev := strconv.FormatInt(st.ModTime().UnixNano(), 36) + "-" + strconv.FormatInt(st.Size(), 36)
	p, err := openPMTiles(f, "pmtiles:"+path, rev, first(opts))
	if err != nil {
		f.Close()
		return nil, err
	}
	return p, nil
}

// PMTilesURL opens a PMTiles archive on an HTTP server, read with range
// requests: self hosting is a static file on any server that answers them,
// nginx, Caddy or an S3 compatible store. Nothing but the header and the
// root directory is read here.
func PMTilesURL(url string, opts ...PMTilesOptions) (Provider, error) {
	o := first(opts)
	r := &rangeReader{url: url, client: o.Client, agent: o.UserAgent}
	if r.client == nil {
		r.client = http.DefaultClient
	}
	if r.agent == "" {
		r.agent = asset.DefaultUserAgent
	}
	p, err := openPMTiles(r, "pmtiles:"+url, "", o)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	p.info.Revision = r.etag
	r.mu.Unlock()
	return p, nil
}

func first(opts []PMTilesOptions) PMTilesOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return PMTilesOptions{}
}

// pmHeader is the fixed 127 byte header of version 3 of the format.
type pmHeader struct {
	rootOff, rootLen uint64
	metaOff, metaLen uint64
	leafOff, leafLen uint64
	dataOff, dataLen uint64
	internalComp     uint8
	tileComp         uint8
	tileType         uint8
	minZoom, maxZoom uint8
	minLon, minLat   int32
	maxLon, maxLat   int32
}

const pmHeaderLen = 127

// The compression and tile type codes of the header.
const (
	pmCompNone = 1
	pmCompGzip = 2

	pmTypeMVT  = 1
	pmTypePNG  = 2
	pmTypeJPEG = 3
	pmTypeWebP = 4
)

func parseHeader(b []byte) (pmHeader, error) {
	if len(b) < pmHeaderLen || string(b[:7]) != "PMTiles" {
		return pmHeader{}, errors.New("maps: not a PMTiles archive")
	}
	if b[7] != 3 {
		return pmHeader{}, fmt.Errorf("maps: PMTiles version %d, only 3 is supported", b[7])
	}
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i:]) }
	s := func(i int) int32 { return int32(binary.LittleEndian.Uint32(b[i:])) }
	return pmHeader{
		rootOff: u(8), rootLen: u(16), metaOff: u(24), metaLen: u(32),
		leafOff: u(40), leafLen: u(48), dataOff: u(56), dataLen: u(64),
		internalComp: b[97], tileComp: b[98], tileType: b[99],
		minZoom: b[100], maxZoom: b[101],
		minLon: s(102), minLat: s(106), maxLon: s(110), maxLat: s(114),
	}, nil
}

// pmEntry is one entry of a directory: a run of tiles in the data section,
// or, with a run length of zero, a leaf directory.
type pmEntry struct {
	id     uint64
	off    uint64
	length uint32
	run    uint32
}

// parseDirectory decodes a directory: the entry count, then the delta coded
// tile IDs, the run lengths, the lengths and the offsets, each as varints,
// where an offset of zero continues the previous entry.
func parseDirectory(b []byte) ([]pmEntry, error) {
	r := bytes.NewReader(b)
	n, err := binary.ReadUvarint(r)
	if err != nil || n > 1<<24 {
		return nil, errors.New("maps: bad PMTiles directory")
	}
	es := make([]pmEntry, n)
	var last uint64
	for i := range es {
		d, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		last += d
		es[i].id = last
	}
	for i := range es {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		es[i].run = uint32(v)
	}
	for i := range es {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		es[i].length = uint32(v)
	}
	for i := range es {
		v, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, err
		}
		if v == 0 && i > 0 {
			es[i].off = es[i-1].off + uint64(es[i-1].length)
		} else {
			es[i].off = v - 1
		}
	}
	return es, nil
}

// findEntry returns the entry that holds id: the last one whose ID is not
// above it.
func findEntry(es []pmEntry, id uint64) (pmEntry, bool) {
	lo, hi := 0, len(es)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case es[mid].id < id:
			lo = mid + 1
		case es[mid].id > id:
			hi = mid - 1
		default:
			return es[mid], true
		}
	}
	if hi >= 0 {
		e := es[hi]
		if e.run == 0 {
			return e, true
		}
		if id-e.id < uint64(e.run) {
			return e, true
		}
	}
	return pmEntry{}, false
}

// tileIDOf is the PMTiles number of a tile: all tiles of the levels above,
// then the position along the Hilbert curve of its level.
func tileIDOf(t TileID) uint64 {
	z := uint(t.Z)
	acc := ((uint64(1) << (2 * z)) - 1) / 3
	x, y := uint64(t.X), uint64(t.Y)
	for s := uint64(1) << z >> 1; s > 0; s >>= 1 {
		var rx, ry uint64
		if x&s != 0 {
			rx = 1
		}
		if y&s != 0 {
			ry = 1
		}
		acc += s * s * ((3 * rx) ^ ry)
		if ry == 0 {
			if rx == 1 {
				x = s - 1 - x
				y = s - 1 - y
			}
			x, y = y, x
		}
	}
	return acc
}

// pmProvider is a PMTiles archive behind the provider seam.
type pmProvider struct {
	r    io.ReaderAt
	h    pmHeader
	root []pmEntry
	info ProviderInfo

	mu     sync.Mutex
	leaves map[uint64][]pmEntry
	order  []uint64
}

func openPMTiles(r io.ReaderAt, name, rev string, o PMTilesOptions) (*pmProvider, error) {
	// The header and, in one read, what usually follows it: the root
	// directory sits right behind the header in every archive the
	// reference tools write.
	head := make([]byte, 16384)
	n, err := r.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	head = head[:n]
	h, err := parseHeader(head)
	if err != nil {
		return nil, err
	}
	p := &pmProvider{r: r, h: h, leaves: make(map[uint64][]pmEntry)}
	var rootRaw []byte
	if h.rootOff+h.rootLen <= uint64(len(head)) {
		rootRaw = head[h.rootOff : h.rootOff+h.rootLen]
	} else {
		rootRaw = make([]byte, h.rootLen)
		if _, err := r.ReadAt(rootRaw, int64(h.rootOff)); err != nil {
			return nil, err
		}
	}
	rootData, err := decompress(rootRaw, h.internalComp)
	if err != nil {
		return nil, err
	}
	if p.root, err = parseDirectory(rootData); err != nil {
		return nil, err
	}
	meta := map[string]any{}
	if h.metaLen > 0 && h.metaLen < 1<<24 {
		raw := make([]byte, h.metaLen)
		if _, err := r.ReadAt(raw, int64(h.metaOff)); err == nil {
			if data, err := decompress(raw, h.internalComp); err == nil {
				_ = json.Unmarshal(data, &meta)
			}
		}
	}
	p.info = ProviderInfo{
		Name:        name,
		MinZoom:     int(h.minZoom),
		MaxZoom:     int(h.maxZoom),
		Prefetch:    true,
		Revision:    rev,
		Schema:      o.Schema,
		Attribution: o.Attribution,
		Bounds: geo.Rect{
			Min: geo.LatLon(float64(h.minLat)/1e7, float64(h.minLon)/1e7),
			Max: geo.LatLon(float64(h.maxLat)/1e7, float64(h.maxLon)/1e7),
		},
	}
	if h.tileType == pmTypeMVT {
		p.info.Kind = Vector
	}
	if p.info.Schema == "" {
		p.info.Schema = schemaOf(meta)
	}
	if p.info.Attribution == "" {
		p.info.Attribution = attributionOf(meta)
	}
	return p, nil
}

// schemaOf names the schema an archive's metadata declares. Protomaps writes
// its name and version, from which "protomaps-v4" is formed.
func schemaOf(meta map[string]any) string {
	name, _ := meta["name"].(string)
	version, _ := meta["version"].(string)
	if strings.Contains(strings.ToLower(name), "protomaps") {
		major, _, _ := strings.Cut(version, ".")
		return "protomaps-v" + major
	}
	return name
}

var tags = regexp.MustCompile(`<[^>]*>`)

// attributionOf is the attribution of the metadata without its markup, or
// the OpenStreetMap notice every archive derived from it owes.
func attributionOf(meta map[string]any) string {
	a, _ := meta["attribution"].(string)
	a = strings.TrimSpace(strings.ReplaceAll(tags.ReplaceAllString(a, ""), "&copy;", "©"))
	if a == "" {
		return "© OpenStreetMap"
	}
	return a
}

func decompress(b []byte, comp uint8) ([]byte, error) {
	switch comp {
	case pmCompNone, 0:
		return b, nil
	case pmCompGzip:
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	}
	return nil, fmt.Errorf("maps: PMTiles compression %d is not supported, only gzip", comp)
}

func (p *pmProvider) Info() ProviderInfo { return p.info }

func (p *pmProvider) Tile(ctx context.Context, t TileID) (Tile, error) {
	if !t.Valid() || t.Z < int(p.h.minZoom) || t.Z > int(p.h.maxZoom) {
		return Tile{}, ErrNoTile
	}
	id := tileIDOf(t)
	dir := p.root
	for depth := 0; depth < 4; depth++ {
		e, ok := findEntry(dir, id)
		if !ok {
			return Tile{}, ErrNoTile
		}
		if e.run > 0 {
			raw := make([]byte, e.length)
			if _, err := p.r.ReadAt(raw, int64(p.h.dataOff+e.off)); err != nil && !errors.Is(err, io.EOF) {
				return Tile{}, err
			}
			data, err := decompress(raw, p.h.tileComp)
			if err != nil {
				return Tile{}, fmt.Errorf("%w: %v", asset.ErrNotAPicture, err)
			}
			return Tile{Data: data, Format: formatOf(p.h.tileType)}, nil
		}
		leaf, err := p.leaf(e)
		if err != nil {
			return Tile{}, err
		}
		dir = leaf
	}
	return Tile{}, ErrNoTile
}

// leaf returns a leaf directory, from a small cache: neighbouring tiles share
// their leaf, and a pan asks for many neighbours at once.
func (p *pmProvider) leaf(e pmEntry) ([]pmEntry, error) {
	p.mu.Lock()
	if d, ok := p.leaves[e.off]; ok {
		p.mu.Unlock()
		return d, nil
	}
	p.mu.Unlock()
	raw := make([]byte, e.length)
	if _, err := p.r.ReadAt(raw, int64(p.h.leafOff+e.off)); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	data, err := decompress(raw, p.h.internalComp)
	if err != nil {
		return nil, err
	}
	d, err := parseDirectory(data)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.leaves[e.off]; !ok {
		p.leaves[e.off] = d
		p.order = append(p.order, e.off)
		if len(p.order) > 64 {
			delete(p.leaves, p.order[0])
			p.order = p.order[1:]
		}
	}
	return d, nil
}

func formatOf(tileType uint8) TileFormat {
	switch tileType {
	case pmTypeMVT:
		return FormatMVT
	case pmTypePNG:
		return FormatPNG
	case pmTypeJPEG:
		return FormatJPEG
	case pmTypeWebP:
		return FormatWebP
	}
	return FormatUnknown
}

// rangeReader reads a file on an HTTP server with range requests.
type rangeReader struct {
	url    string
	client *http.Client
	agent  string

	mu   sync.Mutex
	etag string
}

func (r *rangeReader) ReadAt(b []byte, off int64) (int, error) {
	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+int64(len(b))-1))
	req.Header.Set("User-Agent", r.agent)
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		// A server that ignores the range sends the whole file; it is read
		// up to the part that was asked for.
		if _, err := io.CopyN(io.Discard, resp.Body, off); err != nil {
			return 0, err
		}
	default:
		return 0, &asset.StatusError{Status: resp.StatusCode}
	}
	r.mu.Lock()
	if r.etag == "" {
		r.etag = resp.Header.Get("ETag")
	}
	r.mu.Unlock()
	n, err := io.ReadFull(resp.Body, b)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		// Past the end of the file: a short read, as ReadAt reports one.
		err = io.EOF
	}
	return n, err
}
