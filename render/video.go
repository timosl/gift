package render

import (
	"io"
	"time"
)

// VideoLayout is how a [VideoFrame] packs its planes into its bytes.
//
// Every layout is luma on top, Height rows of Stride bytes, and chroma below
// it, (Height+1)/2 rows of Stride bytes with U and V interleaved. Read as RGBA
// texels that is one texture of Stride/4 by Height+(Height+1)/2, which a
// backend uploads in one call and takes apart in a shader: a YUV picture is a
// third of the size of its RGBA conversion, and the conversion is cheaper on
// the GPU than on the CPU of a Raspberry Pi.
type VideoLayout uint8

const (
	// VideoNV12 is 8 bit 4:2:0.
	VideoNV12 VideoLayout = iota + 1
	// VideoP016 is 10 bit 4:2:0 in 16 bit little endian samples.
	VideoP016
)

// VideoMatrix is the YUV to RGB matrix a picture was encoded with.
type VideoMatrix uint8

const (
	BT601 VideoMatrix = iota
	BT709
	BT2020
)

// VideoField selects what of an interlaced picture is shown.
type VideoField uint8

const (
	// FieldProgressive shows the picture as it is.
	FieldProgressive VideoField = iota
	// FieldTop and FieldBottom show one field, line doubled ("bob"), so
	// that the two fields of an interlaced picture are shown one after the
	// other at twice its frame rate.
	FieldTop
	FieldBottom
)

// VideoFrame is one decoded picture on its way to the GPU. Like [Pixels] it
// is borrowed for the duration of the call it is passed to.
type VideoFrame struct {
	// Pix is the packed picture; see [VideoLayout]. Nil keeps the pixels of
	// the previous call and changes only the other fields, which is how a
	// producer switches the field of an interlaced picture without uploading
	// it again.
	Pix    []byte
	Stride int
	// Width and Height are the size of the picture in its own pixels, not
	// the size it is shown at: anamorphic pictures have pixels that are not
	// square, and the producer accounts for that in the Bounds it draws to.
	Width, Height int

	Layout    VideoLayout
	Matrix    VideoMatrix
	FullRange bool
	Field     VideoField
}

// IsEmpty reports whether f describes no picture.
func (f VideoFrame) IsEmpty() bool {
	return f.Width <= 0 || f.Height <= 0 || f.Stride < f.Width || f.Layout == 0
}

// Videos is the video picture service of a backend: the counterpart of
// [Images] for a picture that is replaced every frame.
//
// A resource is one texture per video, written by [Videos.Present] whenever
// the producer has a new picture and drawn by an [OpVideo]. It is not part of
// the image budget and never evicted: a video's memory is one picture, and the
// producer releases it when the video goes away.
//
// # Threading
//
// Like [Images] it belongs to the UI executor and is called from painters, so
// that a new picture is uploaded once per drawn frame and never more often.
type Videos interface {
	// Present makes f the current picture of the resource h and returns its
	// handle. A zero or stale h allocates a new resource. It reports false
	// for a malformed frame.
	Present(h ImageHandle, f VideoFrame) (ImageHandle, bool)

	// Resolve reports whether h is live and returns the id to put into an
	// [OpVideo].
	Resolve(h ImageHandle) (ImageID, bool)

	// Release frees the resource h refers to. A stale handle is ignored.
	Release(h ImageHandle)
}

// AudioOutput is the sound device of a backend.
//
// A process has one, at one sample rate, because Ebitengine has one audio
// context per process; a producer resamples to [AudioOutput.SampleRate].
type AudioOutput interface {
	// SampleRate is the rate every stream is played at.
	SampleRate() int

	// Open creates a player that reads interleaved little endian float32
	// stereo at SampleRate from src. It does not start playing.
	Open(src io.Reader) (AudioPlayer, error)
}

// AudioPlayer plays one stream of an [AudioOutput].
type AudioPlayer interface {
	Play()
	Pause()
	IsPlaying() bool
	// Position is how much of the stream has been played, measured from the
	// first byte the player read. It lags what has been read by the
	// buffering of the device, which is what makes it the right clock for
	// pictures.
	Position() time.Duration
	Close() error
}
