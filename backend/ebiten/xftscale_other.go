//go:build !linux || android

package ebiten

// newSystemScale has nothing to correct outside X11: there Ebitengine's
// monitor reading follows the desktop. See the X11 variant.
func newSystemScale() func() float64 { return nil }
