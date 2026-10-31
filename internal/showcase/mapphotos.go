package showcase

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/internal/exifwrite"
)

// MapPhotos writes n small photographs with a location and a time into dir,
// once, and returns them: a camera roll of somebody who lives in Oldenburg,
// in clumps around the places they go, for the map of photographs. The
// pictures are generated, so nothing has to be checked in, and the same n
// gives the same roll on every machine.
func MapPhotos(dir string, n int) ([]asset.Source, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rng := rand.New(rand.NewPCG(53, 8))
	// Places in and around Oldenburg, inside the extract of the project
	// plan, section 37: the old town, the castle garden, the harbour, the
	// university, the Eversten woods, the Haaren and two villages.
	places := []struct{ lat, lon, spread, weight float64 }{
		{53.1405, 8.2135, 0.002, 0.25}, {53.1365, 8.2125, 0.0015, 0.1}, {53.1430, 8.2330, 0.002, 0.1},
		{53.1475, 8.1810, 0.003, 0.12}, {53.1290, 8.1830, 0.004, 0.1}, {53.1520, 8.2000, 0.004, 0.08},
		{53.1900, 8.2700, 0.006, 0.07}, {53.0800, 8.1200, 0.008, 0.06}, {53.2100, 8.0900, 0.01, 0.06},
		{53.1600, 8.3200, 0.008, 0.06},
	}
	t0 := time.Date(2021, 3, 1, 9, 0, 0, 0, time.UTC)
	out := make([]asset.Source, 0, n)
	for i := range n {
		u, p := rng.Float64(), 0
		for p < len(places)-1 && u > places[p].weight {
			u -= places[p].weight
			p++
		}
		pl := places[p]
		lat := pl.lat + rng.NormFloat64()*pl.spread
		lon := pl.lon + rng.NormFloat64()*pl.spread*1.6
		taken := t0.Add(time.Duration(rng.Int64N(int64(5 * 365 * 24 * time.Hour))))
		hue := rng.Float64()
		path := filepath.Join(dir, "photo"+strconv.Itoa(i)+".jpg")
		if _, err := os.Stat(path); err != nil {
			raw, err := exifwrite.JPEG(samplePhoto(hue, i), lat, lon, taken)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				return nil, err
			}
		}
		out = append(out, asset.File(path))
	}
	return out, nil
}

// samplePhoto is a landscape of two bands of colour around a hue, sky over
// ground, with a sun: different enough per photograph to tell the markers
// apart.
func samplePhoto(hue float64, i int) *image.RGBA {
	const w, h = 160, 120
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sky := hsv(hue, 0.45, 0.95)
	ground := hsv(math.Mod(hue+0.35, 1), 0.6, 0.6)
	sx, sy := 30+(i*37)%100, 20+(i*13)%30
	for y := range h {
		for x := range w {
			c := sky
			if y > h/2+int(8*math.Sin(float64(x)/18+hue*6)) {
				c = ground
			}
			if dx, dy := x-sx, y-sy; dx*dx+dy*dy < 120 {
				c = color.RGBA{255, 236, 170, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func hsv(h, s, v float64) color.RGBA {
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}
