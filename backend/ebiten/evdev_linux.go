//go:build linux

package ebiten

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// evdevPanel reads a multitouch panel from /dev/input; see touchpanel.go.
//
// One goroutine finds the panel, reads it and decodes it, and queues the
// events for the bridge, which drains them once per tick. A panel that is not
// there yet is looked for every two seconds, so one that is plugged in later,
// or the virtual one of cmd/gift-touchinject, is found.
type evdevPanel struct {
	path   string // a fixed device, or empty to search
	start  time.Time
	logger *slog.Logger
	wake   func()

	mu   sync.Mutex
	q    []panelEvent
	down int
	// last is the bridge time of the last contact activity, in nanoseconds.
	last atomic.Int64

	f      atomic.Pointer[os.File]
	closed atomic.Bool
}

// openPanel starts reading the panel at path, or the first one found when
// path is empty. wake is called from the reader when a finger comes down on
// an idle panel.
func openPanel(path string, start time.Time, logger *slog.Logger, wake func()) panel {
	p := &evdevPanel{path: path, start: start, logger: logger, wake: wake}
	go p.run()
	return p
}

func (p *evdevPanel) drain(dst []panelEvent) []panelEvent {
	p.mu.Lock()
	dst = append(dst, p.q...)
	p.q = p.q[:0]
	p.mu.Unlock()
	return dst
}

func (p *evdevPanel) recent(now, d time.Duration) bool {
	p.mu.Lock()
	down := p.down
	p.mu.Unlock()
	if down > 0 {
		return true
	}
	last := p.last.Load()
	return last != 0 && now-time.Duration(last) < d
}

func (p *evdevPanel) close() {
	p.closed.Store(true)
	if f := p.f.Load(); f != nil {
		_ = f.Close()
	}
}

func (p *evdevPanel) run() {
	for !p.closed.Load() {
		path := p.path
		if path == "" {
			path = findPanel()
		}
		if path == "" {
			time.Sleep(2 * time.Second)
			continue
		}
		err := p.read(path)
		if p.closed.Load() {
			return
		}
		if p.logger != nil {
			p.logger.Info("touch panel gone", slog.String("device", path), slog.Any("err", err))
		}
		p.mu.Lock()
		p.down = 0
		p.mu.Unlock()
		time.Sleep(2 * time.Second)
	}
}

// read decodes the device at path until it fails.
func (p *evdevPanel) read(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	p.f.Store(f)
	defer func() { p.f.Store(nil); _ = f.Close() }()
	fd := f.Fd()
	// Monotonic time stamps rather than the wall clock the kernel uses by
	// default, so that they convert into the bridge's clock by an offset.
	// First, because the kernel flushes a queue that already holds events
	// when the clock changes, and says so with SYN_DROPPED.
	clk := int32(unix.CLOCK_MONOTONIC)
	if err := ioctl(fd, eviocsclockid, unsafe.Pointer(&clk)); err != nil {
		return err
	}
	var d mtDecoder
	var ax, ay absInfo
	if err := ioctl(fd, eviocgabs(absMTPositionX), unsafe.Pointer(&ax)); err != nil {
		return err
	}
	if err := ioctl(fd, eviocgabs(absMTPositionY), unsafe.Pointer(&ay)); err != nil {
		return err
	}
	var as absInfo
	if ioctl(fd, eviocgabs(absMTSlot), unsafe.Pointer(&as)) == nil {
		d.slot = int(as.value)
	}
	d.minX, d.maxX, d.minY, d.maxY = ax.min, ax.max, ay.min, ay.max
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return err
	}
	// Go's monotonic clock is CLOCK_MONOTONIC, so the bridge's clock,
	// time.Since(start), differs from the kernel's by a constant.
	base := time.Since(p.start) - time.Duration(ts.Nano())
	if p.logger != nil {
		p.logger.Info("touch panel", slog.String("device", path), slog.String("name", deviceName(fd)),
			slog.Int("x_max", int(ax.max)), slog.Int("y_max", int(ay.max)))
	}

	const size = 24 // struct input_event on a 64 bit kernel
	buf := make([]byte, size*64)
	var out []panelEvent
	for {
		n, err := f.Read(buf)
		if err != nil {
			return err
		}
		out = out[:0]
		for off := 0; off+size <= n; off += size {
			e := buf[off : off+size]
			sec := int64(binary.LittleEndian.Uint64(e[0:]))
			usec := int64(binary.LittleEndian.Uint64(e[8:]))
			typ := binary.LittleEndian.Uint16(e[16:])
			code := binary.LittleEndian.Uint16(e[18:])
			val := int32(binary.LittleEndian.Uint32(e[20:]))
			t := time.Duration(sec)*time.Second + time.Duration(usec)*time.Microsecond + base
			out = d.feed(typ, code, val, t, out)
		}
		if len(out) == 0 {
			continue
		}
		p.mu.Lock()
		wasIdle := p.down == 0
		for _, e := range out {
			switch e.kind {
			case panelDown:
				p.down++
			case panelUp:
				p.down = max(0, p.down-1)
			}
		}
		p.q = append(p.q, out...)
		p.mu.Unlock()
		p.last.Store(int64(out[len(out)-1].t))
		if wasIdle && p.wake != nil {
			p.wake()
		}
	}
}

// findPanel returns the first input device that is a direct touch device —
// a touch screen, not a touch pad — with the multitouch protocol B, or "".
func findPanel() string {
	paths, _ := filepath.Glob("/dev/input/event*")
	sort.Strings(paths)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		ok := isPanel(f.Fd())
		_ = f.Close()
		if ok {
			return path
		}
	}
	return ""
}

const inputPropDirect = 0x01

func isPanel(fd uintptr) bool {
	var props [8]byte
	if ioctl(fd, ioc(2, 'E', 0x09, len(props)), unsafe.Pointer(&props[0])) != nil {
		return false
	}
	if props[inputPropDirect/8]&(1<<(inputPropDirect%8)) == 0 {
		return false
	}
	var abs [8]byte
	if ioctl(fd, ioc(2, 'E', 0x20+evAbs, len(abs)), unsafe.Pointer(&abs[0])) != nil {
		return false
	}
	has := func(code int) bool { return abs[code/8]&(1<<(code%8)) != 0 }
	return has(absMTSlot) && has(absMTPositionX) && has(absMTPositionY) && has(absMTTrackingID)
}

func deviceName(fd uintptr) string {
	var name [128]byte
	if ioctl(fd, ioc(2, 'E', 0x06, len(name)), unsafe.Pointer(&name[0])) != nil {
		return ""
	}
	for i, c := range name {
		if c == 0 {
			return string(name[:i])
		}
	}
	return string(name[:])
}

// absInfo is struct input_absinfo.
type absInfo struct {
	value, min, max, fuzz, flat, resolution int32
}

// ioc is the _IOC macro of the kernel.
func ioc(dir, typ, nr, size int) uintptr {
	return uintptr(dir<<30 | size<<16 | typ<<8 | nr)
}

func eviocgabs(code int) uintptr { return ioc(2, 'E', 0x40+code, int(unsafe.Sizeof(absInfo{}))) }

var eviocsclockid = ioc(1, 'E', 0xa0, 4)

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	if errno != 0 {
		return errors.New(errno.Error())
	}
	return nil
}
