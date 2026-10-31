// Command example-video plays a video with sound and the touch controls of
// ui/videoplayer, full window: the program to try on a Raspberry Pi with a
// five or seven inch touch panel.
//
//	go run ./cmd/example-video                        # choose a video in your home directory
//	go run ./cmd/example-video -sample                # the short clip gift brings along
//	go run ./cmd/example-video -file movie.mp4 -hw    # this one, HEVC in hardware on a Pi 5
//	go run ./cmd/example-video -dir /media/usb        # choose one on a stick
//	go run ./cmd/example-video -width 800 -height 480 # the size of a five inch panel
//
// It needs the FFmpeg libraries at run time, still without cgo: they are on
// Raspberry Pi OS already, and Homebrew's ffmpeg brings them to macOS.
//
// Without -file or -sample it opens with the file picker of ui/filepicker,
// which lists the videos below -dir, your home directory by default. Tap the
// picture to show or hide the controls; the close button at their top left
// goes back to the picker. The window draws straight into the framebuffer
// (backend.Config.DirectToScreen), which is what lets a Raspberry Pi 5 show
// full HD at 60 frames a second; see the project plan, section 25.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/internal/sampleclip"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/filepicker"
	"github.com/worldiety/gift/ui/videoplayer"
	"github.com/worldiety/gift/video"
)

// videoExtensions are the files the picker offers.
var videoExtensions = []string{".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".ts", ".mts", ".mpg", ".mpeg"}

func main() {
	file := flag.String("file", "", "the video to play; without it, and without -sample, a file picker opens")
	sample := flag.Bool("sample", false, "play the short clip gift brings along")
	dir := flag.String("dir", "", "the directory the file picker shows; the default is your home directory")
	hw := flag.Bool("hw", false, "decode in hardware where there is a decoder, HEVC on a Raspberry Pi 5")
	loop := flag.Bool("loop", true, "start over at the end")
	width := flag.Int("width", 1280, "window width")
	height := flag.Int("height", 720, "window height")
	full := flag.Bool("fullscreen", false, "fullscreen")
	autoHide := flag.Duration("autohide", 3*time.Second, "how long the controls stay after a touch while playing; 0 keeps them")
	stats := flag.Bool("stats", false, "print the frame rate and how the video was drawn every five seconds")
	flag.Parse()

	if err := example.LoadFont(); err != nil {
		fail(err)
	}
	start := *file
	if *sample {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		if start, err = sampleclip.Path(filepath.Join(cache, "gift", "video")); err != nil {
			fail(err)
		}
	}
	root := *dir
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fail(err)
		}
		root = home
	}

	s := &screen{root: root, opts: video.Options{HW: *hw, Loop: *loop}, autoHide: *autoHide}
	app := gift.New(gift.Options{Root: s.view})
	s.path = start
	ui.SetTheme(app, ui.DarkTheme())
	if *full {
		// gift's backend has no option for it; Ebitengine's own call is
		// the one a kiosk makes.
		eb.SetFullscreen(true)
	}
	cfg := backend.Config{
		Title:          "gift video",
		Width:          *width,
		Height:         *height,
		DirectToScreen: true,
		DrawOnDemand:   true,
	}
	if *stats {
		report(&cfg)
	}
	err := backend.Run(app, cfg)
	s.close()
	if err != nil {
		fail(err)
	}
}

// screen is the one screen of the program: the picker while no video is
// open, the player while one is.
type screen struct {
	root     string
	opts     video.Options
	autoHide time.Duration

	// path is the video to play, empty for the picker; player is it opened,
	// and failed what went wrong opening the last one.
	path   string
	player *video.Player
	failed string
}

func (s *screen) view(ctx *gift.Context) gift.View {
	// The video is chosen and opened outside of any state: the screen is
	// rebuilt through this one state when it changes.
	gen := ctx.State("gen", 0)
	ctx.Read(gen)
	changed := func() { gen.Set(gen.Get() + 1) }

	if s.path != "" && s.player == nil {
		p, err := video.Open(s.path, s.opts)
		if err != nil {
			s.failed = filepath.Base(s.path) + ": " + err.Error()
			s.path = ""
		} else {
			p.Play()
			s.player, s.failed = p, ""
		}
	}
	if s.player != nil {
		// The player is the root and not the content of a ui.Window: the
		// window would fill the screen with its background under the
		// picture, a full screen pass a Raspberry Pi pays for in every
		// frame.
		return videoplayer.New(s.player).AutoHide(s.autoHide).OnClose(func() {
			s.close()
			changed()
		})
	}

	picker := filepicker.New(os.DirFS(s.root), func(paths []string) {
		s.path = filepath.Join(s.root, filepath.FromSlash(paths[0]))
		changed()
	}, nil).
		Title("Choose a video").
		RootLabel(filepath.Base(s.root)).
		Extensions(videoExtensions...)
	content := []gift.View{picker.Flex(1)}
	if s.failed != "" {
		content = append([]gift.View{
			ui.Text(s.failed).Foreground(ui.ColorDanger).MaxLines(2).Padding(12),
		}, content...)
	}
	return ui.Window(ui.VStack(content...).Flex(1))
}

// close closes the player, if one is open, and goes back to the picker.
func (s *screen) close() {
	if s.player != nil {
		s.player.Close()
		s.player = nil
	}
	s.path = ""
}

// report prints, every five seconds, the frames drawn and the video
// operations, and how many of those were converted straight into the frame.
func report(cfg *backend.Config) {
	var r *backend.Renderer
	var last backend.RendererStats
	lastAt := time.Now()
	cfg.OnRenderer = func(rr *backend.Renderer) { r = rr }
	cfg.OnUpdate = func() error {
		if time.Since(lastAt) < 5*time.Second {
			return nil
		}
		s := r.Stats()
		el := time.Since(lastAt).Seconds()
		fmt.Printf("%.1f frames/s, %d video ops, %d of them direct\n",
			float64(s.Frames-last.Frames)/el, s.VideoOps-last.VideoOps, s.VideoDirectOps-last.VideoDirectOps)
		last, lastAt = s, time.Now()
		return nil
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "example-video:", err)
	os.Exit(1)
}
