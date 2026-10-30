package video

import (
	"encoding/binary"
	"math"
	"time"
)

// AudioStream is the sound of a [Player] as an io.Reader of interleaved
// little endian float32 stereo at [Player.AudioRate], which is what
// Ebitengine's audio.Context.NewPlayerF32 reads.
//
// A player hands out a new stream after every seek, because the samples an
// audio device has already buffered belong to the old position and the only
// way to drop them is to drop the device player. A stream that has been
// replaced reads as silence.
//
// The backend tells the stream how far its device player really is through
// [AudioStream.SetPosition]; the player derives its clock from that.
type AudioStream struct {
	p    *Player
	gen  int
	rate int

	// Guarded by p.mu.
	ring     []float32 // stereo samples, interleaved
	head     int       // read index into ring, in float32s
	size     int       // filled float32s
	nextPTS  time.Duration
	read     int64 // sample frames handed out
	segs     []seg
	eof      bool
	position func() time.Duration
}

// seg says that the sample frame at index start of the stream has the
// presentation time pts. A new seg starts wherever the samples handed out
// stop being continuous: after silence filled in for an underrun.
type seg struct {
	start int64
	pts   time.Duration
}

func newAudioStream(p *Player, gen, rate int, pts time.Duration) *AudioStream {
	return &AudioStream{p: p, gen: gen, rate: rate, ring: make([]float32, 2*rate), nextPTS: pts,
		segs: []seg{{0, pts}}}
}

// SetPosition installs the function that reports how much of this stream the
// device has played, measured from the first byte it read.
func (s *AudioStream) SetPosition(f func() time.Duration) {
	s.p.mu.Lock()
	s.position = f
	s.p.mu.Unlock()
}

// Read fills b with samples. It never blocks: what the decoder has not
// delivered yet is silence, and the clock holds still through it.
func (s *AudioStream) Read(b []byte) (int, error) {
	n := len(b) / 8 * 8
	p := s.p
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.gen != p.gen || p.closed {
		clear(b[:n])
		return n, nil
	}
	frames := n / 8
	have := min(frames, s.size/2)
	vol := float32(p.volume)
	if p.muted {
		vol = 0
	}
	for i := 0; i < have*2; i++ {
		v := s.ring[(s.head+i)%len(s.ring)] * vol
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	s.head = (s.head + have*2) % len(s.ring)
	s.size -= have * 2
	s.read += int64(have)
	s.nextPTS += time.Duration(have) * time.Second / time.Duration(s.rate)
	if have < frames {
		clear(b[have*8 : n])
		if !s.eof {
			p.stats.AudioUnderruns++
		}
		s.read += int64(frames - have)
		s.segs = append(s.segs, seg{s.read, s.nextPTS})
	}
	// Segments the device has played long ago are no longer needed; keep
	// the slice from growing through a long run of underruns.
	if len(s.segs) > 64 {
		s.segs = append(s.segs[:0], s.segs[len(s.segs)-32:]...)
	}
	p.cond.Broadcast()
	return n, nil
}

// played is the presentation time of the sample the device plays now, given
// how much of the stream it has played. Called with p.mu held.
func (s *AudioStream) played(pos time.Duration) time.Duration {
	idx := int64(pos) * int64(s.rate) / int64(time.Second)
	if idx <= 0 {
		return s.segs[0].pts
	}
	i := len(s.segs) - 1
	for i > 0 && s.segs[i].start > idx {
		i--
	}
	t := s.segs[i].pts + time.Duration(idx-s.segs[i].start)*time.Second/time.Duration(s.rate)
	if i+1 < len(s.segs) && t > s.segs[i+1].pts {
		t = s.segs[i+1].pts // silence: the clock waits for the samples
	}
	return t
}

// write appends decoded samples, blocking while the ring is full. It returns
// false when the stream was replaced or the player closed meanwhile. Called
// with p.mu held; waiting releases it.
func (s *AudioStream) write(pcm []float32) bool {
	p := s.p
	for len(pcm) > 0 {
		for s.size == len(s.ring) {
			if s.gen != p.gen || p.closed {
				return false
			}
			p.cond.Wait()
		}
		if s.gen != p.gen || p.closed {
			return false
		}
		tail := (s.head + s.size) % len(s.ring)
		n := min(len(pcm), len(s.ring)-s.size, len(s.ring)-tail)
		copy(s.ring[tail:tail+n], pcm[:n])
		s.size += n
		pcm = pcm[n:]
	}
	return true
}

// discard drops the samples before t, for a stream no device reads. Called
// with p.mu held.
func (s *AudioStream) discard(t time.Duration) {
	if t <= s.nextPTS {
		return
	}
	n := min(int(int64(t-s.nextPTS)*int64(s.rate)/int64(time.Second)), s.size/2)
	if n <= 0 {
		return
	}
	s.head = (s.head + 2*n) % len(s.ring)
	s.size -= 2 * n
	s.read += int64(n)
	s.nextPTS += time.Duration(n) * time.Second / time.Duration(s.rate)
	s.p.cond.Broadcast()
}

// buffered is how much sound is waiting. Called with p.mu held.
func (s *AudioStream) buffered() time.Duration {
	return time.Duration(s.size/2) * time.Second / time.Duration(s.rate)
}
