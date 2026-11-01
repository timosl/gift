//go:build !darwin || ios

package ebiten

// openWheelPhases has nothing to read outside macOS: GLFW reports no phases
// of a trackpad gesture there, and neither does the platform underneath it in
// a form the backend could reach. See the darwin variant.
func openWheelPhases() func([]wheelSample) []wheelSample { return nil }
