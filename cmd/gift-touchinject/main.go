//go:build linux

// Command gift-touchinject plays touch gestures on a virtual touch screen, for
// testing multitouch on a Linux machine that has no touch screen.
//
// It creates a multitouch device through uinput — a direct device with the
// kernel's multitouch protocol B, like the panel of a Raspberry Pi display —
// waits for the compositor and the application to find it, plays the
// gestures and removes the device. The compositor maps it onto the screen and
// turns the first finger into a mouse for X11 clients, exactly as with a real
// panel, so what is tested is the whole path including the emulated mouse a
// gift application has to ignore.
//
// /dev/uinput belongs to root, so it runs with sudo:
//
//	sudo gift-touchinject -hz 50 swipe:1500,540,300,540,200 pinch:960,540,200,700,400
//
// A gesture is a name and its numbers, in screen pixels and milliseconds:
//
//	tap:x,y                      one finger down and up
//	doubletap:x,y                two taps 120 ms apart
//	swipe:x0,y0,x1,y1,ms         one finger from one point to another
//	pinch:cx,cy,d0,d1,ms         two fingers on a horizontal line through
//	                             (cx, cy), from d0 apart to d1 apart
//	wait:ms                      nothing
//
// -hz is the sample rate of the panel; 50 is what the drivers of the
// Raspberry Pi touch displays produce.
package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	width  = flag.Int("width", 1920, "screen width the panel covers")
	height = flag.Int("height", 1080, "screen height the panel covers")
	hz     = flag.Int("hz", 50, "panel sample rate")
	settle = flag.Duration("settle", 3*time.Second, "time for the compositor and the application to find the device")
)

func main() {
	flag.Parse()
	if err := run(flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "gift-touchinject:", err)
		os.Exit(1)
	}
}

// The uinput and input constants; see linux/uinput.h and
// linux/input-event-codes.h.
const (
	evSyn = 0x00
	evKey = 0x01
	evAbs = 0x03

	synReport = 0
	btnTouch  = 0x14a

	absX            = 0x00
	absY            = 0x01
	absMTSlot       = 0x2f
	absMTPositionX  = 0x35
	absMTPositionY  = 0x36
	absMTTrackingID = 0x39

	inputPropDirect = 0x01
	busVirtual      = 0x06
)

func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

var (
	uiSetEvBit   = ioc(1, 'U', 100, 4)
	uiSetKeyBit  = ioc(1, 'U', 101, 4)
	uiSetAbsBit  = ioc(1, 'U', 103, 4)
	uiSetPropBit = ioc(1, 'U', 110, 4)
	uiDevSetup   = ioc(1, 'U', 3, 92)
	uiAbsSetup   = ioc(1, 'U', 4, 28)
	uiDevCreate  = ioc(0, 'U', 1, 0)
	uiDevDestroy = ioc(0, 'U', 2, 0)
)

type device struct{ f *os.File }

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

func create() (*device, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	fd := f.Fd()
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	must(ioctl(fd, uiSetEvBit, evKey))
	must(ioctl(fd, uiSetEvBit, evAbs))
	must(ioctl(fd, uiSetKeyBit, btnTouch))
	must(ioctl(fd, uiSetPropBit, inputPropDirect))
	for _, a := range []uintptr{absX, absY, absMTSlot, absMTPositionX, absMTPositionY, absMTTrackingID} {
		must(ioctl(fd, uiSetAbsBit, a))
	}
	abs := func(code uint16, lo, hi int32) {
		var b [28]byte
		binary.LittleEndian.PutUint16(b[0:], code)
		binary.LittleEndian.PutUint32(b[8:], uint32(lo))
		binary.LittleEndian.PutUint32(b[12:], uint32(hi))
		must(ioctl(fd, uiAbsSetup, uintptr(unsafe.Pointer(&b[0]))))
	}
	w, h := int32(*width-1), int32(*height-1)
	abs(absX, 0, w)
	abs(absY, 0, h)
	abs(absMTPositionX, 0, w)
	abs(absMTPositionY, 0, h)
	abs(absMTSlot, 0, 9)
	abs(absMTTrackingID, 0, 65535)
	var setup [92]byte
	binary.LittleEndian.PutUint16(setup[0:], busVirtual)
	binary.LittleEndian.PutUint16(setup[2:], 0x6769) // "gi"
	binary.LittleEndian.PutUint16(setup[4:], 0x6674) // "ft"
	copy(setup[8:], "gift virtual touch screen")
	must(ioctl(fd, uiDevSetup, uintptr(unsafe.Pointer(&setup[0]))))
	must(ioctl(fd, uiDevCreate, 0))
	return &device{f: f}, err
}

func (d *device) close() {
	_ = ioctl(d.f.Fd(), uiDevDestroy, 0)
	_ = d.f.Close()
}

func (d *device) emit(typ, code uint16, value int32) {
	var b [24]byte
	binary.LittleEndian.PutUint16(b[16:], typ)
	binary.LittleEndian.PutUint16(b[18:], code)
	binary.LittleEndian.PutUint32(b[20:], uint32(value))
	_, _ = d.f.Write(b[:])
}

// finger is one contact of a frame: its slot, and where it is, or up.
type finger struct {
	slot int32
	x, y int32
	up   bool
}

var nextID int32 = 1
var live = map[int32]int32{}

// frame sends one sample of every finger and ends it.
func (d *device) frame(fs ...finger) {
	for _, f := range fs {
		d.emit(evAbs, absMTSlot, f.slot)
		if f.up {
			d.emit(evAbs, absMTTrackingID, -1)
			delete(live, f.slot)
			continue
		}
		if _, ok := live[f.slot]; !ok {
			live[f.slot] = nextID
			d.emit(evAbs, absMTTrackingID, nextID)
			nextID++
		}
		d.emit(evAbs, absMTPositionX, f.x)
		d.emit(evAbs, absMTPositionY, f.y)
	}
	d.emit(evKey, btnTouch, boolInt(len(live) > 0))
	if len(live) > 0 {
		x, y := fs[0].x, fs[0].y
		d.emit(evAbs, absX, x)
		d.emit(evAbs, absY, y)
	}
	d.emit(evSyn, synReport, 0)
}

func boolInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func run(gestures []string) error {
	if len(gestures) == 0 {
		return errors.New("no gestures; see the package documentation")
	}
	d, err := create()
	if err != nil {
		return fmt.Errorf("creating the device (run with sudo): %w", err)
	}
	defer d.close()
	time.Sleep(*settle)
	period := time.Second / time.Duration(max(1, *hz))
	for _, g := range gestures {
		name, args, _ := strings.Cut(g, ":")
		n, err := numbers(args)
		if err != nil {
			return fmt.Errorf("%s: %w", g, err)
		}
		switch {
		case name == "tap" && len(n) == 2:
			d.frame(finger{slot: 0, x: n[0], y: n[1]})
			time.Sleep(60 * time.Millisecond)
			d.frame(finger{slot: 0, up: true})
		case name == "doubletap" && len(n) == 2:
			for range 2 {
				d.frame(finger{slot: 0, x: n[0], y: n[1]})
				time.Sleep(50 * time.Millisecond)
				d.frame(finger{slot: 0, up: true})
				time.Sleep(120 * time.Millisecond)
			}
		case name == "swipe" && len(n) == 5:
			steps := max(1, int(time.Duration(n[4])*time.Millisecond/period))
			for i := 0; i <= steps; i++ {
				f := float64(i) / float64(steps)
				d.frame(finger{slot: 0, x: lerp(n[0], n[2], f), y: lerp(n[1], n[3], f)})
				time.Sleep(period)
			}
			d.frame(finger{slot: 0, up: true})
		case name == "pinch" && len(n) == 5:
			steps := max(1, int(time.Duration(n[4])*time.Millisecond/period))
			for i := 0; i <= steps; i++ {
				f := float64(i) / float64(steps)
				half := lerp(n[2], n[3], f) / 2
				d.frame(finger{slot: 0, x: n[0] - half, y: n[1]}, finger{slot: 1, x: n[0] + half, y: n[1]})
				time.Sleep(period)
			}
			d.frame(finger{slot: 0, up: true}, finger{slot: 1, up: true})
		case name == "wait" && len(n) == 1:
			time.Sleep(time.Duration(n[0]) * time.Millisecond)
		default:
			return fmt.Errorf("unknown gesture %q", g)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func numbers(s string) ([]int32, error) {
	if s == "" {
		return nil, nil
	}
	var out []int32
	for _, p := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		out = append(out, int32(v))
	}
	return out, nil
}

func lerp(a, b int32, f float64) int32 { return a + int32(float64(b-a)*f) }
