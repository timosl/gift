package videoplayer

import (
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/icon/solid"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/video"
)

var (
	playerType = gift.RegisterType("videoplayer.Player")
	layerType  = gift.RegisterType("videoplayer.layer")
)

// Metrics of the controls, in logical pixels. They are sized for a finger on
// a five inch panel of 800x480 pixels and still fit one: the smallest target
// is 44 pixels, Apple's minimum, and the play button is larger because it is
// the one a person looks for.
const (
	playSize    = float32(76)
	skipSize    = float32(56)
	barButton   = float32(44)
	volumeWidth = float32(120)
	controlGap  = float32(36)
	edge        = float32(12)
)

// Colours of the controls. A video player is dark whatever the theme: the
// controls sit on a picture, and white on a translucent black reads on every
// picture, which no themed colour does.
var (
	white     = ui.RGB(255, 255, 255)
	pill      = ui.RGBA(0, 0, 0, 110)
	pillPress = ui.RGBA(60, 60, 60, 170)
	menuFace  = ui.RGBA(28, 28, 30, 235)
)

// PlayerView is a [video.Player] with controls a finger can operate: the
// SwiftUI VideoPlayer, laid out like the player of iOS. It is created by
// [New]; the zero value is not useful.
//
//	p, err := video.Open(path, video.Options{HW: true})
//	...
//	return videoplayer.New(p)
//
// # The controls
//
// A tap on the picture shows them and a second one hides them; while the
// video plays they hide by themselves after [PlayerView.AutoHide]. In the
// middle are play and pause between a jump back and a jump forward of
// [PlayerView.Skip]. Along the bottom runs the position bar with the elapsed
// and the remaining time: tap it anywhere or drag along it. While a finger is
// on it the video pauses and the picture follows the finger, and the exact
// seek happens when it lifts. At the top are the volume, a mute button and a
// slider, and, for a file with more than one audio track, the track menu.
//
// # The player stays the application's
//
// The view does not open, own or close the player. Keep it in state, close it
// when the screen goes, and call its methods from anywhere else; the controls
// show what it does, whoever did it.
type PlayerView struct {
	p        *video.Player
	gravity  Gravity
	radius   float32
	autoHide time.Duration
	skip     time.Duration
	controls bool
	onClose  func()
	key      string
	flex     float32
}

// New returns a player view for p. p must not be nil.
func New(p *video.Player) PlayerView {
	if p == nil {
		panic("gift/ui/videoplayer: New with a nil player")
	}
	return PlayerView{p: p, autoHide: 3 * time.Second, skip: 10 * time.Second, controls: true}
}

// OnClose puts a close button at the leading end of the top bar, as
// AVPlayerViewController shows one, which calls fn. The player is not closed
// for it: closing it, and going back, is the application's; see the type.
func (v PlayerView) OnClose(fn func()) PlayerView { v.onClose = fn; return v }

// Gravity sets how the picture fills the view; see [Gravity].
func (v PlayerView) Gravity(g Gravity) PlayerView { v.gravity = g; return v }

// CornerRadius rounds the view.
func (v PlayerView) CornerRadius(r float32) PlayerView { v.radius = r; return v }

// AutoHide sets how long the controls stay after the last touch while the
// video plays. Zero keeps them. The default is three seconds.
func (v PlayerView) AutoHide(d time.Duration) PlayerView { v.autoHide = max(d, 0); return v }

// Skip sets how far the jump buttons jump. The default is ten seconds.
func (v PlayerView) Skip(d time.Duration) PlayerView {
	if d <= 0 {
		panic("gift/ui/videoplayer: Skip must be positive")
	}
	v.skip = d
	return v
}

// ShowsControls switches the controls off, which leaves a [Surface] that
// still stretches like this view. The default is true.
func (v PlayerView) ShowsControls(b bool) PlayerView { v.controls = b; return v }

// Key sets the identity among siblings.
func (v PlayerView) Key(s string) PlayerView { v.key = s; return v }

// Flex sets the share of the free space of a stack.
func (v PlayerView) Flex(f float32) PlayerView { v.flex = f; return v }

// ViewType implements gift.View.
func (v PlayerView) ViewType() gift.TypeID { return playerType }

// Build implements gift.View. The controls keep state – shown or not, the
// open menu, a drag in progress – so the body is a [gift.Component].
func (v PlayerView) Build(bc *gift.BuildContext) gift.Element {
	return ui.ZStack(gift.Component("videoplayer", v.body)).Key(v.key).Flex(v.flex).Build(bc)
}

// model is the state of the controls of one player view.
type model struct {
	visible   bool
	lastTouch time.Time
	menu      bool // the track menu is open

	scrubbing   bool
	scrubAt     time.Duration // where the finger is, while scrubbing
	wasPlaying  bool          // the player played when the finger came down
	lastPreview time.Time     // the last seek a drag issued
}

// previewEvery is how often a drag on the position bar seeks to show the
// picture under the finger. Every seek decodes from the key frame before the
// target, which on a Raspberry Pi is tens of milliseconds for full HD; four
// a second keep the picture moving without queueing seeks behind each other.
const previewEvery = 250 * time.Millisecond

func (v PlayerView) body(ctx *gift.Context) gift.View {
	p := v.p
	slot := ctx.State("model", (*model)(nil))
	m := slot.Get()
	if m == nil {
		m = &model{visible: true, lastTouch: time.Now()}
		slot.Set(m)
	}
	version := ctx.State("version", 0)
	ctx.Read(version)
	tick := ctx.State("tick", 0)
	ctx.Read(tick)
	changed := func() { version.Set(version.Get() + 1) }
	touch := func() {
		m.visible, m.lastTouch = true, time.Now()
		changed()
	}

	status := p.TimeControlStatus()
	playing := status != video.Paused
	if m.visible && playing && v.autoHide > 0 && !m.scrubbing && !m.menu && time.Since(m.lastTouch) >= v.autoHide {
		m.visible = false
	}

	// The time labels and the auto hide need a build now and then while the
	// video plays; nothing else changes on its own. Four times a second
	// while the controls show, once a second while they do not, so that the
	// end of the video brings them back.
	if playing {
		every := time.Second
		if m.visible {
			every = previewEvery
		}
		tok := ctx.Request()
		go func() {
			time.Sleep(every)
			tok.Post(func() { tick.Set(tick.Get() + 1) })
		}()
	}

	surface := Surface(p).Gravity(v.gravity).CornerRadius(v.radius).Key("surface").Label("Video").
		OnTap(func() {
			if !v.controls {
				return
			}
			m.visible = !m.visible || m.menu
			m.menu = false
			m.lastTouch = time.Now()
			changed()
		})
	if !v.controls || !m.visible {
		return layer{children: []gift.View{surface}}
	}
	return layer{children: []gift.View{
		surface,
		// No scrim dims the picture under the controls: the pills give
		// them their contrast, and a translucent fill over the whole
		// picture is one more full screen pass, which cost a full HD
		// video on a Raspberry Pi 5 a frame every second or so.
		ui.ZStack(
			ui.VStack(
				v.topBar(m, touch, changed),
				ui.Spacer(),
				v.transport(status, touch),
				ui.Spacer(),
				v.bottomBar(m, touch, changed),
			).Padding(edge),
			v.trackMenu(m, touch, changed),
		),
	}}
}

// roundButton is a white icon on a translucent disc.
func roundButton(sym ui.Symbol, size, icon float32, label string, action func()) ui.ButtonView {
	return ui.Button(ui.Icon(sym).Size(icon).Foreground(white), action).
		Style(ui.ButtonStyle{Background: pill, CornerRadius: size / 2}).
		HoverStyle(ui.ButtonStyle{Background: pillPress, CornerRadius: size / 2}).
		PressedStyle(ui.ButtonStyle{Background: pillPress, CornerRadius: size / 2}).
		Border(ui.Border{}).
		Padding(0).
		Frame(size, size).
		Label(label)
}

// transport is jump back, play or pause, jump forward.
func (v PlayerView) transport(status video.TimeControlStatus, touch func()) gift.View {
	p := v.p
	sym, label := solid.Pause, "Pause"
	if status == video.Paused {
		sym, label = solid.Play, "Play"
	}
	return ui.HStack(
		ui.Spacer(),
		roundButton(outline.Undo, skipSize, 28, "Back", func() {
			p.Seek(p.CurrentTime() - v.skip)
			touch()
		}).Key("back"),
		roundButton(sym, playSize, 40, label, func() {
			if p.TimeControlStatus() == video.Paused {
				p.Play()
			} else {
				p.Pause()
			}
			touch()
		}).Key("play"),
		roundButton(outline.Redo, skipSize, 28, "Forward", func() {
			p.Seek(min(p.CurrentTime()+v.skip, p.Duration()))
			touch()
		}).Key("forward"),
		ui.Spacer(),
	).Gap(controlGap).Align(geom.Center)
}

// topBar is the volume on the left and the track menu button on the right.
func (v PlayerView) topBar(m *model, touch, changed func()) gift.View {
	p := v.p
	vol, muted := p.Volume(), p.Muted()
	sym := solid.VolumeUp
	switch {
	case muted || vol == 0:
		sym = solid.VolumeMute
	case vol < 0.5:
		sym = solid.VolumeDown
	}
	label := "Mute"
	if muted {
		label = "Unmute"
	}
	disc := ui.ButtonStyle{Background: ui.ColorClear, CornerRadius: barButton / 2}
	pressed := ui.ButtonStyle{Background: pillPress, CornerRadius: barButton / 2}
	volume := ui.HStack(
		ui.Button(ui.Icon(sym).Size(24).Foreground(white), func() {
			p.SetMuted(!p.Muted())
			touch()
		}).Style(disc).HoverStyle(disc).PressedStyle(pressed).Border(ui.Border{}).
			Padding(0).Frame(barButton, barButton).Label(label).Key("mute"),
		volumeBar{value: vol, onChange: func(x float64) {
			p.SetVolume(x)
			if x > 0 && p.Muted() {
				p.SetMuted(false)
			}
			touch()
		}},
	).Gap(2).PaddingInsets(geom.Insets{Right: 12}).Background(pill).CornerRadius(barButton / 2).Align(geom.Center)

	row := []gift.View{volume, ui.Spacer()}
	if v.onClose != nil {
		row = append([]gift.View{roundButton(outline.Close, barButton, 22, "Close", v.onClose).Key("close")}, row...)
	}
	if len(p.AudioTracks()) > 1 {
		row = append(row, roundButton(solid.Headphones, barButton, 22, "Audio track", func() {
			m.menu = !m.menu
			touch()
		}).Key("tracks"))
	}
	return ui.HStack(row...).Gap(10).Align(geom.Center)
}

// bottomBar is the position bar between the elapsed and the remaining time.
func (v PlayerView) bottomBar(m *model, touch, changed func()) gift.View {
	p := v.p
	pos, dur := p.CurrentTime(), p.Duration()
	if m.scrubbing {
		pos = m.scrubAt
	}
	at := func(f float64) time.Duration { return time.Duration(f * float64(dur)) }
	bar := scrubber{
		p:     p,
		label: "Position",
		onBegin: func() {
			m.scrubbing, m.wasPlaying = true, p.TimeControlStatus() != video.Paused
			p.Pause()
			touch()
		},
		onMove: func(f float64) {
			m.scrubAt = at(f)
			if time.Since(m.lastPreview) >= previewEvery {
				m.lastPreview = time.Now()
				p.Seek(m.scrubAt)
			}
			changed()
		},
		onEnd: func(f float64) {
			m.scrubbing = false
			p.Seek(at(f))
			if m.wasPlaying {
				p.Play()
			}
			touch()
		},
	}
	label := func(s, key string) ui.TextView {
		return ui.Text(s).FontSize(15).Foreground(white).Key(key)
	}
	return ui.HStack(
		label(clock(pos), "elapsed").MinWidth(52),
		bar,
		label(remaining(pos, dur), "remaining").Align(ui.AlignTrailing).MinWidth(60),
	).Gap(12).PaddingInsets(geom.Insets{Left: 16, Right: 16}).Background(pill).CornerRadius(barButton / 2).Align(geom.Center)
}

// trackMenu lists the audio tracks under the menu button while the menu is
// open, and is nothing otherwise.
func (v PlayerView) trackMenu(m *model, touch, changed func()) gift.View {
	p := v.p
	if !m.menu {
		return ui.Box().Frame(0, 0)
	}
	cur := p.AudioTrack()
	rows := make([]gift.View, 0, len(p.AudioTracks()))
	for i, t := range p.AudioTracks() {
		check := ui.Icon(outline.Check).Size(20).Foreground(white)
		if i != cur {
			check = check.Foreground(ui.ColorClear)
		}
		name := trackName(t, i)
		rows = append(rows, ui.Button(
			ui.HStack(check, ui.Text(name).FontSize(17).Foreground(white)).Gap(10).Align(geom.Center),
			func() {
				p.SelectAudioTrack(i)
				m.menu = false
				touch()
			}).
			Style(ui.ButtonStyle{Background: ui.ColorClear}).
			HoverStyle(ui.ButtonStyle{Background: ui.RGBA(255, 255, 255, 30), CornerRadius: 10}).
			PressedStyle(ui.ButtonStyle{Background: ui.RGBA(255, 255, 255, 50), CornerRadius: 10}).
			Border(ui.Border{}).
			PaddingInsets(geom.Insets{Left: 12, Right: 16, Top: 10, Bottom: 10}).
			MinHeight(barButton).Align(geom.Leading).Label(name).Key("track-"+t.Language+"-"+itoa(i)))
	}
	menu := ui.VStack(
		ui.Text("Audio").FontSize(13).Foreground(ui.RGBA(255, 255, 255, 150)).
			PaddingInsets(geom.Insets{Left: 12, Top: 4, Bottom: 2}),
		ui.VStack(rows...).Gap(2),
	).Gap(4).Padding(8).Background(menuFace).CornerRadius(14).MaxWidth(320).Key("track-menu")
	return ui.VStack(ui.HStack(ui.Spacer(), menu), ui.Spacer()).
		PaddingInsets(geom.Insets{Top: edge + barButton + 8, Right: edge})
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return clock(time.Duration(i) * time.Second)
}

// layer sizes its first child as the constraints allow and every other child
// to exactly the size of the first, stacked on top: the controls cover the
// picture, not whatever the picture's parent offered. A [ui.ZStack] would
// offer the controls the parent's constraints, and in a stack with an
// unbounded height the spacers between them would collapse.
type layer struct{ children []gift.View }

func (l layer) ViewType() gift.TypeID { return layerType }

func (l layer) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: layerLayout{}, Children: l.children}
}

type layerLayout struct{}

func (layerLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	for i := 1; i < ctx.ChildCount(); i++ {
		ctx.Measure(i, geom.Tight(s))
		ctx.Place(i, geom.Point{})
	}
	return s
}
