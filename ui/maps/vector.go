package maps

import (
	"context"
	"errors"
)

func (m *Map) fetchVector(ctx context.Context, k srcKey) ([]byte, error) {
	return nil, errors.New("maps: vector tiles are not drawn yet")
}
