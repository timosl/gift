//go:build !linux

package ebiten

import (
	"log/slog"
	"time"
)

// openPanel has nothing to open where there is no evdev: a platform with
// touch input of its own delivers it through Ebitengine.
func openPanel(string, time.Time, *slog.Logger, func()) panel { return nil }
