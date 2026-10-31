package maps

import "github.com/worldiety/gift/ui"

// Style draws the vector tiles of one schema.
type Style struct {
	// Schema is the layer schema the style is written for; see
	// [ProviderInfo.Schema].
	Schema string
	// Background is the colour of the land where no layer draws, and of the
	// map before its tiles arrive.
	Background ui.Color
}

func (s *Style) background() ui.Color { return s.Background }

// id names the style in cache keys.
func (s *Style) id() string { return s.Schema }
