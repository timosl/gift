package kiosk

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// actionEnv names what the program does as root instead of opening a
// window: what the settings run through pkexec when the program runs on the
// desktop without root. pkexec passes no environment of the caller, so it
// runs env with the variable set, and no argument reaches the flags of the
// program.
const actionEnv = "GIFT_KIOSK_ACTION"

// The actions of actionEnv.
const (
	actionInstall    = "install"
	actionKiosk      = "kiosk"
	actionDesktop    = "desktop"
	actionKioskNow   = "kiosk-now"
	actionDesktopNow = "desktop-now"
)

// Run runs the kiosk: full screen when it runs as the kiosk of the device,
// in a window on a desktop otherwise. It returns when the window closes.
//
// On a desktop the program runs without root. Its settings then only show
// the system, and Startup offers to install it as the kiosk, which asks for
// the password of an administrator through pkexec and runs the program
// again as root, without a window; Run is where that run ends, so a
// program calls it before it does anything else that needs a display.
func Run(cfg Config) error {
	if a := os.Getenv(actionEnv); a != "" {
		return actHere(cfg, a)
	}
	k, err := New(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	st, _ := k.sys.Startup(ctx)
	disp, _ := k.sys.Display(ctx)
	cancel()

	bc := backend.Config{Title: cfg.Title, Width: cmp.Or(cfg.Width, 1100), Height: cmp.Or(cfg.Height, 720),
		X11ClassName: cfg.Name, X11InstanceName: cfg.Name, DrawOnDemand: true, IdleTPS: 30}
	// A simulated system only pretends to be the kiosk; the program runs in
	// a window like any other.
	if st.Kiosk && k.sys.Platform().Kind != system.SimulatedKind {
		// A kiosk has a touch screen and no keyboard, shows nothing but the
		// program, and has a screen whose size says how dense it is.
		ui.SetOnScreenKeyboard(nil, true)
		ebiten.SetFullscreen(true)
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
		bc.DirectToScreen = true
		bc.Density = density(disp)
	}
	app := gift.New(gift.Options{Root: k.View})
	k.Attach(app)
	k.applyAppearance()
	return backend.Run(app, bc)
}

// density is the density of a panel by its size: 2 for a screen of 5.5
// inches or less, whose 1280 by 720 pixels are too fine for a finger at 1,
// and 1 for the 7 and 10 inch panels. Unknown sizes keep the monitor's.
func density(d system.Display) float64 {
	in := d.Diagonal()
	switch {
	case in <= 0:
		return 0
	case in <= 5.5:
		return 2
	}
	return 1
}

// actHere does an action as root and returns: the second half of
// [Kiosk.privileged] when the program runs on a desktop.
func actHere(cfg Config, action string) error {
	if err := system.CheckName(cfg.Name); err != nil {
		return err
	}
	sys := cfg.System
	if sys == nil {
		sys = system.Open()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	switch action {
	case actionInstall:
		return sys.Install(ctx, system.Installation{Name: cfg.Name, Boot: system.BootKiosk})
	case actionKiosk:
		return sys.SetBoot(ctx, system.BootKiosk, false)
	case actionDesktop:
		return sys.SetBoot(ctx, system.BootDesktop, false)
	case actionKioskNow:
		return sys.SetBoot(ctx, system.BootKiosk, true)
	case actionDesktopNow:
		return sys.SetBoot(ctx, system.BootDesktop, true)
	}
	return fmt.Errorf("kiosk: unknown action %q", action)
}

// canInstall reports whether the settings can install the program as the
// kiosk: as root through the system, or on a Linux desktop through pkexec.
func (k *Kiosk) canInstall() bool {
	if k.sys.Platform().Kind == system.SimulatedKind {
		return true
	}
	if k.sys.Platform().Root {
		return k.sys.Can(system.CanInstall)
	}
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("pkexec")
	return err == nil
}

// install installs the program as the kiosk: directly as root, or
// through pkexec.
func (k *Kiosk) install(ctx context.Context) error {
	if k.sys.Platform().Root || k.sys.Platform().Kind == system.SimulatedKind {
		return k.sys.Install(ctx, system.Installation{Name: k.cfg.Name, Boot: system.BootKiosk})
	}
	return k.privileged(ctx, actionInstall)
}

// setBoot chooses what the device starts into: directly as root, or
// through pkexec.
func (k *Kiosk) setBoot(ctx context.Context, b system.Boot, now bool) error {
	if k.sys.Platform().Root || k.sys.Platform().Kind == system.SimulatedKind {
		return k.sys.SetBoot(ctx, b, now)
	}
	a := map[[2]bool]string{{false, false}: actionDesktop, {false, true}: actionDesktopNow,
		{true, false}: actionKiosk, {true, true}: actionKioskNow}[[2]bool{b == system.BootKiosk, now}]
	return k.privileged(ctx, a)
}

// privileged runs the program again as root through pkexec, which asks for
// the password of an administrator, to do action; see actionEnv.
func (k *Kiosk) privileged(ctx context.Context, action string) error {
	if !k.canInstall() {
		return system.ErrReadOnly
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "pkexec", "/usr/bin/env", actionEnv+"="+action, exe)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return errors.New("This needs the password of an administrator.")
		}
		msg := strings.TrimSpace(out.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// applyAppearance installs the light or the dark theme as the settings
// chose it.
func (k *Kiosk) applyAppearance() {
	k.mu.Lock()
	app := k.app
	k.mu.Unlock()
	if app == nil {
		return
	}
	if k.store.appearance() == "dark" {
		ui.SetTheme(app, ui.DarkTheme())
	} else {
		ui.SetTheme(app, ui.LightTheme())
	}
}
