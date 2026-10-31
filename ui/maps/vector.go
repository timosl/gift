package maps

import (
	"context"
	"fmt"

	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/ui"
)

// fetchVector reads a vector tile and wraps it for the rasterizer of the
// pipeline; see [MIMEVectorJob]. Above the provider's last level the tile is
// drawn from the geometry of its ancestor there, enlarged: sharp at any zoom,
// because it is drawn again and not scaled. Outside the provider's bounds it
// is drawn the same way from its nearest ancestor inside them; see
// [Map.dataTile].
func (m *Map) fetchVector(ctx context.Context, k srcKey) ([]byte, error) {
	st := m.styles[k.style]
	src, ok := m.dataTile(k.t)
	if !ok {
		return nil, ErrNoTile
	}
	t, err := m.cfg.Tiles.Tile(ctx, src)
	if err != nil {
		return nil, err
	}
	if t.Format != FormatMVT {
		return nil, fmt.Errorf("%w: maps: a vector provider delivered format %d", asset.ErrNotAPicture, t.Format)
	}
	return encodeVectorJob(vectorJob{
		style: st.register(), want: k.t, src: src, px: k.px,
		scale: float32(k.px) / TileSize,
	}, t.Data), nil
}

// styleIndex is the style of the current theme: the dark style when the
// theme is dark and the map has one.
func (m *Map) styleIndex() uint8 {
	if m.styles[1] != nil && ui.CurrentTheme().IsDark() {
		return 1
	}
	return 0
}
