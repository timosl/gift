// Command gift-videoproto is the feasibility prototype of gift's video player:
// it plays one file with sound in an Ebitengine window and prints, once a
// second, what the project plan, section 25, needs measured.
//
//	go run ./cmd/gift-videoproto -file bbb.mp4 [-hw] [-seconds 30]
//
// Keys: space pauses, left and right seek ten seconds, q quits.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/worldiety/gift/video"
	"github.com/worldiety/gift/video/ffmpeg"
)

//go:embed yuv.kage
var yuvKage []byte

//go:embed yuvdirect.kage
var yuvDirectKage []byte

var (
	flagFile    = flag.String("file", "", "video file")
	flagHW      = flag.Bool("hw", false, "hardware decoding where available")
	flagSeconds = flag.Float64("seconds", 0, "quit after this many seconds and print a summary; 0 plays to the end")
	flagFull    = flag.Bool("fullscreen", false, "fullscreen")
	flagLinear  = flag.Bool("chroma-linear", true, "bilinear chroma")
	flagVolume  = flag.Float64("volume", 0.5, "volume 0..1")
	flagSeek    = flag.Duration("seek-test", 0, "seek forward by this much every 5 s, to exercise seeking")
	flagOSD     = flag.Bool("osd", true, "draw the statistics on screen")
	flagHz      = flag.Float64("hz", 60, "display refresh rate, for counting missed refreshes")
	flagClear   = flag.Bool("clear", false, "direct mode: let Ebitengine clear the offscreen every frame")
	flagMarker  = flag.String("marker", "set", "direct mode: how the offscreen is marked modified: set, fill, none")
	flagVsync   = flag.Bool("vsync", true, "wait for the display refresh")
	flagMode    = flag.String("mode", "direct", "direct: convert and scale onto the framebuffer in one pass; full: convert into a target, then blit; noupload: decode, keep the first picture; nodraw: decode, draw nothing")
)

type game struct {
	p      *video.Player
	info   ffmpeg.Info
	shader *ebiten.Shader
	direct *ebiten.Shader
	yuv    *ebiten.Image
	cur    *ffmpeg.Frame // the picture DrawFinalScreen converts
	fused  bool          // convert and scale in one pass, or blit g.rgb
	rgb    *ebiten.Image

	actx   *audio.Context
	aplay  *audio.Player
	stream *video.AudioStream

	start    time.Time
	first    time.Time
	lastDraw time.Time
	lastSeek time.Time
	interval time.Duration // display refresh period, measured

	// per second
	sec       secStats
	total     secStats
	lastPrint time.Time
	osd       string
	hist      [7]int // draw intervals: <12, <18, <22, <28, <36, <45, more ms
}

type secStats struct {
	draws, uploads, repeats, missed int
	upload                          time.Duration
	maxDraw                         time.Duration
}

func main() {
	flag.Parse()
	if *flagFile == "" {
		log.Fatal("-file is required")
	}
	if !ffmpeg.Available() {
		_, err := ffmpeg.Open(*flagFile, ffmpeg.Options{})
		log.Fatal(err)
	}
	u, c, f := ffmpeg.Versions()
	p, err := video.Open(*flagFile, video.Options{HW: *flagHW})
	if err != nil {
		log.Fatal(err)
	}
	info := p.Info()
	log.Printf("libavutil %d libavcodec %d libavformat %d", u, c, f)
	log.Printf("%s: %s %dx%d SAR %d:%d DAR %.3f %.3f fps, hw=%q | audio %s %d Hz %d ch | %v",
		*flagFile, info.VideoCodec, info.Width, info.Height, info.SARNum, info.SARDen, info.DisplayAspect(),
		info.FrameRate, p.VideoHW(), info.AudioCodec, info.AudioRate, info.AudioChannels, info.Duration)

	sh, err := ebiten.NewShader(yuvKage)
	if err != nil {
		log.Fatal(err)
	}
	dsh, err := ebiten.NewShader(yuvDirectKage)
	if err != nil {
		log.Fatal(err)
	}
	g := &game{p: p, info: info, shader: sh, direct: dsh, actx: audio.NewContext(p.AudioRate()), start: time.Now()}
	p.SetVolume(*flagVolume)
	p.Play()

	ebiten.SetWindowTitle("gift video prototype")
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(*flagFull)
	ebiten.SetVsyncEnabled(*flagVsync)
	var run ebiten.Game = g
	if *flagMode == "direct" || *flagMode == "hybrid" || *flagMode == "auto" {
		ebiten.SetScreenClearedEveryFrame(*flagClear)
		run = directGame{g}
	}
	if err := ebiten.RunGame(run); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
	g.summary()
}

func (g *game) Update() error {
	p := g.p
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		if p.TimeControlStatus() == video.Paused {
			p.Play()
		} else {
			p.Pause()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		p.Seek(p.CurrentTime() + 10*time.Second)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		p.Seek(p.CurrentTime() - 10*time.Second)
	}
	if *flagSeek > 0 && time.Since(g.lastSeek) > 5*time.Second && time.Since(g.start) > 3*time.Second {
		g.lastSeek = time.Now()
		to := p.CurrentTime() + *flagSeek
		if to > g.info.Duration-5*time.Second {
			to = 0
		}
		log.Printf("seek %v -> %v", p.CurrentTime().Round(time.Millisecond), to)
		p.Seek(to)
	}
	if err := p.Err(); err != nil {
		return err
	}
	if *flagSeconds > 0 && time.Since(g.start).Seconds() > *flagSeconds {
		return ebiten.Termination
	}
	if p.TimeControlStatus() == video.Paused && p.CurrentTime() >= g.info.Duration-50*time.Millisecond && *flagSeconds == 0 {
		return ebiten.Termination
	}
	g.syncAudio()
	return nil
}

// syncAudio keeps one device player per audio stream and runs it when the
// player's clock runs.
func (g *game) syncAudio() {
	s := g.p.AudioStream()
	if s == nil {
		return
	}
	if s != g.stream {
		if g.aplay != nil {
			g.aplay.Close()
		}
		ap, err := g.actx.NewPlayerF32(s)
		if err != nil {
			log.Fatal(err)
		}
		ap.SetBufferSize(60 * time.Millisecond)
		s.SetPosition(ap.Position)
		g.aplay, g.stream = ap, s
	}
	switch g.p.TimeControlStatus() {
	case video.Playing:
		if !g.aplay.IsPlaying() {
			g.aplay.Play()
		}
	default:
		if g.aplay.IsPlaying() {
			g.aplay.Pause()
		}
	}
}

func (g *game) Draw(screen *ebiten.Image) {
	now := time.Now()
	if !g.lastDraw.IsZero() {
		d := now.Sub(g.lastDraw)
		g.interval = time.Duration(float64(time.Second) / *flagHz)
		if d > g.interval*3/2 {
			g.sec.missed++
		}
		g.sec.maxDraw = max(g.sec.maxDraw, d)
		ms := d.Seconds() * 1000
		b := 6
		for i, lim := range []float64{12, 18, 22, 28, 36, 45} {
			if ms < lim {
				b = i
				break
			}
		}
		g.hist[b]++
	}
	g.lastDraw = now
	if g.first.IsZero() {
		g.first = now
	}
	g.sec.draws++

	// The picture is chosen for the moment it reaches the screen, about
	// one refresh from now.
	f, changed := g.p.Frame(now.Add(g.interval))
	switch *flagMode {
	case "direct", "hybrid", "auto":
		if f != nil && changed {
			t0 := time.Now()
			g.ensureYUV(f)
			g.yuv.WritePixels(f.Pix)
			g.fused = *flagMode == "direct" || *flagMode == "auto" && g.oneToOne(f, screen.Bounds().Dx(), screen.Bounds().Dy())
			if !g.fused {
				g.upload(f) // converts into g.rgb at the picture's own size
			}
			g.sec.upload += time.Since(t0)
			g.sec.uploads++
		} else if f != nil && g.p.TimeControlStatus() == video.Playing {
			g.sec.repeats++
		}
		if f != nil && f.Interlaced && !g.fused {
			g.convert(f, now)
		}
		g.cur = f
		// Ebitengine presents only when the offscreen changed.
		switch *flagMarker {
		case "set":
			screen.Set(0, 0, color.Transparent)
		case "fill":
			screen.SubImage(image.Rect(0, 0, 1, 1)).(*ebiten.Image).Fill(color.Black)
		}
		g.stats(now, nil)
		return
	case "nodraw":
		g.stats(now, screen)
		return
	case "noupload":
		changed = changed && g.rgb == nil
	case "convertonly":
		// The shader runs every draw on the first picture, no upload.
		if g.rgb != nil {
			g.convert(f, now)
			g.present(screen, f)
			g.stats(now, screen)
			return
		}
	case "uploadonly":
		// Upload every picture, never convert after the first.
		if g.rgb != nil && f != nil && changed {
			g.yuv.WritePixels(f.Pix)
			g.present(screen, f)
			g.stats(now, screen)
			return
		}
	}
	if f != nil && changed {
		t0 := time.Now()
		g.upload(f)
		g.sec.upload += time.Since(t0)
		g.sec.uploads++
	} else if f != nil && g.p.TimeControlStatus() == video.Playing {
		g.sec.repeats++
	}
	if f != nil && f.Interlaced {
		g.convert(f, now)
	}
	if g.rgb != nil {
		g.present(screen, f)
	}
	g.stats(now, screen)
}

func (g *game) ensureYUV(f *ffmpeg.Frame) {
	tw, th := f.TextureSize()
	if g.yuv == nil || g.yuv.Bounds().Dx() != tw || g.yuv.Bounds().Dy() != th {
		if g.yuv != nil {
			g.yuv.Deallocate()
		}
		g.yuv = ebiten.NewImageWithOptions(rectWH(tw, th), &ebiten.NewImageOptions{Unmanaged: true})
	}
}

type directGame struct{ *game }

// DrawFinalScreen converts and scales the current picture onto the
// framebuffer in one pass.
func (d directGame) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, geoM ebiten.GeoM) {
	g := d.game
	screen.Clear()
	f := g.cur
	if f == nil || g.yuv == nil {
		return
	}
	if !g.fused && g.rgb != nil {
		sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
		vw, vh := float64(f.Width), float64(f.Height)
		sar := 1.0
		if g.info.SARNum > 0 && g.info.SARDen > 0 {
			sar = float64(g.info.SARNum) / float64(g.info.SARDen)
		}
		s := min(sw/(vw*sar), sh/vh)
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(s*sar, s)
		op.GeoM.Translate((sw-vw*sar*s)/2, (sh-vh*s)/2)
		screen.DrawImage(g.rgb, op)
		return
	}
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	vw, vh := float64(f.Width), float64(f.Height)
	sar := 1.0
	if g.info.SARNum > 0 && g.info.SARDen > 0 {
		sar = float64(g.info.SARNum) / float64(g.info.SARDen)
	}
	s := min(sw/(vw*sar), sh/vh)
	dw, dh := vw*sar*s, vh*s
	x0, y0 := (sw-dw)/2, (sh-dh)/2
	scaleX, scaleY := vw/dw, vh/dh // picture pixels per screen pixel
	r, gg, b := matrix(f.Matrix, f.FullRange)
	lumaLinear := float32(1)
	if abs(scaleX-1) < 1e-3 && abs(scaleY-1) < 1e-3 {
		lumaLinear = 0
	}
	chromaLinear := float32(0)
	if *flagLinear {
		chromaLinear = 1
	}
	X0, Y0, X1, Y1 := float32(x0), float32(y0), float32(x0+dw), float32(y0+dh)
	W, H := float32(vw), float32(vh)
	vs := []ebiten.Vertex{
		{DstX: X0, DstY: Y0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: X1, DstY: Y0, SrcX: W, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: X0, DstY: Y1, SrcX: 0, SrcY: H, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: X1, DstY: Y1, SrcX: W, SrcY: H, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &ebiten.DrawTrianglesShaderOptions{Blend: ebiten.BlendCopy}
	op.Images[0] = g.yuv
	op.Uniforms = map[string]any{
		"Layout": float32(f.Layout), "Height": float32(vh), "Width": float32(vw),
		"LumaLinear": lumaLinear, "ChromaLinear": chromaLinear,
		"RowR": r[:], "RowG": gg[:], "RowB": b[:],
	}
	screen.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, g.direct, op)
}

// oneToOne reports whether f is shown at its own size on a w by h screen,
// where converting straight onto the framebuffer saves a pass. Scaled up, the
// fused shader would interpolate luma itself at four fetches a pixel of the
// larger picture, while the two pass path converts the smaller one and lets
// the texture unit interpolate.
func (g *game) oneToOne(f *ffmpeg.Frame, w, h int) bool {
	sar := 1.0
	if g.info.SARNum > 0 && g.info.SARDen > 0 {
		sar = float64(g.info.SARNum) / float64(g.info.SARDen)
	}
	s := min(float64(w)/(float64(f.Width)*sar), float64(h)/float64(f.Height))
	return abs(s-1) < 1e-3 && abs(sar-1) < 1e-3
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func (g *game) upload(f *ffmpeg.Frame) {
	tw, th := f.TextureSize()
	if g.yuv == nil || g.yuv.Bounds().Dx() != tw || g.yuv.Bounds().Dy() != th {
		if g.yuv != nil {
			g.yuv.Deallocate()
		}
		g.yuv = ebiten.NewImageWithOptions(rectWH(tw, th), &ebiten.NewImageOptions{Unmanaged: true})
	}
	if g.rgb == nil || g.rgb.Bounds().Dx() != f.Width || g.rgb.Bounds().Dy() != f.Height {
		if g.rgb != nil {
			g.rgb.Deallocate()
		}
		g.rgb = ebiten.NewImageWithOptions(rectWH(f.Width, f.Height), &ebiten.NewImageOptions{Unmanaged: true})
	}
	g.yuv.WritePixels(f.Pix)
	if !f.Interlaced {
		g.convert(f, time.Time{})
	}
}

// convert runs the YUV shader into the RGB target. Interlaced pictures are
// converted every draw, because which field is shown depends on the time.
func (g *game) convert(f *ffmpeg.Frame, now time.Time) {
	field := 0.0
	if f.Interlaced {
		second := g.p.CurrentTime() >= f.PTS+f.Duration/2
		top := f.TopFieldFirst != second
		field = 2
		if top {
			field = 1
		}
	}
	r, gg, b := matrix(f.Matrix, f.FullRange)
	w, h := float32(f.Width), float32(f.Height)
	vs := []ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: 0, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: w, DstY: 0, SrcX: w, SrcY: 0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 0, DstY: h, SrcX: 0, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: w, DstY: h, SrcX: w, SrcY: h, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &ebiten.DrawTrianglesShaderOptions{Blend: ebiten.BlendCopy}
	op.Images[0] = g.yuv
	linear := 0.0
	if *flagLinear {
		linear = 1
	}
	op.Uniforms = map[string]any{
		"Layout": float32(f.Layout), "Height": h, "Field": float32(field),
		"ChromaLinear": float32(linear), "RowR": r[:], "RowG": gg[:], "RowB": b[:],
	}
	g.rgb.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, g.shader, op)
}

// present draws the RGB picture letterboxed at its display aspect ratio.
func (g *game) present(screen *ebiten.Image, f *ffmpeg.Frame) {
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	vw, vh := float64(g.rgb.Bounds().Dx()), float64(g.rgb.Bounds().Dy())
	sar := 1.0
	if g.info.SARNum > 0 && g.info.SARDen > 0 {
		sar = float64(g.info.SARNum) / float64(g.info.SARDen)
	}
	dw := vw * sar // display width in the picture's own height units
	s := min(sw/dw, sh/vh)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s*sar, s)
	op.GeoM.Translate((sw-dw*s)/2, (sh-vh*s)/2)
	screen.DrawImage(g.rgb, op)
}

func (g *game) stats(now time.Time, screen *ebiten.Image) {
	if g.lastPrint.IsZero() {
		g.lastPrint = now
	}
	if now.Sub(g.lastPrint) >= time.Second {
		st := g.p.Stats()
		ups := max(g.sec.uploads, 1)
		g.osd = fmt.Sprintf("t=%6.2fs fps=%5.1f draws=%d uploads=%d repeats=%d missed=%d maxdraw=%v upload=%v/frame | decoded=%d presented=%d late=%d underruns=%d av=%+v clk=%+v | cpu=%s",
			g.p.CurrentTime().Seconds(), ebiten.ActualFPS(), g.sec.draws, g.sec.uploads, g.sec.repeats, g.sec.missed,
			g.sec.maxDraw.Round(100*time.Microsecond), (g.sec.upload / time.Duration(ups)).Round(10*time.Microsecond),
			st.Decoded, st.Presented, st.Late, st.AudioUnderruns, st.AVOffset.Round(100*time.Microsecond),
			st.ClockError.Round(100*time.Microsecond), cpu())
		fmt.Println(g.osd)
		g.total.draws += g.sec.draws
		g.total.uploads += g.sec.uploads
		g.total.repeats += g.sec.repeats
		g.total.missed += g.sec.missed
		g.total.upload += g.sec.upload
		g.sec = secStats{}
		g.lastPrint = now
	}
	if *flagOSD && screen != nil {
		ebitenutil.DebugPrint(screen, strings.ReplaceAll(g.osd, " | ", "\n"))
	}
}

func (g *game) summary() {
	st := g.p.Stats()
	el := time.Since(g.start)
	fmt.Printf("SUMMARY %s: %.1fs, draws=%d (%.1f/s) uploads=%d repeats=%d missed=%d decoded=%d presented=%d late=%d underruns=%d upload=%v/frame cpu=%s\n",
		*flagFile, el.Seconds(), g.total.draws, float64(g.total.draws)/el.Seconds(), g.total.uploads, g.total.repeats,
		g.total.missed, st.Decoded, st.Presented, st.Late, st.AudioUnderruns,
		(g.total.upload / time.Duration(max(g.total.uploads, 1))).Round(10*time.Microsecond), cpuTotal(el))
	n := 0
	for _, c := range g.hist {
		n += c
	}
	fmt.Printf("RATE %.1f draws/s since the first draw\n", float64(n)/time.Since(g.first).Seconds())
	fmt.Printf("INTERVALS ms <12:%d 12-18:%d 18-22:%d 22-28:%d 28-36:%d 36-45:%d >45:%d\n",
		g.hist[0], g.hist[1], g.hist[2], g.hist[3], g.hist[4], g.hist[5], g.hist[6])
	g.p.Close()
}

func (g *game) Layout(w, h int) (int, int) { return w, h }

// matrix returns the rows that turn normalised (Y, U, V, 1) into R, G, B.
func matrix(m ffmpeg.Matrix, full bool) (r, g, b [4]float32) {
	kr, kb := 0.2126, 0.0722 // BT.709
	switch m {
	case ffmpeg.BT601:
		kr, kb = 0.299, 0.114
	case ffmpeg.BT2020:
		kr, kb = 0.2627, 0.0593
	}
	kg := 1 - kr - kb
	ys, cs, yo := 1.0, 1.0, 0.0
	if !full {
		ys, cs, yo = 255.0/219, 255.0/224, 16.0/255
	}
	const co = 128.0 / 255
	rv := 2 * (1 - kr) * cs
	bu := 2 * (1 - kb) * cs
	gu := -2 * kb * (1 - kb) / kg * cs
	gv := -2 * kr * (1 - kr) / kg * cs
	row := func(cu, cv float64) [4]float32 {
		return [4]float32{float32(ys), float32(cu), float32(cv), float32(-ys*yo - (cu+cv)*co)}
	}
	return row(0, rv), row(gu, gv), row(bu, 0)
}

// cpu is the CPU time of the process in the last interval, in percent of
// one core.
var lastCPU, lastWall = cpuTicks(), time.Now()

func cpu() string {
	t, w := cpuTicks(), time.Now()
	pct := float64(t-lastCPU) / 100 / w.Sub(lastWall).Seconds() * 100
	lastCPU, lastWall = t, w
	return fmt.Sprintf("%.0f%%", pct)
}

var startCPU = cpuTicks()

func cpuTotal(el time.Duration) string {
	return fmt.Sprintf("%.0f%%", float64(cpuTicks()-startCPU)/100/el.Seconds()*100)
}

// cpuTicks reads utime+stime from /proc/self/stat, in clock ticks of 10 ms.
func cpuTicks() int64 {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	s := string(b)
	s = s[strings.LastIndexByte(s, ')')+2:]
	f := strings.Fields(s)
	if len(f) < 13 {
		return 0
	}
	u, _ := strconv.ParseInt(f[11], 10, 64)
	k, _ := strconv.ParseInt(f[12], 10, 64)
	return u + k
}

func rectWH(w, h int) image.Rectangle { return image.Rect(0, 0, w, h) }
