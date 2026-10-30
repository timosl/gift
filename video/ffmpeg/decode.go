package ffmpeg

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

// ErrAgain is returned by Send when the decoder has output waiting that has to
// be received first, and by Receive when it needs more input.
var ErrAgain = errors.New("ffmpeg: again")

// Layout is how a [Frame] packs its planes into [Frame.Pix].
//
// Every layout is Y on top, Height rows of Stride bytes, and chroma below it,
// (Height+1)/2 rows of Stride bytes with U and V interleaved. Read as RGBA
// texels, that is one texture of Stride/4 by Height+(Height+1)/2, uploaded in
// one call; the shader takes the bytes back apart.
//
// Chroma is always interleaved, also where the decoder delivers it planar:
// one texel fetch then yields both chroma samples, and on the GPU of a
// Raspberry Pi 5 the fetches are what the conversion costs. Interleaving on
// the CPU is a copy loop over a quarter of the picture.
type Layout uint8

const (
	// LayoutNV12 is 8 bit 4:2:0.
	LayoutNV12 Layout = iota + 1
	// LayoutP016 is 10 bit 4:2:0 in 16 bit little endian samples, so a
	// chroma texel is U low, U high, V low, V high.
	LayoutP016
)

// Matrix is the YUV to RGB matrix a frame was encoded with.
type Matrix uint8

const (
	BT601 Matrix = iota
	BT709
	BT2020
)

// Frame is one decoded picture.
type Frame struct {
	PTS      time.Duration
	Duration time.Duration

	Width, Height  int
	SARNum, SARDen int

	Layout    Layout
	Matrix    Matrix
	FullRange bool

	Interlaced    bool
	TopFieldFirst bool

	// Stride is the length of one row of Pix in bytes, a multiple of 8.
	Stride int
	// Pix holds the planes; see [Layout]. Its backing array is reused by
	// the next Receive into the same Frame.
	Pix []byte
}

// TextureSize is the size of Pix read as RGBA texels.
func (f *Frame) TextureSize() (w, h int) { return f.Stride / 4, f.Height + (f.Height+1)/2 }

// VideoDecoder decodes the video stream of a [File]. It belongs to one
// goroutine at a time.
type VideoDecoder struct {
	ctx       unsafe.Pointer
	frame     unsafe.Pointer
	sw        unsafe.Pointer
	hwDev     unsafe.Pointer
	tb        [2]int64
	codecName string
	hwName    string
}

func newVideoDecoder(codec, par unsafe.Pointer, tb [2]int64, opt Options) (*VideoDecoder, error) {
	d := &VideoDecoder{tb: tb}
	d.codecName = lib.codecName(i32(par, 4))
	d.ctx = lib.allocContext3(codec)
	if d.ctx == nil {
		return nil, errors.New("ffmpeg: avcodec_alloc_context3 failed")
	}
	if rc := lib.parToContext(d.ctx, par); rc < 0 {
		d.close()
		return nil, averror("parameters to context", rc)
	}
	a := &lib.abi
	*(*[2]int32)(unsafe.Add(d.ctx, a.ctxPktTimebase)) = [2]int32{int32(tb[0]), int32(tb[1])}
	lib.optSetInt(d.ctx, "threads", int64(opt.Threads), 0)

	// The default get_format of libavcodec picks the hardware format by
	// itself once a device is attached, so no callback into Go is needed,
	// and purego callbacks from FFmpeg's decoder threads are best avoided.
	if opt.HW && d.codecName == "hevc" && lib.hwDRM > 0 {
		var dev unsafe.Pointer
		if lib.hwdeviceCreate(&dev, lib.hwDRM, nil, nil, 0) >= 0 {
			d.hwDev = dev
			*(*unsafe.Pointer)(unsafe.Add(d.ctx, a.ctxHWDeviceCtx)) = lib.bufferRef(dev)
			d.hwName = "drm"
		}
	}
	if rc := lib.open2(d.ctx, codec, nil); rc < 0 {
		d.close()
		return nil, averror("open video decoder", rc)
	}
	d.frame = lib.frameAlloc()
	d.sw = lib.frameAlloc()
	return d, nil
}

// Codec is the name of the codec, "h264" or "hevc".
func (d *VideoDecoder) Codec() string { return d.codecName }

// HW is the name of the hardware device decoding is attached to, or "".
func (d *VideoDecoder) HW() string { return d.hwName }

// Send hands one packet to the decoder and takes ownership of it. A nil packet
// starts draining at the end of the file. On [ErrAgain] the packet is not
// consumed; the caller receives frames and sends it again.
func (d *VideoDecoder) Send(p *Packet) error {
	var pp unsafe.Pointer
	if p != nil {
		pp = p.p
	}
	switch rc := lib.sendPacket(d.ctx, pp); {
	case rc == lib.eagain:
		return ErrAgain
	case rc == averrorEOF:
		return ErrEOF
	case rc < 0:
		return averror("send video packet", rc)
	}
	return nil
}

// Receive decodes the next frame into dst, reusing its buffer.
func (d *VideoDecoder) Receive(dst *Frame) error {
	switch rc := lib.receiveFrame(d.ctx, d.frame); {
	case rc == lib.eagain:
		return ErrAgain
	case rc == averrorEOF:
		return ErrEOF
	case rc < 0:
		return averror("receive video frame", rc)
	}
	defer lib.frameUnref(d.frame)
	src := d.frame
	if i32(src, lib.abi.frFormat) == lib.pixDRMPrime {
		if rc := lib.hwframeTransfer(d.sw, src, 0); rc < 0 {
			return averror("transfer hardware frame", rc)
		}
		defer lib.frameUnref(d.sw)
		src = d.sw
	}
	return d.fill(dst, d.frame, src)
}

// fill copies the planes of src into dst. Timing comes from meta, which is the
// decoded frame; a transferred hardware frame carries the pixels only.
func (d *VideoDecoder) fill(dst *Frame, meta, src unsafe.Pointer) error {
	a := &lib.abi
	pts := i64(meta, a.frBestEffort)
	if pts == noPTS {
		pts = i64(meta, a.frPTS)
	}
	dst.PTS = toDuration(pts, d.tb)
	dst.Duration = toDuration(i64(meta, a.frDuration), d.tb)
	n, dd := rational(meta, a.frSAR)
	dst.SARNum, dst.SARDen = int(n), int(dd)
	if a.frInterlaced != 0 {
		dst.Interlaced = i32(meta, a.frInterlaced) != 0
		dst.TopFieldFirst = i32(meta, a.frTopFirst) != 0
	} else {
		flags := i32(meta, a.frFlags)
		dst.Interlaced = flags&frameFlagInterlaced != 0
		dst.TopFieldFirst = flags&frameFlagTopFirst != 0
	}

	w, h := int(i32(src, a.frWidth)), int(i32(src, a.frHeight))
	dst.Width, dst.Height = w, h
	switch i32(meta, a.frColorspace) {
	case 1: // AVCOL_SPC_BT709
		dst.Matrix = BT709
	case 9, 10: // AVCOL_SPC_BT2020_NCL, _CL
		dst.Matrix = BT2020
	case 5, 6: // AVCOL_SPC_BT470BG, AVCOL_SPC_SMPTE170M
		dst.Matrix = BT601
	default: // unspecified: guess from the size, as players do
		dst.Matrix = BT709
		if h <= 576 {
			dst.Matrix = BT601
		}
	}
	format := i32(src, a.frFormat)
	dst.FullRange = i32(meta, a.frColorRange) == 2 || format == lib.pixYUVJ420P

	plane := func(i int) (unsafe.Pointer, int) {
		return ptr(src, a.frData+uintptr(i)*8), int(i32(src, a.frLinesize+uintptr(i)*4))
	}
	cw, ch := (w+1)/2, (h+1)/2
	switch format {
	case lib.pixYUV420P, lib.pixYUVJ420P:
		dst.Layout = LayoutNV12
		dst.alloc(roundUp(w, 8))
		y, ys := plane(0)
		u, us := plane(1)
		v, vs := plane(2)
		copyPlane(dst.Pix, dst.Stride, 0, y, ys, w, h)
		interleave8(dst.Pix[h*dst.Stride:], dst.Stride, u, us, v, vs, cw, ch)
	case lib.pixNV12:
		dst.Layout = LayoutNV12
		dst.alloc(roundUp(w, 8))
		y, ys := plane(0)
		uv, uvs := plane(1)
		copyPlane(dst.Pix, dst.Stride, 0, y, ys, w, h)
		copyPlane(dst.Pix, dst.Stride, h*dst.Stride, uv, uvs, 2*cw, ch)
	case lib.pixYUV420P10:
		dst.Layout = LayoutP016
		dst.alloc(roundUp(2*w, 16))
		y, ys := plane(0)
		u, us := plane(1)
		v, vs := plane(2)
		copyPlane(dst.Pix, dst.Stride, 0, y, ys, 2*w, h)
		interleave16(dst.Pix[h*dst.Stride:], dst.Stride, u, us, v, vs, cw, ch)
	default:
		return fmt.Errorf("ffmpeg: pixel format %s is not supported", lib.getPixFmtName(format))
	}
	runtime.KeepAlive(src)
	return nil
}

func (f *Frame) alloc(stride int) {
	f.Stride = stride
	n := stride * (f.Height + (f.Height+1)/2)
	if cap(f.Pix) < n {
		f.Pix = make([]byte, n)
	}
	f.Pix = f.Pix[:n]
}

func copyPlane(dst []byte, dstStride, off int, src unsafe.Pointer, srcStride, rowBytes, rows int) {
	for r := 0; r < rows; r++ {
		s := unsafe.Slice((*byte)(unsafe.Add(src, r*srcStride)), rowBytes)
		copy(dst[off+r*dstStride:off+r*dstStride+rowBytes], s)
	}
}

// interleave8 writes rows of U0 V0 U1 V1 ... from two planes.
func interleave8(dst []byte, stride int, u unsafe.Pointer, us int, v unsafe.Pointer, vs int, cw, ch int) {
	for r := 0; r < ch; r++ {
		ur := unsafe.Slice((*byte)(unsafe.Add(u, r*us)), cw)
		vr := unsafe.Slice((*byte)(unsafe.Add(v, r*vs)), cw)
		d := dst[r*stride : r*stride+2*cw]
		for i := range cw {
			d[2*i] = ur[i]
			d[2*i+1] = vr[i]
		}
	}
}

// interleave16 is interleave8 for two byte samples.
func interleave16(dst []byte, stride int, u unsafe.Pointer, us int, v unsafe.Pointer, vs int, cw, ch int) {
	for r := 0; r < ch; r++ {
		ur := unsafe.Slice((*uint16)(unsafe.Add(u, r*us)), cw)
		vr := unsafe.Slice((*uint16)(unsafe.Add(v, r*vs)), cw)
		d := unsafe.Slice((*uint16)(unsafe.Pointer(&dst[r*stride])), 2*cw)
		for i := range cw {
			d[2*i] = ur[i]
			d[2*i+1] = vr[i]
		}
	}
}

func roundUp(n, m int) int { return (n + m - 1) / m * m }

// Flush drops everything buffered in the decoder, after a seek.
func (d *VideoDecoder) Flush() { lib.flushBuffers(d.ctx) }

func (d *VideoDecoder) close() {
	if d.frame != nil {
		lib.frameFree(&d.frame)
	}
	if d.sw != nil {
		lib.frameFree(&d.sw)
	}
	if d.ctx != nil {
		lib.freeContext(&d.ctx)
	}
	if d.hwDev != nil {
		lib.bufferUnref(&d.hwDev)
	}
}

// AudioDecoder decodes the audio stream of a [File] to interleaved float32
// stereo. It belongs to one goroutine at a time.
type AudioDecoder struct {
	ctx       unsafe.Pointer
	frame     unsafe.Pointer
	swr       unsafe.Pointer
	tb        [2]int64
	rate      int
	wantRate  atomic.Int32 // set by SetRate from another goroutine
	inRate    int
	codecName string
	// outLayout is an AVChannelLayout. It holds a pointer only for custom
	// orders, and stereo is a native one, so it may live in Go memory.
	outLayout [3]uint64
}

func newAudioDecoder(codec, par unsafe.Pointer, tb [2]int64, rate int) (*AudioDecoder, error) {
	d := &AudioDecoder{tb: tb, rate: rate}
	d.wantRate.Store(int32(rate))
	d.codecName = lib.codecName(i32(par, 4))
	d.ctx = lib.allocContext3(codec)
	if d.ctx == nil {
		return nil, errors.New("ffmpeg: avcodec_alloc_context3 failed")
	}
	if rc := lib.parToContext(d.ctx, par); rc < 0 {
		d.close()
		return nil, averror("parameters to context", rc)
	}
	*(*[2]int32)(unsafe.Add(d.ctx, lib.abi.ctxPktTimebase)) = [2]int32{int32(tb[0]), int32(tb[1])}
	if rc := lib.open2(d.ctx, codec, nil); rc < 0 {
		d.close()
		return nil, averror("open audio decoder", rc)
	}
	lib.chLayoutDefault(unsafe.Pointer(&d.outLayout), 2)
	d.frame = lib.frameAlloc()
	return d, nil
}

// Codec is the name of the codec.
func (d *AudioDecoder) Codec() string { return d.codecName }

// Send is [VideoDecoder.Send] for audio.
func (d *AudioDecoder) Send(p *Packet) error {
	var pp unsafe.Pointer
	if p != nil {
		pp = p.p
	}
	switch rc := lib.sendPacket(d.ctx, pp); {
	case rc == lib.eagain:
		return ErrAgain
	case rc == averrorEOF:
		return ErrEOF
	case rc < 0:
		return averror("send audio packet", rc)
	}
	return nil
}

// Receive decodes the next frame, appends its samples to buf as interleaved
// stereo float32 at the output rate and returns the extended slice with the
// presentation time of the first appended sample.
func (d *AudioDecoder) Receive(buf []float32) ([]float32, time.Duration, error) {
	switch rc := lib.receiveFrame(d.ctx, d.frame); {
	case rc == lib.eagain:
		return buf, 0, ErrAgain
	case rc == averrorEOF:
		return buf, 0, ErrEOF
	case rc < 0:
		return buf, 0, averror("receive audio frame", rc)
	}
	defer lib.frameUnref(d.frame)
	a := &lib.abi
	f := d.frame
	if r := int(d.wantRate.Load()); r != d.rate {
		d.rate = r
		if d.swr != nil {
			lib.swrFree(&d.swr)
		}
	}
	if d.swr == nil {
		d.inRate = int(i32(f, a.frSampleRate))
		rc := lib.swrAllocSetOpts2(&d.swr, unsafe.Pointer(&d.outLayout), lib.sampleFLT, int32(d.rate),
			unsafe.Add(f, a.frChLayout), i32(f, a.frFormat), int32(d.inRate), 0, nil)
		if rc < 0 {
			return buf, 0, averror("swr_alloc_set_opts2", rc)
		}
		if rc := lib.swrInit(d.swr); rc < 0 {
			lib.swrFree(&d.swr)
			return buf, 0, averror("swr_init", rc)
		}
	}
	pts := i64(f, a.frBestEffort)
	if pts == noPTS {
		pts = i64(f, a.frPTS)
	}
	nb := int(i32(f, a.frNbSamples))
	room := nb*d.rate/max(d.inRate, 1) + 256
	if cap(buf)-len(buf) < 2*room {
		nbuf := make([]float32, len(buf), 2*(len(buf)+2*room))
		copy(nbuf, buf)
		buf = nbuf
	}
	out := [1]unsafe.Pointer{unsafe.Pointer(unsafe.SliceData(buf[len(buf):cap(buf)]))}
	var pin runtime.Pinner
	pin.Pin(out[0])
	n := lib.swrConvert(d.swr, unsafe.Pointer(&out), int32(room), ptr(f, a.frExtendedData), int32(nb))
	pin.Unpin()
	if n < 0 {
		return buf, 0, averror("swr_convert", n)
	}
	return buf[:len(buf)+2*int(n)], toDuration(pts, d.tb), nil
}

// SetRate changes the output sample rate from the next frame on. It may be
// called from any goroutine.
func (d *AudioDecoder) SetRate(rate int) { d.wantRate.Store(int32(rate)) }

// Flush drops everything buffered in the decoder and the resampler.
func (d *AudioDecoder) Flush() {
	lib.flushBuffers(d.ctx)
	if d.swr != nil {
		lib.swrFree(&d.swr)
	}
}

func (d *AudioDecoder) close() {
	if d.frame != nil {
		lib.frameFree(&d.frame)
	}
	if d.swr != nil {
		lib.swrFree(&d.swr)
	}
	if d.ctx != nil {
		lib.freeContext(&d.ctx)
	}
}
