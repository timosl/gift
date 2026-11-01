//go:build darwin || freebsd || linux

package typst

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// lib holds the bound functions of libgifttypst. usize is uint, which is the
// width of a pointer on every platform purego runs on.
var lib struct {
	once sync.Once
	err  error
	path string

	version    func() unsafe.Pointer
	newEngine  func() uintptr
	freeEngine func(e uintptr)
	addFont    func(e uintptr, data unsafe.Pointer, n uint) int32
	setSource  func(e uintptr, text unsafe.Pointer, n uint) int32
	compile    func(e uintptr, out *unsafe.Pointer, n *uint) int32
	pdf        func(e uintptr, out *unsafe.Pointer, n *uint) int32
	free       func(p unsafe.Pointer, n uint)
}

// candidates are the places libgifttypst is looked for, in order: the path
// [Open] was given, the environment variable GIFT_TYPST_LIB, the directory of
// the executable, and the dynamic linker's own search path.
func candidates() []string {
	name := "libgifttypst.so"
	if runtime.GOOS == "darwin" {
		name = "libgifttypst.dylib"
	}
	var c []string
	if lib.path != "" {
		c = append(c, lib.path)
	}
	if p := os.Getenv("GIFT_TYPST_LIB"); p != "" {
		c = append(c, p)
	}
	if exe, err := os.Executable(); err == nil {
		c = append(c, filepath.Join(filepath.Dir(exe), name))
	}
	return append(c, name)
}

func load() error {
	lib.once.Do(func() { lib.err = bind() })
	return lib.err
}

func bind() (err error) {
	var h uintptr
	for _, name := range candidates() {
		if h, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL); err == nil {
			break
		}
	}
	if h == 0 {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: binding failed: %v", ErrUnavailable, r)
		}
	}()
	purego.RegisterLibFunc(&lib.version, h, "gt_version")
	purego.RegisterLibFunc(&lib.newEngine, h, "gt_new")
	purego.RegisterLibFunc(&lib.freeEngine, h, "gt_free_engine")
	purego.RegisterLibFunc(&lib.addFont, h, "gt_add_font")
	purego.RegisterLibFunc(&lib.setSource, h, "gt_set_source")
	purego.RegisterLibFunc(&lib.compile, h, "gt_compile")
	purego.RegisterLibFunc(&lib.pdf, h, "gt_pdf")
	purego.RegisterLibFunc(&lib.free, h, "gt_free")
	return nil
}

func version() string { return cString(lib.version()) }

func newEngine() uintptr { return lib.newEngine() }

func freeEngine(h uintptr) { lib.freeEngine(h) }

func addFont(h uintptr, data []byte) int {
	n := int(lib.addFont(h, bytesPtr(data), uint(len(data))))
	runtime.KeepAlive(data)
	return n
}

func setSource(h uintptr, src string) bool {
	ok := lib.setSource(h, stringPtr(src), uint(len(src))) == 0
	runtime.KeepAlive(src)
	return ok
}

// call runs one of the functions that hand back a buffer and copies the
// buffer into Go memory before giving it back to Rust.
func call(f func(uintptr, *unsafe.Pointer, *uint) int32, h uintptr) (int32, []byte) {
	var p unsafe.Pointer
	var n uint
	code := f(h, &p, &n)
	if p == nil {
		return code, nil
	}
	b := make([]byte, n)
	copy(b, unsafe.Slice((*byte)(p), n))
	lib.free(p, n)
	return code, b
}

func compile(h uintptr) (int32, []byte) { return call(lib.compile, h) }

func pdf(h uintptr) (int32, []byte) { return call(lib.pdf, h) }

// zero is what an empty slice or string points to: Rust requires a non null
// pointer even for a length of zero.
var zero byte

func bytesPtr(b []byte) unsafe.Pointer {
	if len(b) == 0 {
		return unsafe.Pointer(&zero)
	}
	return unsafe.Pointer(&b[0])
}

func stringPtr(s string) unsafe.Pointer {
	if len(s) == 0 {
		return unsafe.Pointer(&zero)
	}
	return unsafe.Pointer(unsafe.StringData(s))
}

func cString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}
