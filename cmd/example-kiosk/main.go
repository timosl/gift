// Command example-kiosk is a gift program as the operating system of a
// device: a small counter of workpieces for a workbench, and the settings of
// the device after those of iOS, with Wi-Fi, Ethernet with fixed addresses,
// software updates, storage, the startup and a passcode.
//
//	go run ./cmd/example-kiosk -simulate            # a simulated Raspberry Pi
//	go run ./cmd/example-kiosk                      # the system it runs on
//
// Started on the desktop of a Raspberry Pi or of Ubuntu, it runs in a window
// without root and only shows the system; Settings, General, Startup offers
// to install it as the kiosk, after which the device starts into it full
// screen, as root. -simulate pretends instead, on any system, and changes
// nothing. -compact and -regular fix the layout of the settings, as on a
// 5 inch and a 7 inch panel.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/kiosk"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

func main() {
	simulate := flag.Bool("simulate", false, "pretend to run on a Raspberry Pi, changing nothing")
	compact := flag.Bool("compact", false, "lay the settings out as on a 5 inch panel")
	regular := flag.Bool("regular", false, "lay the settings out as on a 7 inch panel")
	pin := flag.String("pin", "", "a passcode set by the program, which protects the settings")
	flag.Parse()

	if err := example.LoadFont(); err != nil {
		fmt.Fprintln(os.Stderr, "example-kiosk:", err)
		os.Exit(1)
	}
	cfg := kiosk.Config{
		Name: "werkbank", Title: "Werkbank", Version: "1.0.0",
		App:     workbench,
		Updates: kiosk.Updates{System: true, Release: true},
		PIN:     *pin,
		Pages: []kiosk.Page{{
			Title: "Counter", Icon: outline.ChartMixed, Protected: true,
			View: counterSettings,
		}},
	}
	switch {
	case *compact:
		cfg.SizeClass = kiosk.Compact
	case *regular:
		cfg.SizeClass = kiosk.Regular
	}
	if *simulate {
		cfg.System = system.Simulated(300 * time.Millisecond)
	}
	if err := kiosk.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "example-kiosk:", err)
		os.Exit(1)
	}
}

// goal is the number of workpieces of a shift, which the settings of the
// program change.
var goal = 120

// workbench is the program: a counter of finished workpieces.
func workbench(ctx *gift.Context, k *kiosk.Kiosk) gift.View {
	count := ctx.State("count", 0)
	ctx.Read(count)
	return ui.VStack(
		ui.Text("Workpieces").FontSize(22).Foreground(ui.ColorSecondaryLabel),
		ui.Text(fmt.Sprint(count.Get())).FontSize(96).Key("count"),
		ui.Text(fmt.Sprintf("of %d this shift", goal)).FontSize(17).Foreground(ui.ColorSecondaryLabel),
		ui.HStack(
			ui.Button(ui.Text("−").FontSize(32), func() { count.Set(max(count.Get()-1, 0)) }).Frame(96, 72).Key("minus"),
			ui.Button(ui.Text("+").FontSize(32), func() { count.Set(count.Get() + 1) }).Frame(160, 72).Key("plus"),
		).Gap(16),
	).Gap(12).Align(geom.Center).Frame(geom.Unbounded(), geom.Unbounded()).Background(ui.ColorBackground)
}

// counterSettings is the page of the program in the settings.
func counterSettings(ctx *gift.Context, k *kiosk.Kiosk) gift.View {
	g := ctx.State("goal", goal)
	ctx.Read(g)
	return ui.VStack(
		ui.Text("GOAL").FontSize(13).Foreground(ui.ColorSecondaryLabel),
		ui.List(ui.Row("Workpieces per Shift").Accessory(ui.Stepper(float64(g.Get()), func(v float64) {
			goal = int(v)
			g.Set(goal)
		}).Range(10, 500).Step(10))).Background(ui.ColorSurface).CornerRadius(10),
	).Gap(6).Align(geom.Alignment{X: 0, Y: 0})
}
