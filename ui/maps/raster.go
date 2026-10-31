package maps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"sync"

	"golang.org/x/image/vector"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/ui"
)

// MIMEVectorJob is the media type of a vector tile on its way through the
// image pipeline: the tile's bytes with what the rasterizer needs to draw
// them, which the pipeline hands to the decoder registered for it.
//
// This is how vector tiles are drawn on the pipeline's own goroutines, under
// its decode budget and with its cache, without a path of their own: the
// "decode" of such a picture is the drawing of the tile. See the project
// plan, section 37, "Anschluss an die Pipeline".
const MIMEVectorJob = "application/x-gift-vector-tile"

var vectorMagic = [8]byte{'G', 'I', 'F', 'T', 'V', 'E', 'C', '1'}

// vectorJob is the header of the container: the style by its registry
// number, the tile wanted and the tile whose geometry draws it, which is an
// ancestor above the provider's last level, and the edge in pixels.
type vectorJob struct {
	style uint64
	want  TileID
	src   TileID
	px    int
	scale float32 // device pixels per logical pixel
}

const vectorHeaderLen = 8 + 8 + 6*4 + 4 + 4

func encodeVectorJob(j vectorJob, mvt []byte) []byte {
	out := make([]byte, vectorHeaderLen+len(mvt))
	copy(out, vectorMagic[:])
	le := binary.LittleEndian
	le.PutUint64(out[8:], j.style)
	for i, v := range [...]int{j.want.Z, j.want.X, j.want.Y, j.src.Z, j.src.X, j.src.Y, j.px} {
		le.PutUint32(out[16+4*i:], uint32(v))
	}
	le.PutUint32(out[44:], math.Float32bits(j.scale))
	copy(out[vectorHeaderLen:], mvt)
	return out
}

func decodeVectorHeader(b []byte) (vectorJob, bool) {
	if len(b) < vectorHeaderLen || !bytes.Equal(b[:8], vectorMagic[:]) {
		return vectorJob{}, false
	}
	le := binary.LittleEndian
	u := func(i int) int { return int(le.Uint32(b[16+4*i:])) }
	j := vectorJob{
		style: le.Uint64(b[8:]),
		want:  TileID{Z: u(0), X: u(1), Y: u(2)},
		src:   TileID{Z: u(3), X: u(4), Y: u(5)},
		px:    u(6),
		scale: math.Float32frombits(le.Uint32(b[44:])),
	}
	if j.px <= 0 || j.px > 4096 {
		return vectorJob{}, false
	}
	return j, true
}

// vectorDecoder draws vector tiles for the image pipeline.
type vectorDecoder struct{}

func (vectorDecoder) Sniff(h []byte) bool { return bytes.HasPrefix(h, vectorMagic[:]) }

func (vectorDecoder) DecodeConfig(r io.Reader) (image.Config, error) {
	head := make([]byte, vectorHeaderLen)
	if _, err := io.ReadFull(r, head); err != nil {
		return image.Config{}, err
	}
	j, ok := decodeVectorHeader(head)
	if !ok {
		return image.Config{}, errNotVector
	}
	return image.Config{ColorModel: color.RGBAModel, Width: j.px, Height: j.px}, nil
}

func (vectorDecoder) Decode(r io.Reader) (image.Image, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	j, ok := decodeVectorHeader(b)
	if !ok {
		return nil, errNotVector
	}
	s, ok := styleByID(j.style)
	if !ok {
		return nil, fmt.Errorf("%w: maps: unknown style %d", asset.ErrNotAPicture, j.style)
	}
	return rasterize(s, j, b[vectorHeaderLen:])
}

// MemoryFactor is the tile, the rasterizer's coverage buffer of float32 per
// pixel and the decoded geometry.
func (vectorDecoder) MemoryFactor() float64 { return 12 }

var errNotVector = fmt.Errorf("%w: maps: not a vector tile job", asset.ErrNotAPicture)

func init() {
	asset.RegisterDecoder(MIMEVectorJob, vectorDecoder{})
}

// rasterizers are reused across tiles; each holds a coverage buffer as large
// as the largest area it drew.
var rasterizers = sync.Pool{New: func() any { return &painter{z: vector.NewRasterizer(1, 1)} }}

// painter draws the batches of one tile.
type painter struct {
	z    *vector.Rasterizer
	mask image.Alpha
	// pts is the geometry of the batch in pixels: rings, each ending with a
	// NaN marker, so that the bounding box is known before anything is
	// handed to the rasterizer.
	pts []float32
	// ring is the current polygon ring or line, before it is added.
	ring []float32
}

// rasterize draws a vector tile with a style.
func rasterize(s *Style, j vectorJob, mvt []byte) (*image.RGBA, error) {
	s.register()
	t, err := decodeMVT(mvt, func(name string) bool { return s.srcs[name] })
	if err != nil {
		return nil, fmt.Errorf("%w: %v", asset.ErrNotAPicture, err)
	}
	px := j.px
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	bg := ui.ResolveColor(s.Background)
	draw.Draw(img, img.Rect, image.NewUniform(rgba8(bg)), image.Point{}, draw.Src)

	p := rasterizers.Get().(*painter)
	defer rasterizers.Put(p)
	z := float64(j.want.Z)
	dz := j.want.Z - j.src.Z
	for li := range s.Layers {
		ly := &s.Layers[li]
		if !ly.applies(z) {
			continue
		}
		ml := t.layer(ly.Source)
		if ml == nil || len(ml.features) == 0 {
			continue
		}
		// Tile units to pixels of the wanted tile: an ancestor's geometry is
		// enlarged and shifted to the quarter, sixteenth, ... wanted.
		k := float64(px) * math.Exp2(float64(dz)) / float64(ml.extent)
		ox := -float64(j.want.X-j.src.X<<dz) * float64(px)
		oy := -float64(j.want.Y-j.src.Y<<dz) * float64(px)
		fill := ui.ResolveColor(ly.Fill)
		stroke := ui.ResolveColor(ly.Line)
		width := ly.Width.At(z) * float64(j.scale)
		if !fill.IsTransparent() {
			p.pts = p.pts[:0]
			for fi := range ml.features {
				f := &ml.features[fi]
				if f.typ != geomPolygon || !matches(ly.Kinds, ly.Details, ml.str(f, "kind"), ml.str(f, "kind_detail")) {
					continue
				}
				p.polygon(f, k, ox, oy)
			}
			p.draw(img, fill)
		}
		if !stroke.IsTransparent() && width > 0 {
			p.pts = p.pts[:0]
			for fi := range ml.features {
				f := &ml.features[fi]
				if f.typ == geomPoint || (f.typ == geomPolygon && !fill.IsTransparent()) {
					continue
				}
				if !matches(ly.Kinds, ly.Details, ml.str(f, "kind"), ml.str(f, "kind_detail")) {
					continue
				}
				p.stroke(f, k, ox, oy, width, f.typ == geomPolygon)
			}
			p.draw(img, stroke)
		}
	}
	return img, nil
}

var ringEnd = float32(math.NaN())

// polygon adds the rings of a polygon feature. The winding the format
// prescribes — exterior rings clockwise on the screen, holes the other way —
// is what the rasterizer's signed coverage needs: a hole cancels its ring.
func (p *painter) polygon(f *mvtFeature, k, ox, oy float64) {
	f.path(func(x, y float64) {
		p.pts = append(p.pts, float32(x*k+ox), float32(y*k+oy))
	}, func(x, y float64) {
		p.pts = append(p.pts, float32(x*k+ox), float32(y*k+oy))
	}, func() {
		p.pts = append(p.pts, ringEnd, ringEnd)
	})
}

// stroke adds the outline of a line, w pixels wide: a quad per segment and a
// disc at every inner vertex for the joins. Every piece has the same
// winding, so where they overlap the coverage adds up and saturates instead
// of cancelling.
func (p *painter) stroke(f *mvtFeature, k, ox, oy, w float64, closed bool) {
	p.ring = p.ring[:0]
	flush := func() {
		r := p.ring
		if closed && len(r) >= 4 {
			r = append(r, r[0], r[1])
		}
		p.strokeRing(r, w/2)
		p.ring = p.ring[:0]
	}
	f.path(func(x, y float64) {
		if len(p.ring) > 0 {
			flush()
		}
		p.ring = append(p.ring, float32(x*k+ox), float32(y*k+oy))
	}, func(x, y float64) {
		p.ring = append(p.ring, float32(x*k+ox), float32(y*k+oy))
	}, func() {})
	if len(p.ring) > 0 {
		flush()
	}
}

func (p *painter) strokeRing(r []float32, h float64) {
	for i := 0; i+3 < len(r); i += 2 {
		x0, y0, x1, y1 := float64(r[i]), float64(r[i+1]), float64(r[i+2]), float64(r[i+3])
		dx, dy := x1-x0, y1-y0
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		// The left normal: (dy, -dx). Corners in this order have a positive
		// signed area whichever way the segment points.
		nx, ny := dy/l*h, -dx/l*h
		p.pts = append(p.pts,
			float32(x0+nx), float32(y0+ny),
			float32(x1+nx), float32(y1+ny),
			float32(x1-nx), float32(y1-ny),
			float32(x0-nx), float32(y0-ny),
			ringEnd, ringEnd)
		if i > 0 && h >= 0.75 {
			p.disc(x0, y0, h)
		}
	}
}

// disc adds a regular polygon around x, y, finer for wider lines.
func (p *painter) disc(x, y, r float64) {
	n := 8
	if r > 4 {
		n = 16
	}
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		s, c := math.Sincos(a)
		p.pts = append(p.pts, float32(x+r*c), float32(y+r*s))
	}
	p.pts = append(p.pts, ringEnd, ringEnd)
}

// draw rasterizes the batch over its bounding box and composites it onto
// img in c.
func (p *painter) draw(img *image.RGBA, c ui.Color) {
	if len(p.pts) == 0 {
		return
	}
	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	for i := 0; i+1 < len(p.pts); i += 2 {
		x, y := p.pts[i], p.pts[i+1]
		if x != x {
			continue
		}
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	b := img.Rect.Intersect(image.Rect(int(math.Floor(float64(minX))), int(math.Floor(float64(minY))),
		int(math.Ceil(float64(maxX)))+1, int(math.Ceil(float64(maxY)))+1))
	if b.Empty() {
		return
	}
	z := p.z
	z.Reset(b.Dx(), b.Dy())
	z.DrawOp = draw.Src
	bx, by := float32(b.Min.X), float32(b.Min.Y)
	open := false
	for i := 0; i+1 < len(p.pts); i += 2 {
		x, y := p.pts[i], p.pts[i+1]
		switch {
		case x != x:
			if open {
				z.ClosePath()
			}
			open = false
		case !open:
			z.MoveTo(x-bx, y-by)
			open = true
		default:
			z.LineTo(x-bx, y-by)
		}
	}
	if open {
		z.ClosePath()
	}
	// Coverage into a mask first, by the rasterizer's own fast path for an
	// opaque source over an alpha image, then composited here, touching only
	// the covered pixels. Drawing the colour straight onto the tile spent
	// most of a tile's time blending pixels the batch did not cover: a
	// batch's bounding box is usually most of the tile, and a style has two
	// dozen of them.
	n := b.Dx() * b.Dy()
	if cap(p.mask.Pix) < n {
		p.mask.Pix = make([]uint8, n)
	}
	p.mask.Pix = p.mask.Pix[:n]
	p.mask.Stride, p.mask.Rect = b.Dx(), image.Rect(0, 0, b.Dx(), b.Dy())
	z.Draw(&p.mask, p.mask.Rect, image.Opaque, image.Point{})
	composite(img, b, &p.mask, rgba8(c))
}

// composite draws colour col through mask onto the part b of img, source
// over destination, both premultiplied.
func composite(img *image.RGBA, b image.Rectangle, mask *image.Alpha, col color.RGBA) {
	cr, cg, cb, ca := uint32(col.R), uint32(col.G), uint32(col.B), uint32(col.A)
	for y := range b.Dy() {
		row := mask.Pix[y*mask.Stride : y*mask.Stride+b.Dx()]
		dst := img.Pix[(b.Min.Y+y)*img.Stride+4*b.Min.X:]
		for x, m := range row {
			if m == 0 {
				continue
			}
			d := dst[4*x : 4*x+4 : 4*x+4]
			if m == 255 && ca == 255 {
				d[0], d[1], d[2], d[3] = col.R, col.G, col.B, 255
				continue
			}
			f := uint32(m)
			// out = c·f + d·(1 − a·f), in units of 255·255.
			inv := 255*255 - ca*f
			d[0] = uint8((cr*f*255 + uint32(d[0])*inv + 255*255/2) / (255 * 255))
			d[1] = uint8((cg*f*255 + uint32(d[1])*inv + 255*255/2) / (255 * 255))
			d[2] = uint8((cb*f*255 + uint32(d[2])*inv + 255*255/2) / (255 * 255))
			d[3] = uint8((ca*f*255 + uint32(d[3])*inv + 255*255/2) / (255 * 255))
		}
	}
}

// rgba8 is c as the premultiplied 8 bit colour of an image.RGBA; both are
// premultiplied, so this is only a change of scale.
func rgba8(c ui.Color) color.RGBA {
	q := func(v float32) uint8 { return uint8(math.Round(float64(max(0, min(1, v)) * 255))) }
	return color.RGBA{q(c.R), q(c.G), q(c.B), q(c.A)}
}
