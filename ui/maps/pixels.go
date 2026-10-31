package maps

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"

	"github.com/worldiety/gift/asset"
)

// MIMEPixels is the media type of gift's raw pixel container, the encoding of
// [FormatPixels].
const MIMEPixels = "application/x-gift-pixels"

// pixelsMagic opens the container. Eight bytes, so that the sniffer of the
// image pipeline recognises it before any other decoder looks.
var pixelsMagic = [8]byte{'G', 'I', 'F', 'T', 'P', 'I', 'X', '1'}

// EncodePixels returns img in gift's raw pixel container: the magic, the
// width and height as little endian uint32 and the premultiplied RGBA rows.
//
// It exists for a provider that draws its own tiles, [DebugTiles] first: an
// encoding would cost a PNG round trip per tile for pixels that never leave
// the process.
func EncodePixels(img *image.RGBA) []byte {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	out := make([]byte, 16+4*w*h)
	copy(out, pixelsMagic[:])
	binary.LittleEndian.PutUint32(out[8:], uint32(w))
	binary.LittleEndian.PutUint32(out[12:], uint32(h))
	for y := range h {
		row := img.Pix[(y)*img.Stride : y*img.Stride+4*w]
		copy(out[16+4*w*y:], row)
	}
	return out
}

// pixelsDecoder decodes the container for the image pipeline.
type pixelsDecoder struct{}

func (pixelsDecoder) Sniff(h []byte) bool { return bytes.HasPrefix(h, pixelsMagic[:]) }

func (pixelsDecoder) DecodeConfig(r io.Reader) (image.Config, error) {
	var head [16]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return image.Config{}, err
	}
	if !bytes.Equal(head[:8], pixelsMagic[:]) {
		return image.Config{}, errNotPixels
	}
	return image.Config{
		ColorModel: color.RGBAModel,
		Width:      int(binary.LittleEndian.Uint32(head[8:])),
		Height:     int(binary.LittleEndian.Uint32(head[12:])),
	}, nil
}

func (d pixelsDecoder) Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decodePixels(data)
}

func decodePixels(data []byte) (*image.RGBA, error) {
	if len(data) < 16 || !bytes.Equal(data[:8], pixelsMagic[:]) {
		return nil, errNotPixels
	}
	w := int(binary.LittleEndian.Uint32(data[8:]))
	h := int(binary.LittleEndian.Uint32(data[12:]))
	if w <= 0 || h <= 0 || w > 1<<14 || h > 1<<14 || len(data) != 16+4*w*h {
		return nil, errNotPixels
	}
	return &image.RGBA{Pix: data[16:], Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}, nil
}

// MemoryFactor is the decoded copy and the read buffer.
func (pixelsDecoder) MemoryFactor() float64 { return 8 }

var errNotPixels = fmt.Errorf("%w: maps: not a gift pixel container", asset.ErrNotAPicture)

func init() {
	asset.RegisterDecoder(MIMEPixels, pixelsDecoder{})
}
