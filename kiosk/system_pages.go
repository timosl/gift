package kiosk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// updateRun is an installation under way or finished.
type updateRun struct {
	progress system.Progress
	done     bool
	err      error
}

func (k *Kiosk) updatesPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	var kids []gift.View
	if k.cfg.Updates.App && k.cfg.AppUpdater != nil {
		kids = append(kids, gift.Component("app", k.appUpdates))
	}
	if k.cfg.Updates.System {
		kids = append(kids, gift.Component("system", k.systemUpdates))
	}
	if k.cfg.Updates.Release {
		kids = append(kids, gift.Component("release", k.releaseUpgrade))
	}
	if len(kids) == 0 {
		kids = append(kids, status("The updates of this device are managed by its supplier.", false))
	}
	return scroll(kids...)
}

// appUpdates looks for a new version of the program and installs it in its
// place, then starts it again.
func (k *Kiosk) appUpdates(ctx *gift.Context) gift.View {
	rel := ctx.State("release", (*AppRelease)(nil))
	msg := ctx.State("msg", "")
	busy := ctx.State("busy", false)
	frac := ctx.State("frac", -1.0)
	ctx.Read(rel)
	ctx.Read(msg)
	ctx.Read(busy)
	ctx.Read(frac)
	check := func() {
		busy.Set(true)
		msg.Set("Checking…")
		go func() {
			r, err := k.cfg.AppUpdater.Check(context.Background())
			k.post(func() {
				busy.Set(false)
				if err != nil {
					msg.Set(errText(err))
					return
				}
				if !newer(r.Version, k.cfg.Version) {
					msg.Set(k.cfg.Title + " is up to date.")
					rel.Set(nil)
					return
				}
				msg.Set("")
				rel.Set(&r)
			})
		}()
	}
	install := func() {
		r := rel.Get()
		busy.Set(true)
		go func() {
			err := k.installApp(context.Background(), *r, func(f float64) { k.post(func() { frac.Set(f) }) })
			k.post(func() {
				busy.Set(false)
				frac.Set(-1)
				if err != nil {
					msg.Set(errText(err))
				}
			})
		}()
	}
	rows := []gift.View{value(k.cfg.Title, k.cfg.Version)}
	if r := rel.Get(); r != nil {
		rows = append(rows, ui.Row("Version "+r.Version).Subtitle(r.Notes).Key("kiosk.app.release"))
		if f := frac.Get(); f >= 0 {
			rows = append(rows, ui.Row("").Accessory(ui.ProgressBar(f).Flex(1)))
		}
		rows = append(rows, action("Download and Install", false, busy.Get(), install))
	} else {
		rows = append(rows, action("Check for Updates", false, busy.Get(), check))
	}
	return ui.VStack(section("This Program", "", rows...), status(msg.Get(), false)).Gap(6)
}

// installApp downloads a version of the program, puts it in the place of
// the running one and restarts the kiosk.
func (k *Kiosk) installApp(ctx context.Context, r AppRelease, progress func(float64)) error {
	st, err := k.sys.Startup(ctx)
	if err != nil {
		return err
	}
	if !st.Installed || st.Executable == "" {
		return fmt.Errorf("Install the program as the kiosk first; see Startup.")
	}
	path, err := k.cfg.AppUpdater.Download(ctx, r, filepath.Dir(st.Executable), progress)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return err
	}
	if err := os.Rename(path, st.Executable); err != nil {
		os.Remove(path)
		return err
	}
	// The unit starts the program again when it ends.
	if st.Kiosk {
		k.post(func() { os.Exit(0) })
	}
	return nil
}

// systemUpdates checks for and installs the updates of the packages of the
// operating system.
func (k *Kiosk) systemUpdates(ctx *gift.Context) gift.View {
	pkgs := ctx.State("pkgs", (*[]system.Package)(nil))
	run := ctx.State("run", (*updateRun)(nil))
	msg := ctx.State("msg", "")
	busy := ctx.State("busy", false)
	ctx.Read(pkgs)
	ctx.Read(run)
	ctx.Read(msg)
	ctx.Read(busy)
	info := use(ctx, k, "info", k.sys.Info)
	can := k.sys.Can(system.CanUpdates)
	check := func() {
		busy.Set(true)
		msg.Set("Checking for updates…")
		go func() {
			p, err := k.sys.CheckUpdates(context.Background())
			k.post(func() {
				busy.Set(false)
				msg.Set(errText(err))
				if err == nil {
					pkgs.Set(&p)
					if len(p) == 0 {
						msg.Set(k.sys.Platform().Name + " is up to date.")
					}
				}
			})
		}()
	}
	install := func() {
		r := &updateRun{}
		run.Set(r)
		busy.Set(true)
		go func() {
			err := k.sys.InstallUpdates(context.Background(), func(p system.Progress) {
				k.post(func() { run.Set(&updateRun{progress: p}) })
			})
			k.post(func() {
				busy.Set(false)
				run.Set(&updateRun{done: true, err: err, progress: system.Progress{Fraction: 1}})
				if err == nil {
					pkgs.Set(nil)
				}
				info.reload()
			})
		}()
	}
	plat := k.sys.Platform()
	rows := []gift.View{value(plat.Name, plat.Version)}
	if r := run.Get(); r != nil && !r.done {
		rows = append(rows, ui.Row(r.progress.Status).Key("kiosk.update.status"),
			ui.Row("").Accessory(ui.ProgressBar(r.progress.Fraction).Flex(1).Key("kiosk.update.progress")))
	} else if p := pkgs.Get(); p != nil && len(*p) > 0 {
		for _, pk := range *p {
			rows = append(rows, ui.Row(pk.Name).Subtitle(pk.Current+" → "+pk.Candidate).Key("kiosk.pkg."+pk.Name))
		}
		rows = append(rows, action(fmt.Sprintf("Install %d Updates", len(*p)), false, busy.Get() || !can, install))
	} else {
		rows = append(rows, action("Check for Updates", false, busy.Get() || !can, check))
	}
	footer := ""
	if plat.Kind == system.RaspberryPiOS {
		footer = "Raspberry Pi OS installs its updates with full-upgrade, as Raspberry Pi recommends."
	}
	kids := []gift.View{section("System", footer, rows...)}
	if r := run.Get(); r != nil && r.done {
		if r.err != nil {
			kids = append(kids, status(errText(r.err), true))
		} else {
			kids = append(kids, status("The updates are installed.", false))
		}
	}
	if info.val.RebootRequired {
		kids = append(kids, section("", "Some updates take effect after a restart.",
			action("Restart Now", false, !k.sys.Can(system.CanPower), func() {
				k.do(background(k.sys.Reboot), func(err error) { msg.Set(errText(err)) })
			})))
	}
	kids = append(kids, status(msg.Get(), false))
	return ui.VStack(kids...).Gap(6)
}

// releaseUpgrade offers the upgrade to the next release of the operating
// system, where it is supported in place.
func (k *Kiosk) releaseUpgrade(ctx *gift.Context) gift.View {
	rel := use(ctx, k, "release", k.sys.Release)
	msg := ctx.State("msg", "")
	confirm := ctx.State("confirm", false)
	busy := ctx.State("busy", false)
	ctx.Read(msg)
	ctx.Read(confirm)
	ctx.Read(busy)
	r := rel.val
	var rows []gift.View
	switch {
	case !rel.done:
		rows = append(rows, ui.Row("Checking…"))
	case r.Available:
		rows = append(rows, value("Available", r.Name),
			action("Upgrade to "+r.Name, false, busy.Get() || !r.Supported || !k.sys.Can(system.CanReleaseUpgrade),
				func() { confirm.Set(true) }))
	default:
		rows = append(rows, value("New Release", "None"))
	}
	if confirm.Get() {
		rows = []gift.View{
			ui.Row("Upgrade to " + r.Name + "?").Subtitle("It takes long, and the device restarts when it is done. Do not switch it off meanwhile."),
			action("Upgrade", false, false, func() {
				confirm.Set(false)
				busy.Set(true)
				msg.Set("Upgrading… This takes a long time.")
				k.do(background(func(ctx context.Context) error { return k.sys.UpgradeRelease(ctx, nil) }), func(err error) {
					busy.Set(false)
					if err != nil {
						msg.Set(errText(err))
						return
					}
					msg.Set("The upgrade is done; restart the device.")
				})
			}),
			action("Cancel", false, false, func() { confirm.Set(false) }),
		}
	}
	return ui.VStack(section("Release Upgrade", r.Note, rows...), status(msg.Get(), false)).Gap(6)
}

func (k *Kiosk) startupPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	st := use(ctx, k, "startup", k.sys.Startup)
	msg := ctx.State("msg", "")
	busy := ctx.State("busy", false)
	ctx.Read(msg)
	ctx.Read(busy)
	s := st.val
	setBoot := func(b system.Boot, now bool) func() {
		return func() {
			busy.Set(true)
			k.do(background(func(ctx context.Context) error { return k.setBoot(ctx, b, now) }), func(err error) {
				busy.Set(false)
				msg.Set(errText(err))
				st.reload()
			})
		}
	}
	check := func(on bool) gift.View {
		if !on {
			return ui.Box().Frame(18, 18)
		}
		return ui.Text("✓").FontSize(17).Foreground(ui.ColorAccent)
	}
	var kids []gift.View
	if s.Installed {
		kids = append(kids, section("Start Into", "",
			ui.Row(k.cfg.Title).Subtitle("Full screen, as the kiosk of this device").Accessory(check(s.Boot == system.BootKiosk)).
				Disabled(!k.canInstall() || busy.Get()).OnTap(setBoot(system.BootKiosk, false)).Key("kiosk.boot.kiosk"),
			ui.Row("Desktop").Subtitle("The desktop of "+k.sys.Platform().Name).Accessory(check(s.Boot == system.BootDesktop)).
				Disabled(!k.canInstall() || busy.Get() || !s.Desktop).OnTap(setBoot(system.BootDesktop, false)).Key("kiosk.boot.desktop"),
		))
		if s.Kiosk {
			kids = append(kids, section("", "Ends the kiosk and starts the desktop now. The setting above decides what the device starts into the next time.",
				action("Switch to the Desktop Now", false, !k.canInstall() || !s.Desktop || busy.Get(), setBoot(system.BootDesktop, true))))
		} else {
			kids = append(kids, section("", "Ends the desktop and starts the kiosk now.",
				action("Start the Kiosk Now", false, !k.canInstall() || busy.Get(), setBoot(system.BootKiosk, true))))
		}
		kids = append(kids, section("Installed", "", value("Program", s.Executable)))
	} else {
		footer := fmt.Sprintf("Copies %s to /usr/local/bin and starts it full screen, as root, every time the device starts, in the place of the desktop. Nothing else is changed, and the desktop can be chosen here again.", k.cfg.Title)
		install := func() {
			busy.Set(true)
			msg.Set("Installing…")
			go func() {
				err := k.install(context.Background())
				k.post(func() {
					busy.Set(false)
					msg.Set(errText(err))
					if err == nil {
						msg.Set("Installed. The device starts into the kiosk from now on.")
					}
					st.reload()
				})
			}()
		}
		kids = append(kids, section("Kiosk", footer,
			action("Install as Kiosk", false, busy.Get() || !k.canInstall(), install)))
		if !k.canInstall() {
			kids = append(kids, status("Installing needs Linux with systemd; macOS is only viewed.", false))
		}
	}
	kids = append(kids, status(msg.Get(), false))
	return scroll(kids...)
}

func (k *Kiosk) passcodePage(ctx *gift.Context, n *navigator, _ string) gift.View {
	setting := ctx.State("setting", false)
	msg := ctx.State("msg", "")
	rev := ctx.State("rev", 0)
	ctx.Read(setting)
	ctx.Read(msg)
	ctx.Read(rev)
	if k.cfg.PIN != "" {
		return scroll(section("", "The passcode of this device is set by the program and cannot be changed here.",
			value("Passcode", fmt.Sprintf("%d digits", len(k.cfg.PIN)))))
	}
	has := k.store.digits("") > 0
	var kids []gift.View
	if has {
		kids = append(kids, section("", "", action("Change Passcode", false, false, func() { setting.Set(true) }),
			action("Turn Passcode Off", true, false, func() {
				msg.Set(errText(k.store.setPIN("")))
				rev.Set(rev.Get() + 1)
			})))
		prot := k.store.protects()
		toggle := func(label string, a Area) gift.View {
			return ui.Row(label).Accessory(ui.Toggle(prot&a != 0, func(on bool) {
				p := k.store.protects()
				if on {
					p |= a
				} else {
					p &^= a
				}
				if p == 0 {
					p = AreaSettings
				}
				msg.Set(errText(k.store.setProtects(p)))
				rev.Set(rev.Get() + 1)
			}).Key("kiosk.protect." + label))
		}
		rows := []gift.View{toggle("Settings", AreaSettings), toggle("Network", AreaNetwork),
			toggle("Software Update", AreaUpdates), toggle("System", AreaSystem)}
		if len(k.cfg.Pages) > 0 {
			rows = append(rows, toggle(k.cfg.Title, AreaApp))
		}
		kids = append(kids, section("Require Passcode For", "Settings asks for the passcode before anything opens; the others ask before their pages.", rows...))
	} else {
		kids = append(kids, section("", "A passcode keeps the settings from guests. After four wrong attempts it waits, longer each time.",
			action("Turn Passcode On", false, false, func() { setting.Set(true) })))
	}
	kids = append(kids, status(msg.Get(), true))
	page := scroll(kids...)
	if !setting.Get() {
		return page
	}
	return ui.ZStack(fill{page}, fill{gift.Component("setup", func(ctx *gift.Context) gift.View {
		return k.pinSetup(ctx, func(pin string) {
			setting.Set(false)
			if pin != "" {
				msg.Set(errText(k.store.setPIN(pin)))
			}
			rev.Set(rev.Get() + 1)
		})
	})})
}
