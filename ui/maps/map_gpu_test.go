//go:build giftgpu

package maps_test

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/maps"
)

// solidTiles is a provider whose every tile is one colour, so that a seam
// between two tiles shows as a pixel of another.
type solidTiles struct{ c color.RGBA }

func (s solidTiles) Info() maps.ProviderInfo {
	return maps.ProviderInfo{Name: "solid", MaxZoom: 18, Bounds: geo.World, Revision: "1"}
}

func (s solidTiles) Tile(_ context.Context, t maps.TileID) (maps.Tile, error) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	draw.Draw(img, img.Rect, &image.Uniform{s.c}, image.Point{}, draw.Src)
	return maps.Tile{Data: maps.EncodePixels(img), Format: maps.FormatPixels}, nil
}

// TestTurnedTilesAtAFractionalZoomHaveNoSeams is the seam check of the
// project plan, section 37: at a heading of 17 degrees and zoom 3.4 every
// tile is drawn at 1.32 times its size and turned, and between two
// neighbours not a single pixel of the background may show.
func TestTurnedTilesAtAFractionalZoomHaveNoSeams(t *testing.T) {
	fill := color.RGBA{40, 120, 200, 255}
	ch := make(chan func(), 4096)
	pipe := asset.NewPipeline(asset.Config{Deliver: func(fn func()) { ch <- fn }, Sizes: []int{256, 512}, Workers: 2})
	t.Cleanup(func() { pipe.Close() })
	ui.SetImagePipeline(pipe)
	t.Cleanup(func() { ui.SetImagePipeline(nil) })
	m, err := maps.New(maps.Config{
		Tiles:    solidTiles{fill},
		Camera:   maps.Camera{Center: geo.LatLon(30, 20), Zoom: 3.4, Heading: 17},
		Rotation: maps.RotationOn,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := gifttest.New(t, gifttest.Options{
		View: maps.View(m), Size: geom.Sz(480, 320),
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme(),
	})
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for drained := true; drained; {
			select {
			case fn := <-ch:
				fn()
			default:
				drained = false
			}
		}
		h.Frame()
		h.Image()
		if st := m.Stats(); st.Tiles > 0 && st.Drawn == st.Tiles {
			break
		}
		time.Sleep(time.Millisecond)
	}
	img := h.Image()
	b := img.Bounds()
	bad := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			// The attribution in the bottom right and the compass in the
			// top right are not tiles.
			if (x > b.Max.X-140 && y > b.Max.Y-30) || (x > b.Max.X-60 && y < 60) {
				continue
			}
			r, g, bl, _ := img.At(x, y).RGBA()
			if d := max(absDiff(r>>8, uint32(fill.R)), absDiff(g>>8, uint32(fill.G)), absDiff(bl>>8, uint32(fill.B))); d > 3 {
				if bad < 5 {
					t.Errorf("pixel %d,%d is %v, not the tiles' colour: a seam", x, y, img.At(x, y))
				}
				bad++
			}
		}
	}
	if bad > 0 {
		if out := os.Getenv("GIFT_MAPS_SEAM_OUT"); out != "" {
			f, _ := os.Create(out)
			png.Encode(f, img)
			f.Close()
		}
		t.Fatalf("%d pixels of seam", bad)
	}
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
