// Package video plays video files with sound: an AVPlayer for gift.
//
// The vocabulary is AVFoundation's on purpose. A [Player] is told to Play,
// Pause and Seek, reports a [Status] and a [TimeControlStatus], and knows its
// CurrentTime and Duration. It renders nothing itself: a backend asks it each
// drawn frame for the picture of that moment ([Player.Frame]) and reads its
// sound as an io.Reader ([Player.AudioStream]).
//
// # Timing
//
// With sound, the audio device is the master: the player's clock follows how
// far the device has played (see clock.go), and pictures are chosen by that
// clock. Without sound, the clock is the wall clock.
//
// # Threads
//
// One goroutine reads packets, one decodes pictures, one decodes sound. All
// three hand their results over under one mutex, which the methods of Player
// take as well; every method is safe for concurrent use.
package video

import (
	"errors"
	"sync"
	"time"

	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/video/ffmpeg"
)

// Status is whether a player can play, in the sense of AVPlayer.Status.
type Status uint8

const (
	StatusUnknown Status = iota
	StatusReadyToPlay
	StatusFailed
)

// TimeControlStatus is what the clock does, in the sense of
// AVPlayer.TimeControlStatus.
type TimeControlStatus uint8

const (
	Paused TimeControlStatus = iota
	// WaitingToPlay: told to play, but the pictures or the sound for the
	// current position are not decoded yet, after opening or a seek.
	WaitingToPlay
	Playing
)

// Options configure a [Player].
type Options struct {
	// HW asks for hardware decoding where there is one; see ffmpeg.Options.
	HW bool
	// Loop starts over at the end instead of pausing.
	Loop bool
	// AudioRate is the sample rate the backend's audio context runs at.
	// Zero is 48000.
	AudioRate int
	// Frames is the number of decoded pictures kept ready. Zero is 8.
	Frames int
}

// Stats are counters for measuring playback.
type Stats struct {
	Decoded        int // pictures out of the decoder
	Presented      int // pictures handed to the renderer
	Late           int // pictures dropped because the clock had passed them
	AudioUnderruns int // device reads that found too little sound
	// AVOffset is the presentation time of the picture shown minus the
	// clock, at the last call to Frame. Positive is early.
	AVOffset time.Duration
	// ClockError is the last correction the audio position asked of the
	// clock.
	ClockError time.Duration
}

// Player plays one file.
type Player struct {
	mu   sync.Mutex
	cond *sync.Cond

	opt    Options
	file   *ffmpeg.File
	info   ffmpeg.Info
	status Status
	err    error

	playing bool // what the application asked for
	ended   bool
	closed  bool
	volume  float64
	muted   bool

	gen      int           // bumped by every seek
	target   time.Duration // the position the current generation starts at
	seekReq  chan time.Duration
	videoEOF bool // the video decoder is drained, for this generation
	audioEOF bool

	free   []*ffmpeg.Frame
	queue  []*ffmpeg.Frame // decoded, presentation order
	shown  *ffmpeg.Frame
	poster bool // the next decoded picture is shown even while paused

	audio *AudioStream
	clk   clock
	stats Stats

	// tracks are the audio tracks of the file and track the one that plays.
	// trackReq is a switch the demuxer has not performed yet, -1 for none.
	tracks   []ffmpeg.AudioTrack
	track    int
	trackReq int

	// out is the sound device, dev the player of out that plays audio, and
	// devStream the stream dev reads. All three are touched by syncDevice
	// only, outside mu; see Frame.
	out       render.AudioOutput
	dev       render.AudioPlayer
	devStream *AudioStream

	onEnd func()
}

// Open opens path and starts decoding. It returns once the file is open, so
// that the size and duration are known; decoding continues in the background.
func Open(path string, opt Options) (*Player, error) {
	if opt.AudioRate <= 0 {
		opt.AudioRate = 48000
	}
	if opt.Frames <= 0 {
		opt.Frames = 8
	}
	f, err := ffmpeg.Open(path, ffmpeg.Options{HW: opt.HW, AudioRate: opt.AudioRate})
	if err != nil {
		return nil, err
	}
	p := &Player{opt: opt, file: f, info: f.Info(), status: StatusReadyToPlay, volume: 1,
		seekReq: make(chan time.Duration, 1), poster: true,
		tracks: f.AudioTracks(), track: f.AudioTrack(), trackReq: -1}
	p.cond = sync.NewCond(&p.mu)
	p.clk.rate = 1
	for range opt.Frames {
		p.free = append(p.free, new(ffmpeg.Frame))
	}
	if p.info.HasAudio {
		p.audio = newAudioStream(p, 0, opt.AudioRate, 0)
	} else {
		p.audioEOF = true
	}
	if !p.info.HasVideo {
		p.videoEOF = true
	}
	vq := make(chan msg, 256)
	aq := make(chan msg, 512)
	go p.demux(vq, aq)
	if p.info.HasVideo {
		go p.decodeVideo(vq)
	}
	if p.info.HasAudio {
		go p.decodeAudio(aq)
	}
	return p, nil
}

// msg carries a packet, or the end of the file, of one generation. An audio
// message names the decoder of its track, because a track switch replaces it
// while the audio goroutine may still be decoding the old one.
type msg struct {
	pkt  *ffmpeg.Packet
	gen  int
	eof  bool
	adec *ffmpeg.AudioDecoder
}

// Info describes the file.
func (p *Player) Info() ffmpeg.Info { return p.info }

// VideoHW is the hardware the pictures are decoded on, or "".
func (p *Player) VideoHW() string {
	if v := p.file.Video(); v != nil {
		return v.HW()
	}
	return ""
}

// Duration is the length of the file, or zero when it is not known.
func (p *Player) Duration() time.Duration { return p.info.Duration }

// Status reports whether the player can play.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// Err is the error that made the status StatusFailed.
func (p *Player) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Play starts or resumes playback. At the end of the file it starts over.
func (p *Player) Play() {
	p.mu.Lock()
	ended := p.ended
	p.playing = true
	p.mu.Unlock()
	if ended {
		p.Seek(0)
	}
}

// Pause stops the clock. The picture stays.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = false
	p.clk.stop(time.Now())
}

// SetVolume sets the volume from 0 to 1.
func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	p.volume = min(max(v, 0), 1)
	p.mu.Unlock()
}

// SetMuted mutes or unmutes the sound without changing the volume.
func (p *Player) SetMuted(m bool) {
	p.mu.Lock()
	p.muted = m
	p.mu.Unlock()
}

// SetLoop makes the player start over at the end.
func (p *Player) SetLoop(l bool) {
	p.mu.Lock()
	p.opt.Loop = l
	p.mu.Unlock()
}

// Volume is the volume from 0 to 1.
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

// Muted reports whether the sound is muted.
func (p *Player) Muted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.muted
}

// AudioTracks lists the audio tracks of the file. The slice must not be
// modified.
func (p *Player) AudioTracks() []ffmpeg.AudioTrack { return p.tracks }

// AudioTrack is the index into [Player.AudioTracks] of the track that plays,
// or -1 for a file without sound.
func (p *Player) AudioTrack() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.track
}

// SelectAudioTrack switches to track i of [Player.AudioTracks], at the
// current position: AVPlayerItem.select(_:in:) for the audible group. The
// switch is a seek to where the clock is, because the sound the device has
// buffered belongs to the old track.
func (p *Player) SelectAudioTrack(i int) {
	p.mu.Lock()
	if i < 0 || i >= len(p.tracks) || i == p.track {
		p.mu.Unlock()
		return
	}
	p.track, p.trackReq = i, i
	p.mu.Unlock()
	p.Seek(p.CurrentTime())
}

// AttachAudio plays the sound on out. Without a device the sound is decoded
// and dropped in step with the clock, which then runs on the wall clock; that
// is the headless case. A device at another sample rate than the player was
// opened for makes the decoder resample to it, from the current position on.
func (p *Player) AttachAudio(out render.AudioOutput) {
	p.mu.Lock()
	if p.out == out {
		p.mu.Unlock()
		return
	}
	p.out = out
	reseek := false
	if out != nil && out.SampleRate() != p.opt.AudioRate && out.SampleRate() > 0 {
		p.opt.AudioRate = out.SampleRate()
		reseek = true
	}
	p.mu.Unlock()
	if reseek {
		p.Seek(p.CurrentTime())
	}
}

// OnEnd installs a function that is called, on a decoder goroutine, when
// playback reaches the end of the file without looping: the analogue of
// AVPlayerItemDidPlayToEndTime.
func (p *Player) OnEnd(f func()) {
	p.mu.Lock()
	p.onEnd = f
	p.mu.Unlock()
}

// Seek moves to t, exactly: the decoders start at the key frame before t and
// everything decoded before t is dropped. A playing player keeps playing from
// there once the first picture and enough sound are decoded.
func (p *Player) Seek(t time.Duration) {
	t = max(t, 0)
	p.mu.Lock()
	p.gen++
	p.target = t
	p.ended = false
	p.videoEOF = !p.info.HasVideo
	p.audioEOF = !p.info.HasAudio
	p.free = append(p.free, p.queue...)
	p.queue = p.queue[:0]
	p.poster = true
	p.clk.stop(time.Now())
	p.clk.set(t, time.Now())
	if p.info.HasAudio {
		p.audio = newAudioStream(p, p.gen, p.opt.AudioRate, t)
	}
	p.cond.Broadcast()
	p.mu.Unlock()
	// The demuxer may be blocked on a full queue; the request wakes it.
	select {
	case <-p.seekReq:
	default:
	}
	p.seekReq <- t
}

// CurrentTime is the position of the clock.
func (p *Player) CurrentTime() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return min(p.clk.now(time.Now()), max(p.info.Duration, p.clk.base))
}

// TimeControlStatus reports what the clock does.
func (p *Player) TimeControlStatus() TimeControlStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.timeControl()
}

func (p *Player) timeControl() TimeControlStatus {
	switch {
	case !p.playing || p.ended:
		return Paused
	case p.clk.running:
		return Playing
	}
	return WaitingToPlay
}

// AudioStream is the sound to play now, or nil for a file without sound. It
// changes after every seek; the backend replaces its device player when it
// does. See [AudioStream].
func (p *Player) AudioStream() *AudioStream {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.audio
}

// AudioRate is the sample rate of [Player.AudioStream].
func (p *Player) AudioRate() int { return p.opt.AudioRate }

// Stats returns the counters.
func (p *Player) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// Frame returns the picture to show at now and whether it differs from the one
// the previous call returned. The frame stays valid until the next call; the
// caller uploads it in between.
//
// It is also what advances the player: it starts the clock once a seek or the
// start has been decoded far enough, follows the audio position, and notices
// the end of the file. A backend calls it once per drawn frame, with the time
// the frame is expected on screen.
func (p *Player) Frame(now time.Time) (*ffmpeg.Frame, bool) {
	f, changed := p.frame(now)
	p.syncDevice()
	return f, changed
}

// syncDevice keeps one device player per audio stream and runs it while the
// clock runs. It is called without p.mu, because the device player takes a
// lock of its own that it also holds while it calls AudioStream.Read.
func (p *Player) syncDevice() {
	p.mu.Lock()
	out, s, dev, devStream := p.out, p.audio, p.dev, p.devStream
	want := p.timeControl() == Playing
	closed := p.closed
	p.mu.Unlock()
	if out == nil || s == nil || closed {
		return
	}
	if s != devStream {
		if dev != nil {
			dev.Close()
		}
		nd, err := out.Open(s)
		if err != nil {
			p.fail(err)
			return
		}
		s.SetPosition(nd.Position)
		dev = nd
		p.mu.Lock()
		p.dev, p.devStream = nd, s
		p.mu.Unlock()
	}
	switch playing := dev.IsPlaying(); {
	case want && !playing:
		dev.Play()
	case !want && playing:
		dev.Pause()
	}
}

func (p *Player) frame(now time.Time) (*ffmpeg.Frame, bool) {
	// The device position is read without p.mu: the device player holds
	// its own lock while it calls AudioStream.Read, which takes p.mu, so
	// the opposite order here would deadlock.
	p.mu.Lock()
	s := p.audio
	var position func() time.Duration
	if s != nil {
		position = s.position
	}
	p.mu.Unlock()
	devPos := time.Duration(-1)
	if position != nil {
		devPos = position()
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if s != p.audio {
		devPos = -1 // a seek replaced the stream meanwhile
	}
	p.advance(now, devPos)
	t := p.clk.now(now)

	changed := false
	// The first picture after opening or a seek is shown even while
	// paused, as a poster of the new position.
	if p.poster && len(p.queue) > 0 {
		p.poster = false
		p.present(p.queue[0])
		changed = true
	}
	for len(p.queue) > 0 && p.queue[0].PTS <= t {
		next := p.queue[0]
		// A picture whose successor is also due is late and skipped.
		if len(p.queue) > 1 && p.queue[1].PTS <= t {
			p.queue = p.queue[1:]
			p.free = append(p.free, next)
			p.stats.Late++
			continue
		}
		if next != p.shown {
			p.present(next)
			changed = true
		}
		break
	}
	if p.shown != nil {
		p.stats.AVOffset = p.shown.PTS - t
	}
	p.cond.Broadcast()
	return p.shown, changed
}

// present makes f, the head of the queue, the picture on screen.
func (p *Player) present(f *ffmpeg.Frame) {
	p.queue = p.queue[1:]
	if p.shown != nil {
		p.free = append(p.free, p.shown)
	}
	p.shown = f
	p.stats.Presented++
}

// advance runs the clock. devPos is how much of the current audio stream the
// device has played, or negative when unknown. Called with p.mu held.
func (p *Player) advance(now time.Time, devPos time.Duration) {
	if !p.playing || p.ended {
		p.clk.stop(now)
		return
	}
	if !p.clk.running {
		// Start once there is a picture of the new position and some
		// sound, or the respective stream has ended.
		videoReady := p.videoEOF || len(p.queue) > 0 || !p.poster && p.shown != nil
		audioReady := p.audioEOF || p.audio == nil || p.audio.buffered() >= 100*time.Millisecond
		if !videoReady || !audioReady {
			return
		}
		p.clk.run(now)
	}
	if p.audio != nil && devPos >= 0 {
		p.stats.ClockError = p.clk.follow(p.audio.played(devPos), now)
	}
	if p.audio != nil && p.out == nil {
		// No device reads the sound; drop it in step with the clock so
		// that the decoder keeps going.
		p.audio.discard(p.clk.now(now))
	}
	// The end: both streams are drained and the last picture has had its
	// time on screen.
	if p.videoEOF && p.audioEOF && len(p.queue) == 0 {
		last := p.clk.now(now)
		if p.shown != nil {
			last = p.shown.PTS + p.shown.Duration
		}
		if p.audio != nil && p.audio.size > 0 {
			return
		}
		if p.clk.now(now) >= last {
			p.ended = true
			p.clk.stop(now)
			if p.opt.Loop {
				go p.Seek(0)
				p.playing = true
			} else if p.onEnd != nil {
				go p.onEnd()
			}
		}
	}
}

// Closed reports whether [Player.Close] was called.
func (p *Player) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// NeedsFrames reports whether [Player.Frame] may return a new picture soon:
// while the clock runs or waits to, and after a seek until its picture is
// shown. A view repaints while it is true and stops when it is not, which is
// what lets an application with a paused video go idle.
func (p *Player) NeedsFrames() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && (p.timeControl() != Paused || p.poster)
}

// Close stops the goroutines and releases the file.
func (p *Player) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	dev := p.dev
	p.dev, p.devStream = nil, nil
	p.cond.Broadcast()
	p.mu.Unlock()
	if dev != nil {
		dev.Close()
	}
	select {
	case <-p.seekReq:
	default:
	}
	select {
	case p.seekReq <- -1:
	default:
	}
}

func (p *Player) fail(err error) {
	p.mu.Lock()
	if p.status != StatusFailed {
		p.status, p.err = StatusFailed, err
	}
	p.mu.Unlock()
}

// demux reads packets and sends them to the decoders, and performs seeks.
func (p *Player) demux(vq, aq chan msg) {
	defer func() {
		close(vq)
		close(aq)
	}()
	gen := 0
	eof := false
	for {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return
		}
		if eof {
			// Nothing to read until a seek.
			t := <-p.seekReq
			if t < 0 {
				return
			}
			gen, eof = p.doSeek(t)
			continue
		}
		pkt, err := p.file.ReadPacket()
		if errors.Is(err, ffmpeg.ErrEOF) {
			eof = true
			vq <- msg{gen: gen, eof: true}
			aq <- msg{gen: gen, eof: true, adec: p.file.Audio()}
			continue
		}
		if err != nil {
			p.fail(err)
			return
		}
		q, m := vq, msg{pkt: pkt, gen: gen}
		if pkt.Kind == ffmpeg.KindAudio {
			q, m.adec = aq, p.file.Audio()
		}
		select {
		case q <- m:
		case t := <-p.seekReq:
			pkt.Free()
			if t < 0 {
				return
			}
			gen, eof = p.doSeek(t)
		}
	}
}

func (p *Player) doSeek(t time.Duration) (int, bool) {
	p.mu.Lock()
	gen, req, rate := p.gen, p.trackReq, p.opt.AudioRate
	p.trackReq = -1
	p.mu.Unlock()
	if req >= 0 {
		if err := p.file.SetAudioTrack(req); err != nil {
			p.fail(err)
		}
	}
	p.file.SetAudioRate(rate)
	if err := p.file.Seek(t); err != nil {
		p.fail(err)
	}
	return gen, false
}

// current reports whether gen is still the player's generation and the player
// is open. Called with p.mu held.
func (p *Player) current(gen int) bool { return gen == p.gen && !p.closed }

func (p *Player) decodeVideo(q chan msg) {
	dec := p.file.Video()
	gen := 0
	var spare *ffmpeg.Frame
	receive := func() {
		for {
			if spare == nil {
				p.mu.Lock()
				for len(p.free) == 0 && p.current(gen) {
					p.cond.Wait()
				}
				if !p.current(gen) {
					p.mu.Unlock()
					return
				}
				spare = p.free[len(p.free)-1]
				p.free = p.free[:len(p.free)-1]
				p.mu.Unlock()
			}
			err := dec.Receive(spare)
			if errors.Is(err, ffmpeg.ErrEOF) {
				p.mu.Lock()
				if p.current(gen) {
					p.videoEOF = true
				}
				p.mu.Unlock()
				return
			}
			if err != nil {
				if !errors.Is(err, ffmpeg.ErrAgain) {
					p.fail(err)
				}
				return
			}
			p.mu.Lock()
			p.stats.Decoded++
			// Pictures that end before the seek target are decoded only
			// to reach it.
			if p.current(gen) && (spare.PTS+spare.Duration > p.target || spare.Duration == 0 && spare.PTS >= p.target) {
				p.queue = append(p.queue, spare)
				spare = nil
			}
			p.mu.Unlock()
		}
	}
	for m := range q {
		p.mu.Lock()
		stale := m.gen != p.gen
		closed := p.closed
		p.mu.Unlock()
		if closed {
			m.pkt.Free()
			return
		}
		if stale {
			m.pkt.Free()
			continue
		}
		if m.gen != gen {
			dec.Flush()
			gen = m.gen
		}
		if m.eof {
			dec.Send(nil)
			receive()
			continue
		}
		for dec.Send(m.pkt) == ffmpeg.ErrAgain {
			receive()
		}
		m.pkt.Free()
		receive()
	}
}

func (p *Player) decodeAudio(q chan msg) {
	var dec *ffmpeg.AudioDecoder
	gen := 0
	var pcm []float32
	receive := func() {
		for {
			var pts time.Duration
			var err error
			pcm, pts, err = dec.Receive(pcm[:0])
			if errors.Is(err, ffmpeg.ErrEOF) {
				p.mu.Lock()
				if p.current(gen) {
					p.audioEOF = true
					if p.audio != nil {
						p.audio.eof = true
					}
				}
				p.mu.Unlock()
				return
			}
			if err != nil {
				if !errors.Is(err, ffmpeg.ErrAgain) {
					p.fail(err)
				}
				return
			}
			p.mu.Lock()
			if !p.current(gen) {
				p.mu.Unlock()
				return
			}
			// Drop the samples before the seek target, also within a
			// frame, so that sound and picture start at the same time.
			if skip := p.target - pts; skip > 0 {
				n := 2 * int(int64(skip)*int64(p.opt.AudioRate)/int64(time.Second))
				pcm = pcm[min(n, len(pcm)):]
			}
			if len(pcm) > 0 {
				p.audio.write(pcm)
			}
			p.mu.Unlock()
		}
	}
	for m := range q {
		p.mu.Lock()
		stale := m.gen != p.gen
		closed := p.closed
		p.mu.Unlock()
		if closed {
			m.pkt.Free()
			return
		}
		if stale {
			m.pkt.Free()
			continue
		}
		if m.adec != nil && m.adec != dec {
			// A track switch, which always comes with a seek: the new
			// decoder may hold what it decoded when it last played.
			dec = m.adec
			dec.Flush()
			gen = m.gen
		} else if m.gen != gen {
			dec.Flush()
			gen = m.gen
		}
		if m.eof {
			dec.Send(nil)
			receive()
			continue
		}
		for dec.Send(m.pkt) == ffmpeg.ErrAgain {
			receive()
		}
		m.pkt.Free()
		receive()
	}
}
