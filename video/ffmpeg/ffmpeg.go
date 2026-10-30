// Package ffmpeg demultiplexes and decodes video and audio through the FFmpeg
// libraries of the system, loaded at run time with purego.
//
// It is the counterpart of asset/turbojpeg for moving pictures: gift builds
// without cgo, and so does this package. libavformat, libavcodec and
// libswresample are opened with dlopen the first time they are needed; a
// system without them, or with a release whose struct layout this package
// has no table for, reports [ErrUnavailable] and nothing else breaks.
//
// # What it hands out
//
// A [VideoDecoder] produces [Frame]s whose pixels are still YUV, packed into
// one byte slice that is uploaded as one RGBA texture and converted on the
// GPU. Converting on the CPU would cost a Raspberry Pi 5 several milliseconds
// per full HD frame and send 8 MB instead of 3 MB per frame to the GPU.
//
// An [AudioDecoder] produces interleaved float32 stereo at the rate the caller
// asks for, which is the rate of the one audio context an Ebitengine process
// has.
//
// # Codecs and patents
//
// Nothing here contains a codec. Which formats play is decided by the FFmpeg
// build of the system, and so is the licensing question that comes with
// H.264 and H.265: see the project plan, section 25.
package ffmpeg

import (
	"errors"
	"runtime"
	"time"
	"unsafe"
)

// ErrUnavailable reports that the FFmpeg libraries could not be loaded, or
// that their release has no ABI table in this package.
var ErrUnavailable = errors.New("ffmpeg: the FFmpeg libraries are not available")

// Available reports whether the libraries were found and bound.
func Available() bool { return load() == nil }

// Versions returns the majors of libavutil, libavcodec and libavformat, or
// zeros when the libraries are not loaded.
func Versions() (util, codec, format uint32) {
	if load() != nil {
		return 0, 0, 0
	}
	return lib.utilVersion() >> 16, lib.codecVersion() >> 16, lib.formatVersion() >> 16
}

const (
	mediaVideo = 0 // AVMEDIA_TYPE_VIDEO
	mediaAudio = 1 // AVMEDIA_TYPE_AUDIO

	noPTS = -1 << 63 // AV_NOPTS_VALUE

	frameFlagInterlaced = 1 << 3 // AV_FRAME_FLAG_INTERLACED
	frameFlagTopFirst   = 1 << 4 // AV_FRAME_FLAG_TOP_FIELD_FIRST
)

// Info describes an opened file.
type Info struct {
	Duration time.Duration

	HasVideo   bool
	VideoCodec string
	Width      int
	Height     int
	// SARNum and SARDen are the sample aspect ratio, the shape of one
	// pixel: 16:15 for 4:3 PAL, 64:45 for anamorphic 16:9 PAL, 1:1 for
	// square pixels.
	SARNum, SARDen int
	FrameRate      float64

	HasAudio      bool
	AudioCodec    string
	AudioRate     int
	AudioChannels int
}

// DisplayAspect is the width to height ratio the picture is meant to be shown
// at, which differs from Width/Height for anamorphic material.
func (i Info) DisplayAspect() float64 {
	if i.Height == 0 {
		return 0
	}
	num, den := i.SARNum, i.SARDen
	if num <= 0 || den <= 0 {
		num, den = 1, 1
	}
	return float64(i.Width*num) / float64(i.Height*den)
}

// Options configure [Open].
type Options struct {
	// HW asks for the hardware decoder where the system has one that this
	// package knows: the HEVC block of the Raspberry Pi 5, through the
	// v4l2-request hwaccel of Raspberry Pi OS's FFmpeg. Without one, or
	// for a codec it does not cover, decoding stays in software.
	HW bool
	// Threads is the number of decoder threads; zero lets FFmpeg choose,
	// which is one per core.
	Threads int
	// AudioRate is the output sample rate of the audio decoder. Zero is
	// 48000.
	AudioRate int
}

// File is an opened media file. Its methods are not safe for concurrent use,
// but the decoders it returns may each run on their own goroutine while one
// goroutine reads packets.
type File struct {
	fmt   unsafe.Pointer
	info  Info
	vIdx  int32
	aIdx  int32
	vTB   [2]int64
	aTB   [2]int64
	video *VideoDecoder
	audio *AudioDecoder

	// tracks are the audio streams, track the index of the one playing,
	// and decs the decoders opened so far, by stream; a track that was
	// played once keeps its decoder until Close.
	tracks    []AudioTrack
	track     int
	decs      map[int32]*AudioDecoder
	audioRate int
}

// AudioTrack describes one audio stream of a file.
type AudioTrack struct {
	// Stream is the index of the stream in the file.
	Stream int
	// Codec is the name of the codec, "aac" or "ac3".
	Codec string
	// Language is the ISO 639 code the file declares, "eng" or "deu", or
	// "" when it declares none.
	Language string
	// Title is the title the file gives the stream, or "".
	Title string
	// Channels and SampleRate are what the stream carries; the decoder
	// mixes and resamples to stereo at the output rate either way.
	Channels   int
	SampleRate int
	// Default reports whether the file marks the stream as the default.
	Default bool
}

// Open opens path, finds the best video and audio stream and opens a decoder
// for each.
func Open(path string, opt Options) (*File, error) {
	if err := load(); err != nil {
		return nil, err
	}
	if opt.AudioRate <= 0 {
		opt.AudioRate = 48000
	}
	f := &File{vIdx: -1, aIdx: -1}
	if rc := lib.openInput(&f.fmt, path, nil, nil); rc < 0 {
		return nil, averror("open "+path, rc)
	}
	if rc := lib.findStreamInfo(f.fmt, nil); rc < 0 {
		f.Close()
		return nil, averror("find stream info", rc)
	}
	a := &lib.abi
	if d := i64(f.fmt, a.fmtDuration); d > 0 {
		f.info.Duration = time.Duration(d) * time.Microsecond
	}

	var vcodec unsafe.Pointer
	if idx := lib.findBestStream(f.fmt, mediaVideo, -1, -1, &vcodec, 0); idx >= 0 && vcodec != nil {
		st := f.stream(idx)
		par := ptr(st, a.stCodecpar)
		f.vIdx = idx
		f.vTB[0], f.vTB[1] = rational(st, a.stTimeBase)
		f.info.HasVideo = true
		f.info.Width = int(i32(par, a.parWidth))
		f.info.Height = int(i32(par, a.parHeight))
		n, d := rational(par, a.parSAR)
		if sn, sd := rational(st, a.stSAR); sn > 0 && sd > 0 {
			n, d = sn, sd
		}
		f.info.SARNum, f.info.SARDen = int(n), int(d)
		if n, d := rational(st, a.stAvgFrameRate); d > 0 {
			f.info.FrameRate = float64(n) / float64(d)
		}
		dec, err := newVideoDecoder(vcodec, par, f.vTB, opt)
		if err != nil {
			f.Close()
			return nil, err
		}
		f.video = dec
		f.info.VideoCodec = dec.codecName
	}

	f.audioRate = opt.AudioRate
	f.decs = map[int32]*AudioDecoder{}
	f.tracks = f.audioTracks()
	if idx := lib.findBestStream(f.fmt, mediaAudio, -1, f.vIdx, nil, 0); idx >= 0 {
		for i, t := range f.tracks {
			if t.Stream == int(idx) {
				if err := f.SetAudioTrack(i); err != nil {
					f.Close()
					return nil, err
				}
			}
		}
	}
	if !f.info.HasVideo && !f.info.HasAudio {
		f.Close()
		return nil, errors.New("ffmpeg: " + path + " has neither video nor audio")
	}
	return f, nil
}

// audioTracks lists the audio streams of the file in stream order.
func (f *File) audioTracks() []AudioTrack {
	a := &lib.abi
	var out []AudioTrack
	n := int32(i32(f.fmt, a.fmtNbStreams))
	for i := range n {
		st := f.stream(i)
		par := ptr(st, a.stCodecpar)
		if i32(par, 0) != mediaAudio { // AVCodecParameters.codec_type
			continue
		}
		meta := ptr(st, a.stMetadata)
		out = append(out, AudioTrack{
			Stream:     int(i),
			Codec:      lib.codecName(i32(par, 4)),
			Language:   dictGet(meta, "language"),
			Title:      dictGet(meta, "title"),
			Channels:   int(i32(unsafe.Add(par, a.parChLayout), a.chNbChannels)),
			SampleRate: int(i32(par, a.parSampleRate)),
			Default:    i32(st, a.stDisposition)&dispositionDefault != 0,
		})
	}
	return out
}

const dispositionDefault = 1 // AV_DISPOSITION_DEFAULT

// dictGet is the value of key in an AVDictionary, or "".
func dictGet(m unsafe.Pointer, key string) string {
	if m == nil {
		return ""
	}
	e := lib.dictGet(m, key, nil, 0)
	if e == nil {
		return ""
	}
	return cString(ptr(e, unsafe.Sizeof(uintptr(0)))) // AVDictionaryEntry.value
}

// cString copies a NUL terminated C string.
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

// AudioTracks lists the audio streams of the file. The slice must not be
// modified.
func (f *File) AudioTracks() []AudioTrack { return f.tracks }

// AudioTrack is the index into [File.AudioTracks] of the stream that plays,
// or -1 for a file without sound.
func (f *File) AudioTrack() int {
	if f.audio == nil {
		return -1
	}
	return f.track
}

// SetAudioTrack makes track i of [File.AudioTracks] the one [File.ReadPacket]
// reads and [File.Audio] decodes. Its decoder is opened on first use and kept,
// so switching back and forth costs nothing after the first time.
//
// Like every method of File it belongs to the goroutine that reads packets.
// The previous decoder stays open, because a decoding goroutine may still
// hold it; the caller seeks after a switch, which is what discards what the
// old decoder had buffered.
func (f *File) SetAudioTrack(i int) error {
	if i < 0 || i >= len(f.tracks) {
		return errors.New("ffmpeg: no such audio track")
	}
	t := f.tracks[i]
	idx := int32(t.Stream)
	dec := f.decs[idx]
	if dec == nil {
		st := f.stream(idx)
		par := ptr(st, lib.abi.stCodecpar)
		codec := lib.findDecoder(i32(par, 4))
		if codec == nil {
			return errors.New("ffmpeg: no decoder for audio codec " + t.Codec)
		}
		var tb [2]int64
		tb[0], tb[1] = rational(st, lib.abi.stTimeBase)
		var err error
		if dec, err = newAudioDecoder(codec, par, tb, f.audioRate); err != nil {
			return err
		}
		f.decs[idx] = dec
	}
	f.aIdx, f.aTB, f.audio, f.track = idx, dec.tb, dec, i
	f.info.HasAudio = true
	f.info.AudioCodec = t.Codec
	f.info.AudioRate = t.SampleRate
	f.info.AudioChannels = t.Channels
	return nil
}

// SetAudioRate changes the output rate of every audio decoder of the file,
// taking effect with the next decoded frame.
func (f *File) SetAudioRate(rate int) {
	f.audioRate = rate
	for _, d := range f.decs {
		d.SetRate(rate)
	}
}

func (f *File) stream(i int32) unsafe.Pointer {
	streams := ptr(f.fmt, lib.abi.fmtStreams)
	return *(*unsafe.Pointer)(unsafe.Add(streams, uintptr(i)*unsafe.Sizeof(uintptr(0))))
}

// Info describes the file.
func (f *File) Info() Info { return f.info }

// Video is the decoder of the video stream, or nil.
func (f *File) Video() *VideoDecoder { return f.video }

// Audio is the decoder of the audio stream, or nil.
func (f *File) Audio() *AudioDecoder { return f.audio }

// Kind says which decoder a [Packet] belongs to.
type Kind uint8

const (
	KindOther Kind = iota
	KindVideo
	KindAudio
)

// Packet is one compressed unit of one stream. It owns C memory: whoever
// receives one hands it to a decoder or calls Free.
type Packet struct {
	p    unsafe.Pointer
	Kind Kind
}

// Free releases the packet. A nil packet is fine.
func (p *Packet) Free() {
	if p != nil && p.p != nil {
		lib.packetFree(&p.p)
	}
}

// ReadPacket reads the next packet of the video or the audio stream; packets
// of other streams are skipped. At the end of the file it returns io.EOF's
// stand in [ErrEOF].
func (f *File) ReadPacket() (*Packet, error) {
	for {
		pkt := lib.packetAlloc()
		if rc := lib.readFrame(f.fmt, pkt); rc < 0 {
			lib.packetFree(&pkt)
			if rc == averrorEOF {
				return nil, ErrEOF
			}
			return nil, averror("read frame", rc)
		}
		switch i32(pkt, lib.abi.pktStreamIndex) {
		case f.vIdx:
			return &Packet{p: pkt, Kind: KindVideo}, nil
		case f.aIdx:
			return &Packet{p: pkt, Kind: KindAudio}, nil
		}
		lib.packetFree(&pkt)
	}
}

// ErrEOF is returned by ReadPacket at the end of the file and by the decoders
// once they are drained.
var ErrEOF = errors.New("ffmpeg: end of file")

// Seek moves the read position to the last key frame at or before t. The
// caller flushes both decoders and discards what they return before t; that
// is what makes a seek frame exact rather than key frame exact.
func (f *File) Seek(t time.Duration) error {
	ts := t.Microseconds()
	if rc := lib.seekFile(f.fmt, -1, -1<<63, ts, ts, 0); rc < 0 {
		return averror("seek", rc)
	}
	return nil
}

// Close releases the decoders and the file.
func (f *File) Close() {
	if f.video != nil {
		f.video.close()
		f.video = nil
	}
	for _, d := range f.decs {
		d.close()
	}
	f.decs, f.audio = nil, nil
	if f.fmt != nil {
		lib.closeInput(&f.fmt)
	}
	runtime.KeepAlive(f)
}

// toDuration converts pts in time base tb to a duration without overflowing
// for any realistic file.
func toDuration(pts int64, tb [2]int64) time.Duration {
	if tb[1] == 0 {
		return 0
	}
	s := pts * tb[0]
	return time.Duration(s/tb[1])*time.Second + time.Duration(s%tb[1])*time.Second/time.Duration(tb[1])
}
