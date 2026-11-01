package kiosk_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/kiosk"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// fixture is a kiosk on a simulated system in a window of the given size.
type fixture struct {
	h   *gifttest.Harness
	k   *kiosk.Kiosk
	sys system.System
}

func newKiosk(t *testing.T, size geom.Size, edit func(*kiosk.Config)) *fixture {
	t.Helper()
	sys := system.Simulated(0)
	cfg := kiosk.Config{
		Name: "werkbank", Title: "Werkbank", Version: "1.0.0", System: sys, DataDir: t.TempDir(),
		Updates: kiosk.Updates{System: true},
		App: func(ctx *gift.Context, k *kiosk.Kiosk) gift.View {
			return ui.Text("the program").Key("program")
		},
	}
	if edit != nil {
		edit(&cfg)
	}
	k, err := kiosk.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := gifttest.New(t, gifttest.Options{Root: k.View, Size: size,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	k.Attach(h.App())
	return &fixture{h: h, k: k, sys: sys}
}

// until runs frames until cond holds, as the system answers on goroutines.
func (f *fixture) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	// The system answers on goroutines, which a busy machine runs late.
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		f.h.AdvanceTicks(1, gifttest.FrameInterval)
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not happen:\n%s", what, f.h.Dump())
}

func (f *fixture) shows(key string) func() bool {
	return func() bool { return f.h.Exists(gifttest.ByKey(key)) }
}

func (f *fixture) tap(t *testing.T, key string) {
	t.Helper()
	f.until(t, key+" shown", f.shows(key))
	f.h.Find(gifttest.ByKey(key)).ScrollIntoView().Click()
	f.h.AdvanceTicks(2, gifttest.FrameInterval)
}

func TestTheGearOpensTheSettingsBesideTheListAndDoneCloses(t *testing.T) {
	f := newKiosk(t, geom.Sz(1100, 720), nil)
	f.h.AssertExists(gifttest.ByKey("program"))
	f.tap(t, "kiosk.entry")
	for _, k := range []string{"kiosk.device", "kiosk.row.wifi", "kiosk.row.ethernet", "kiosk.row.general", "kiosk.row.passcode"} {
		f.until(t, k, f.shows(k))
	}
	// The regular layout shows the first page beside the list: Wi-Fi with
	// the network the device is joined to.
	f.until(t, "the current network", f.shows("kiosk.wifi.current"))
	f.tap(t, "kiosk.done")
	f.h.AssertNone(gifttest.ByKey("kiosk.row.wifi"))
	f.h.AssertExists(gifttest.ByKey("program"))
}

func TestACompactWindowPushesThePagesOfTheList(t *testing.T) {
	f := newKiosk(t, geom.Sz(390, 700), nil)
	f.tap(t, "kiosk.entry")
	f.tap(t, "kiosk.row.general")
	f.tap(t, "kiosk.row.about")
	f.until(t, "the name of the device", func() bool {
		n := f.h.Find(gifttest.ByKey("kiosk.row.name"))
		return !n.IsZero() && n.Text() != ""
	})
	// The list stays below the pages it pushed, out of sight.
	if f.h.Find(gifttest.ByKey("kiosk.row.wifi")).IsVisible() {
		t.Fatal("the list is still in sight")
	}
}

func TestASecuredNetworkAsksForItsPasswordAgainWhenItIsWrong(t *testing.T) {
	f := newKiosk(t, geom.Sz(1100, 720), nil)
	f.tap(t, "kiosk.entry")
	f.tap(t, "kiosk.wifi.Halle 2")
	f.until(t, "the password field", f.shows("kiosk.wifi.password"))
	f.h.Find(gifttest.ByKey("kiosk.wifi.password")).Focus()
	f.h.TypeText("short")
	f.tap(t, "kiosk.wifi.join")
	f.until(t, "the password asked again", f.shows("kiosk.wifi.password"))
	f.h.Find(gifttest.ByKey("kiosk.wifi.password")).Focus()
	f.h.TypeText("a long password")
	f.tap(t, "kiosk.wifi.join")
	f.until(t, "joined", func() bool {
		ifs, _ := f.sys.Interfaces(context.Background())
		return ifs[1].SSID == "Halle 2"
	})
}

func TestAFixedAddressIsAppliedAndKeptOnlyWhenConfirmed(t *testing.T) {
	f := newKiosk(t, geom.Sz(1100, 720), nil)
	f.tap(t, "kiosk.entry")
	f.tap(t, "kiosk.row.ethernet")
	f.tap(t, "kiosk.eth.eth0")
	f.until(t, "the address field", f.shows("kiosk.ip.ipaddress"))
	set := func(key, text string) {
		f.h.Find(gifttest.ByKey(key)).ScrollIntoView().Focus()
		f.h.Key(gift.KeyA, gift.ShortcutModifier)
		f.h.SetModifiers() // the shortcut is let go before typing
		f.h.TypeText(text)
	}
	set("kiosk.ip.ipaddress", "10.20.1.50")
	set("kiosk.ip.subnetmask", "255.255.0.0")
	set("kiosk.ip.router", "10.20.0.1")
	f.tap(t, "kiosk.action.Apply")
	f.until(t, "the question to keep", func() bool { return f.h.Exists(gifttest.ByText("Keep")) })
	ifs, _ := f.sys.Interfaces(context.Background())
	if want := netip.MustParsePrefix("10.20.1.50/16"); ifs[0].Config.Address != want {
		t.Fatalf("eth0 is configured with %v, want %v", ifs[0].Config.Address, want)
	}
	f.h.Find(gifttest.ByText("Keep")).Click()
	f.h.AdvanceTicks(4, gifttest.FrameInterval)
	f.h.AssertNone(gifttest.ByText("Keep"))
}

func TestThePasscodeGuardsTheSettings(t *testing.T) {
	f := newKiosk(t, geom.Sz(1100, 720), func(c *kiosk.Config) { c.PIN = "1357" })
	f.tap(t, "kiosk.entry")
	f.until(t, "the passcode", f.shows("kiosk.pin"))
	for _, d := range "2468" {
		f.tap(t, "kiosk.pin."+string(d))
	}
	f.until(t, "the failure", func() bool {
		return f.h.Find(gifttest.ByKey("kiosk.pin.message")).Text() != ""
	})
	f.h.AssertNone(gifttest.ByKey("kiosk.row.wifi"))
	for _, d := range "1357" {
		f.tap(t, "kiosk.pin."+string(d))
	}
	f.until(t, "the settings", f.shows("kiosk.row.wifi"))
}

func TestSystemUpdatesAreListedAndInstalled(t *testing.T) {
	f := newKiosk(t, geom.Sz(1100, 720), nil)
	f.tap(t, "kiosk.entry")
	f.tap(t, "kiosk.row.general")
	f.tap(t, "kiosk.row.updates")
	f.tap(t, "kiosk.action.Check for Updates")
	f.until(t, "the packages", f.shows("kiosk.pkg.libc6"))
	f.tap(t, "kiosk.action.Install 5 Updates")
	f.until(t, "the updates installed", func() bool { return f.h.Exists(gifttest.ByText("The updates are installed.")) })
}
