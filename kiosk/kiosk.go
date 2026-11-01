package kiosk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// Config configures a kiosk.
type Config struct {
	// Name identifies the kiosk: the program becomes /usr/local/bin/<Name>,
	// its unit <Name>-kiosk.service, its data /var/lib/<Name>. Lower case
	// letters, digits and hyphens, starting with a letter.
	Name string
	// Title is the name of the device as the settings show it, Name if
	// empty, and Version the version of the program.
	Title, Version string

	// App is the program: the view the kiosk shows when the settings are
	// closed.
	App func(ctx *gift.Context, k *Kiosk) gift.View
	// Pages are settings of the program, listed after those of the system.
	Pages []Page

	// System is the operating system; nil opens the one the program runs
	// on.
	System system.System

	// PIN, if set, is the PIN of the device as the program sets it; the
	// settings cannot change it. Without it the settings offer to set one.
	PIN string
	// Protect says what the PIN protects. With a PIN and nothing chosen it
	// protects the settings as a whole.
	Protect Area

	// Updates says which updates the settings offer, and AppUpdater where
	// the program finds new versions of itself.
	Updates    Updates
	AppUpdater AppUpdater

	// SizeClass fixes the layout of the settings; the zero value chooses by
	// the width of the window.
	SizeClass SizeClass
	// Entry is how the settings are opened from the program.
	Entry Entry

	// DataDir is where the kiosk keeps its state: /var/lib/<Name> as root,
	// the user's configuration directory otherwise.
	DataDir string
}

// Page is a page of settings of the program.
type Page struct {
	// Title names the page in the list, Icon and Tint draw its tile.
	Title string
	Icon  ui.Symbol
	Tint  ui.Color
	// Protected pages ask for the PIN when [AreaApp] is protected.
	Protected bool
	// View builds the page.
	View func(ctx *gift.Context, k *Kiosk) gift.View
}

// Area is a part of the settings the PIN can protect; areas combine with |.
type Area uint16

// The areas of [Config.Protect].
const (
	// AreaSettings asks for the PIN before the settings open at all.
	AreaSettings Area = 1 << iota
	// AreaNetwork protects Wi-Fi and Ethernet.
	AreaNetwork
	// AreaUpdates protects Software Update.
	AreaUpdates
	// AreaSystem protects the startup, the power, the name, the date and
	// time, the display and the storage.
	AreaSystem
	// AreaApp protects the pages of the program marked Protected.
	AreaApp
)

// Updates says which updates the settings offer. Nothing is offered unless
// it is allowed here.
type Updates struct {
	// App offers new versions of the program from the AppUpdater.
	App bool
	// System offers the updates of the operating system's packages.
	System bool
	// Release offers an upgrade to a new release of the operating system,
	// where the system supports one in place.
	Release bool
}

// SizeClass is the layout of the settings, after iOS.
type SizeClass uint8

// The size classes of [Config.SizeClass].
const (
	// Automatic is Compact in a window narrower than RegularWidth, Regular
	// otherwise.
	Automatic SizeClass = iota
	// Compact is one page at a time, pushed and popped as on an iPhone: a
	// screen of 5 inches or less.
	Compact
	// Regular is the list of pages beside the page, as on an iPad: a screen
	// of 7 inches or more.
	Regular
)

// RegularWidth is the width of a window, in logical pixels, from which the
// settings take the Regular layout.
const RegularWidth = 680

// Entry is how the settings are opened from the program.
type Entry uint8

// The entries of [Config.Entry].
const (
	// EntryButton puts a small gear in the top right corner.
	EntryButton Entry = iota
	// EntryCorner opens the settings after five taps within two seconds in
	// the top left corner, which a guest does not find.
	EntryCorner
	// EntryNone leaves it to the program, which calls
	// [Kiosk.OpenSettings].
	EntryNone
)

// Kiosk is a gift program as the operating system of a device: the program,
// and settings after those of iOS for the device it runs on. It is made by
// [New] and shown by [Kiosk.View], or by [Run] in a window of its own.
type Kiosk struct {
	cfg Config
	sys system.System

	mu      sync.Mutex
	app     *gift.App
	pending []func()

	store *store

	// The navigation of the settings, which the program opens and closes
	// from event handlers.
	open   *gift.State[bool]
	path   *gift.State[string]
	toggle func(bool)
}

// New returns the kiosk of cfg.
func New(cfg Config) (*Kiosk, error) {
	if err := system.CheckName(cfg.Name); err != nil {
		return nil, err
	}
	if cfg.Title == "" {
		cfg.Title = cfg.Name
	}
	if cfg.System == nil {
		cfg.System = system.Open()
	}
	if cfg.PIN != "" && cfg.Protect == 0 {
		cfg.Protect = AreaSettings
	}
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir(cfg.Name, cfg.System.Platform().Root)
	}
	k := &Kiosk{cfg: cfg, sys: cfg.System}
	k.store = openStore(filepath.Join(cfg.DataDir, "kiosk.json"))
	return k, nil
}

func defaultDataDir(name string, root bool) string {
	if root {
		return filepath.Join("/var/lib", name)
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, name)
	}
	return filepath.Join(os.TempDir(), name)
}

// System returns the operating system of the kiosk.
func (k *Kiosk) System() system.System { return k.sys }

// Config returns the configuration of the kiosk.
func (k *Kiosk) Config() Config { return k.cfg }

// Attach connects the kiosk to the app that shows it, through which it hands
// the results of its background work to the user interface. [Run] does it;
// a program or a test that makes the app itself calls it once.
func (k *Kiosk) Attach(app *gift.App) {
	k.mu.Lock()
	k.app = app
	pending := k.pending
	k.pending = nil
	k.mu.Unlock()
	for _, fn := range pending {
		app.Post(fn)
	}
}

// post runs fn on the UI goroutine, as soon as there is an app.
func (k *Kiosk) post(fn func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.app == nil {
		k.pending = append(k.pending, fn)
		return
	}
	k.app.Post(fn)
}

// OpenSettings opens the settings, asking for the PIN first if it protects
// them. It is for [EntryNone] and must be called on the UI goroutine, from
// an event handler.
func (k *Kiosk) OpenSettings() {
	if k.toggle != nil {
		k.toggle(true)
	}
}

// CloseSettings closes the settings and returns to the program.
func (k *Kiosk) CloseSettings() {
	if k.toggle != nil {
		k.toggle(false)
	}
}

// do runs fn on a goroutine and hands its error to then on the UI
// goroutine.
func (k *Kiosk) do(fn func(ctx context.Context) error, then func(error)) {
	go func() {
		err := fn(context.Background())
		k.post(func() { then(err) })
	}()
}

// errText is an error as a sentence for the settings.
func errText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, system.ErrReadOnly):
		return "This system can only be viewed here."
	case errors.Is(err, system.ErrUnsupported):
		return "This system does not support this."
	case errors.Is(err, system.ErrWrongPassword):
		return "The password is wrong."
	}
	return err.Error()
}
