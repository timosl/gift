package ffmpeg

import (
	"fmt"
	"sync"
	"unsafe"
)

// lib holds the bound functions. Every signature is the C one with int as
// int32, int64_t as int64 and pointers as unsafe.Pointer.
var lib struct {
	once sync.Once
	err  error
	abi  abi

	// Pixel formats are looked up by name at load time: the enum is
	// appended to between majors and its values are not part of what the
	// ABI table promises.
	pixYUV420P, pixYUVJ420P, pixNV12, pixYUV420P10, pixDRMPrime int32
	sampleFLT                                                   int32
	hwDRM                                                       int32
	eagain                                                      int32

	utilVersion, codecVersion, formatVersion func() uint32

	frameAlloc      func() unsafe.Pointer
	frameFree       func(f *unsafe.Pointer)
	frameUnref      func(f unsafe.Pointer)
	strerror        func(code int32, buf unsafe.Pointer, size uintptr) int32
	logSetLevel     func(level int32)
	getPixFmt       func(name string) int32
	getPixFmtName   func(f int32) string
	hwdeviceCreate  func(ctx *unsafe.Pointer, typ int32, device unsafe.Pointer, opts unsafe.Pointer, flags int32) int32
	hwdeviceFind    func(name string) int32
	bufferRef       func(b unsafe.Pointer) unsafe.Pointer
	bufferUnref     func(b *unsafe.Pointer)
	hwframeTransfer func(dst, src unsafe.Pointer, flags int32) int32
	optSetInt       func(obj unsafe.Pointer, name string, val int64, flags int32) int32
	chLayoutDefault func(l unsafe.Pointer, n int32)

	openInput      func(ps *unsafe.Pointer, url string, f unsafe.Pointer, opts unsafe.Pointer) int32
	findStreamInfo func(s unsafe.Pointer, opts unsafe.Pointer) int32
	findBestStream func(s unsafe.Pointer, typ, wanted, related int32, dec *unsafe.Pointer, flags int32) int32
	readFrame      func(s, pkt unsafe.Pointer) int32
	seekFile       func(s unsafe.Pointer, stream int32, min, ts, max int64, flags int32) int32
	closeInput     func(ps *unsafe.Pointer)

	allocContext3 func(codec unsafe.Pointer) unsafe.Pointer
	freeContext   func(pc *unsafe.Pointer)
	parToContext  func(c, par unsafe.Pointer) int32
	open2         func(c, codec unsafe.Pointer, opts unsafe.Pointer) int32
	sendPacket    func(c, pkt unsafe.Pointer) int32
	receiveFrame  func(c, f unsafe.Pointer) int32
	flushBuffers  func(c unsafe.Pointer)
	packetAlloc   func() unsafe.Pointer
	packetFree    func(p *unsafe.Pointer)
	packetUnref   func(p unsafe.Pointer)
	packetMoveRef func(dst, src unsafe.Pointer)
	codecName     func(id int32) string
	findDecoder   func(id int32) unsafe.Pointer
	dictGet       func(m unsafe.Pointer, key string, prev unsafe.Pointer, flags int32) unsafe.Pointer
	codecID       func(c unsafe.Pointer) int32

	swrAllocSetOpts2 func(ps *unsafe.Pointer, outLayout unsafe.Pointer, outFmt, outRate int32, inLayout unsafe.Pointer, inFmt, inRate, logOffset int32, logCtx unsafe.Pointer) int32
	swrInit          func(s unsafe.Pointer) int32
	swrConvert       func(s unsafe.Pointer, out unsafe.Pointer, outCount int32, in unsafe.Pointer, inCount int32) int32
	swrFree          func(ps *unsafe.Pointer)
}

func load() error {
	lib.once.Do(func() { lib.err = bind() })
	return lib.err
}

// averror turns a negative FFmpeg return code into an error.
func averror(op string, code int32) error {
	var buf [128]byte
	lib.strerror(code, unsafe.Pointer(&buf[0]), uintptr(len(buf)))
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return fmt.Errorf("ffmpeg: %s: %s (%d)", op, buf[:n], code)
}

// averrorEOF is AVERROR_EOF, FFERRTAG('E','O','F',' ').
const averrorEOF = -int32('E' | 'O'<<8 | 'F'<<16 | ' '<<24)

func i32(p unsafe.Pointer, off uintptr) int32 { return *(*int32)(unsafe.Add(p, off)) }
func i64(p unsafe.Pointer, off uintptr) int64 { return *(*int64)(unsafe.Add(p, off)) }
func ptr(p unsafe.Pointer, off uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Add(p, off))
}

// rational reads an AVRational, two ints.
func rational(p unsafe.Pointer, off uintptr) (int64, int64) {
	return int64(i32(p, off)), int64(i32(p, off+4))
}
