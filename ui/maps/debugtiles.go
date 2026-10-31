package maps

import (
	"context"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/worldiety/gift/geo"
)

// DebugTiles returns a provider that draws every tile itself: a tint per zoom
// level, the tile's outline, a finer grid and its z/x/y in the corner. It
// needs no data and no network, which makes it the source of the map's own
// tests and a way to see the tile pyramid at work: which level is shown, and
// which coarser one stands in while it loads.
func DebugTiles() Provider { return debugTiles{px: 2 * TileSize} }

type debugTiles struct{ px int }

func (d debugTiles) Info() ProviderInfo {
	return ProviderInfo{
		Name:        "debug",
		Kind:        Raster,
		MinZoom:     0,
		MaxZoom:     22,
		Bounds:      geo.World,
		Attribution: "gift debug tiles",
		Prefetch:    true,
		Revision:    "1",
	}
}

// debugTints are the background of the levels, one per zoom modulo their
// number, light enough for the black text.
var debugTints = [...]color.RGBA{
	{232, 240, 250, 255}, {236, 248, 236, 255}, {252, 244, 230, 255},
	{246, 236, 248, 255}, {236, 246, 246, 255}, {250, 236, 236, 255},
}

func (d debugTiles) Tile(_ context.Context, t TileID) (Tile, error) {
	if !t.Valid() {
		return Tile{}, ErrNoTile
	}
	img := DrawDebugTile(t, d.px)
	return Tile{Data: EncodePixels(img), Format: FormatPixels}, nil
}

// DrawDebugTile draws the debug picture of t at px by px pixels.
func DrawDebugTile(t TileID, px int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	draw.Draw(img, img.Rect, &image.Uniform{debugTints[t.Z%len(debugTints)]}, image.Point{}, draw.Src)
	grid := color.RGBA{200, 205, 215, 255}
	edge := color.RGBA{90, 100, 120, 255}
	for i := 0; i < px; i++ {
		for _, k := range []int{px / 4, px / 2, 3 * px / 4} {
			img.SetRGBA(k, i, grid)
			img.SetRGBA(i, k, grid)
		}
		for _, k := range []int{0, 1, px - 2, px - 1} {
			img.SetRGBA(k, i, edge)
			img.SetRGBA(i, k, edge)
		}
	}
	fd := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{30, 30, 40, 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(8, 18),
	}
	fd.DrawString(t.String())
	return img
}
