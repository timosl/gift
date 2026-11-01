//go:build !(darwin || freebsd || linux)

package typst

var lib struct{ path string }

// load always fails where purego cannot dlopen; see asset/turbojpeg for why
// Windows is not served.
func load() error { return ErrUnavailable }

func version() string                 { return "" }
func newEngine() uintptr              { return 0 }
func freeEngine(uintptr)              {}
func addFont(uintptr, []byte) int     { return 0 }
func setSource(uintptr, string) bool  { return false }
func compile(uintptr) (int32, []byte) { return -1, nil }
func pdf(uintptr) (int32, []byte)     { return -1, nil }
