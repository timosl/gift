package ebiten

import (
	"io"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/worldiety/gift/render"
)

// audioOutput is the [render.AudioOutput] of this backend: Ebitengine's audio
// context, opened by the first stream.
//
// Ebitengine allows one audio context per process, at one sample rate, and an
// application may have made it already; then that one is used, at its rate,
// and the rate asked for is ignored. Underneath it is oto, which talks to
// PulseAudio in Go and falls back to ALSA through purego, so playing sound
// needs no cgo either.
type audioOutput struct {
	rate int

	once sync.Once
	ctx  *audio.Context
}

func newAudioOutput(rate int) *audioOutput {
	if rate <= 0 {
		rate = 48000
	}
	return &audioOutput{rate: rate}
}

func (o *audioOutput) context() *audio.Context {
	o.once.Do(func() {
		if c := audio.CurrentContext(); c != nil {
			o.ctx = c
		} else {
			o.ctx = audio.NewContext(o.rate)
		}
		o.rate = o.ctx.SampleRate()
	})
	return o.ctx
}

// SampleRate implements [render.AudioOutput].
func (o *audioOutput) SampleRate() int {
	if c := audio.CurrentContext(); c != nil {
		return c.SampleRate()
	}
	return o.rate
}

// audioBuffer is the device buffering a stream asks for: short, because the
// position it reports lags by it and a video seeks and pauses through it, and
// long enough that a busy frame does not starve the device.
const audioBuffer = 60 * time.Millisecond

// Open implements [render.AudioOutput].
func (o *audioOutput) Open(src io.Reader) (render.AudioPlayer, error) {
	p, err := o.context().NewPlayerF32(src)
	if err != nil {
		return nil, err
	}
	p.SetBufferSize(audioBuffer)
	return p, nil
}
