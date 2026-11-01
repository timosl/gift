package kiosk

import (
	"context"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// value is a row of a label and a value, as the About page has them.
func value(title, v string) gift.View { return ui.Row(title).Value(v) }

// status is a line under a section that says what happened, or nothing.
func status(msg string, bad bool) gift.View {
	if msg == "" {
		return ui.Box().Frame(0, 0)
	}
	fg := ui.ColorSecondaryLabel
	if bad {
		fg = ui.ColorDanger
	}
	return ui.Text(msg).FontSize(13).Foreground(fg).PaddingInsets(geom.Insets{Left: 16, Right: 16}).Key("kiosk.status")
}

// action is a row that runs something, in the colour of a link, or red when
// it destroys.
func action(title string, destructive, disabled bool, fn func()) gift.View {
	fg := ui.ColorAccent
	if destructive {
		fg = ui.ColorDanger
	}
	if disabled {
		fg = ui.ColorSecondaryLabel
		fn = nil
	}
	r := ui.Row("").Accessory(ui.Text(title).FontSize(17).Foreground(fg)).Key("kiosk.action." + title)
	if fn != nil {
		r = r.OnTap(fn)
	}
	return r
}

func byteSize(n uint64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func uptime(d time.Duration) string {
	days := int(d / (24 * time.Hour))
	h := int(d/time.Hour) % 24
	m := int(d/time.Minute) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d d %d h", days, h)
	case h > 0:
		return fmt.Sprintf("%d h %d min", h, m)
	}
	return fmt.Sprintf("%d min", m)
}

func (k *Kiosk) aboutPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	info := use(ctx, k, "info", k.sys.Info)
	st := use(ctx, k, "storage", k.sys.Storage)
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	in := info.val
	name := ui.Row("Name").Value(in.Hostname).Key("kiosk.row.name")
	if k.sys.Can(system.CanHostname) {
		name = name.Chevron(outline.AngleRight).OnTap(func() { n.push("name") })
	}
	rows := []gift.View{
		value("Operating System", in.OS),
		value("Kernel", in.Kernel),
		value("Model", in.Model),
	}
	if in.Serial != "" {
		rows = append(rows, value("Serial Number", in.Serial))
	}
	cpu := in.CPU
	if in.Cores > 0 {
		cpu = strings.TrimSpace(fmt.Sprintf("%s, %d cores", cpu, in.Cores))
	}
	rows = append(rows, value("Processor", cpu), value("Memory", byteSize(in.Memory)))
	if len(st.val.Volumes) > 0 {
		v := st.val.Volumes[0]
		rows = append(rows, value("Capacity", byteSize(v.Total)), value("Available", byteSize(v.Free)))
	}
	rows = append(rows, value("Uptime", uptime(in.Uptime)))
	if !math.IsNaN(in.Temperature) && in.Temperature != 0 {
		rows = append(rows, value("Temperature", fmt.Sprintf("%.0f °C", in.Temperature)))
	}
	if in.Undervoltage {
		rows = append(rows, value("Power Supply", "Undervoltage since start"))
	} else if in.Throttled {
		rows = append(rows, value("Processor Speed", "Throttled since start"))
	}
	var net []gift.View
	for _, i := range ifs.val {
		label := "Ethernet"
		if i.Kind == system.Wifi {
			label = "Wi-Fi"
		}
		if i.MAC != "" {
			net = append(net, value(label+" Address", i.MAC))
		}
		if i.Addr.Address.IsValid() {
			net = append(net, value(label+" IP Address", i.Addr.Address.Addr().String()))
		}
	}
	kids := []gift.View{
		section("", "", name),
		section("", "", rows...),
	}
	if len(net) > 0 {
		kids = append(kids, section("Network", "", net...))
	}
	kids = append(kids, section("Program", "", value(k.cfg.Title, k.cfg.Version), value("Data", k.cfg.DataDir)))
	return scroll(kids...)
}

func (k *Kiosk) namePage(ctx *gift.Context, n *navigator, _ string) gift.View {
	info := use(ctx, k, "info", k.sys.Info)
	ed := ui.Editor(ctx, "name", info.val.Hostname)
	msg := ctx.State("msg", "")
	ctx.Read(msg)
	if ed.Text() == "" && info.val.Hostname != "" {
		ed.SetText(info.val.Hostname)
	}
	save := func() {
		name := strings.TrimSpace(ed.Text())
		k.do(background(func(ctx context.Context) error { return k.sys.SetHostname(ctx, name) }), func(err error) {
			if err != nil {
				msg.Set(errText(err))
				return
			}
			n.pop()
		})
	}
	return scroll(
		section("", "The name of the device on the network: letters, digits and hyphens.",
			ui.TextField(ed).Placeholder("Name").OnSubmit(func(string) { save() }).
				Border(ui.Border{Width: 1, Color: ui.ColorClear}).Background(ui.ColorClear).Key("kiosk.name.field"),
		),
		status(msg.Get(), true),
		section("", "", action("Save", false, false, save)),
	)
}

func (k *Kiosk) generalPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	msg := ctx.State("msg", "")
	confirm := ctx.State("confirm", "")
	ctx.Read(msg)
	ctx.Read(confirm)
	nav := func(key, value string) gift.View {
		p, _ := k.page(key)
		return ui.Row(p.title).Value(value).Chevron(outline.AngleRight).OnTap(func() { n.push(key) }).Key("kiosk.row." + key)
	}
	rows := []gift.View{nav("about", "")}
	if k.cfg.Updates.App && k.cfg.AppUpdater != nil || k.cfg.Updates.System || k.cfg.Updates.Release {
		rows = append(rows, nav("updates", ""))
	}
	rows2 := []gift.View{nav("storage", ""), nav("datetime", ""), nav("startup", "")}
	power := k.sys.Can(system.CanPower)
	run := func(what string, fn func(context.Context) error) func() {
		return func() {
			confirm.Set("")
			k.do(background(fn), func(err error) { msg.Set(errText(err)) })
			msg.Set(what)
		}
	}
	page := scroll(
		section("", "", rows...),
		section("", "", rows2...),
		section("", "",
			action("Restart", false, !power, func() { confirm.Set("restart") }),
			action("Shut Down", true, !power, func() { confirm.Set("shutdown") }),
		),
		status(msg.Get(), false),
	)
	var alert gift.View = ui.Box()
	switch confirm.Get() {
	case "restart":
		alert = ui.Alert("Restart the device?", "The program is closed and starts again with the device.",
			ui.AlertCancel("Cancel", func() { confirm.Set("") }),
			ui.AlertAction("Restart", run("Restarting…", k.sys.Reboot)))
	case "shutdown":
		alert = ui.Alert("Shut down the device?", "It stays off until its power is switched off and on again.",
			ui.AlertCancel("Cancel", func() { confirm.Set("") }),
			ui.AlertDestructive("Shut Down", run("Shutting down…", k.sys.PowerOff)))
	}
	return ui.Modal(page, alert).Presented(confirm.Get() != "").OnDismiss(func() { confirm.Set("") })
}

func (k *Kiosk) storagePage(ctx *gift.Context, n *navigator, _ string) gift.View {
	st := use(ctx, k, "storage", k.sys.Storage)
	data := use(ctx, k, "data", func(context.Context) (int64, error) { return dirSize(k.cfg.DataDir), nil })
	msg := ctx.State("msg", "")
	busy := ctx.State("busy", false)
	ctx.Read(msg)
	ctx.Read(busy)
	if !st.done {
		return scroll(ui.ProgressBar(0).Indeterminate().Key("kiosk.loading"))
	}
	if st.err != nil {
		return scroll(status(errText(st.err), true))
	}
	s := st.val
	var kids []gift.View
	if len(s.Volumes) > 0 {
		v := s.Volumes[0]
		usage := s.Usage
		// The data of the program is part of what the system reports as its
		// own; it is shown on its own.
		if d := data.val; d > 0 && len(usage) > 0 && usage[0].Name == "System" {
			usage = append([]system.Usage{{Name: "System", Bytes: max(usage[0].Bytes-d, 0)}, {Name: k.cfg.Title, Bytes: d}}, usage[1:]...)
		}
		colors := []ui.Color{tintGray, tintBlue, tintOrange, tintGreen, tintIndigo}
		var bar, legend []gift.View
		used := int64(0)
		for i, u := range usage {
			used += u.Bytes
			w := float32(float64(u.Bytes) / float64(max(v.Total, 1)))
			bar = append(bar, ui.Box().Frame(geom.Unbounded(), 14).Background(colors[i%len(colors)]).Flex(max(w, 0.002)))
			legend = append(legend, ui.HStack(ui.Box().Frame(10, 10).CornerRadius(5).Background(colors[i%len(colors)]),
				ui.Text(u.Name).FontSize(13).Foreground(ui.ColorSecondaryLabel)).Gap(6).Align(geom.Center))
		}
		if free := float32(float64(v.Free) / float64(max(v.Total, 1))); free > 0 {
			bar = append(bar, ui.Box().Frame(geom.Unbounded(), 14).Background(ui.ColorControl).Flex(free))
		}
		head := ui.VStack(
			ui.HStack(ui.Text(v.Path).FontSize(17), ui.Spacer(),
				ui.Text(fmt.Sprintf("%s of %s used", byteSize(v.Total-v.Free), byteSize(v.Total))).FontSize(15).Foreground(ui.ColorSecondaryLabel)),
			ui.HStack(bar...).Gap(0).CornerRadius(4).Clip(true),
			ui.HStack(legend...).Gap(14),
		).Gap(10).Padding(16).Background(ui.ColorSurface).CornerRadius(10)
		kids = append(kids, head)
		var rows []gift.View
		for _, u := range usage {
			rows = append(rows, value(u.Name, byteSize(uint64(u.Bytes))))
		}
		rows = append(rows, value("Free", byteSize(v.Free)))
		kids = append(kids, section("", "", rows...))
	}
	clean := func() {
		busy.Set(true)
		k.do(background(func(ctx context.Context) error {
			freed, err := k.sys.CleanTemporary(ctx)
			if err == nil {
				k.post(func() { msg.Set("Freed " + byteSize(uint64(freed)) + ".") })
			}
			return err
		}), func(err error) {
			busy.Set(false)
			if err != nil {
				msg.Set(errText(err))
			}
			st.reload()
		})
	}
	label := "Clean Up Temporary Files"
	if busy.Get() {
		label = "Cleaning Up…"
	}
	kids = append(kids, section("", fmt.Sprintf("Downloaded packages, old logs and expired temporary files: about %s.", byteSize(uint64(s.Cleanable))),
		action(label, false, busy.Get() || !k.sys.Can(system.CanClean), clean)))
	kids = append(kids, status(msg.Get(), false))
	if len(s.Volumes) > 1 {
		var rows []gift.View
		for _, v := range s.Volumes[1:] {
			rows = append(rows, value(v.Path, fmt.Sprintf("%s free of %s", byteSize(v.Free), byteSize(v.Total))))
		}
		kids = append(kids, section("Other Volumes", "", rows...))
	}
	return scroll(kids...)
}

// dirSize is the size of the files under dir.
func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func (k *Kiosk) dateTimePage(ctx *gift.Context, n *navigator, _ string) gift.View {
	clock := use(ctx, k, "clock", k.sys.Clock)
	msg := ctx.State("msg", "")
	ctx.Read(msg)
	c := clock.val
	can := k.sys.Can(system.CanClock)
	ntp := ui.Row("Set Automatically").Accessory(ui.Toggle(c.NTP, func(on bool) {
		k.do(background(func(ctx context.Context) error { return k.sys.SetNTP(ctx, on) }), func(err error) {
			msg.Set(errText(err))
			clock.reload()
		})
	}).Disabled(!can).Key("kiosk.ntp"))
	sync := "No"
	if c.Synchronized {
		sync = "Yes"
	}
	zone := ui.Row("Time Zone").Value(c.Zone).Key("kiosk.row.zone")
	if can {
		zone = zone.Chevron(outline.AngleRight).OnTap(func() { n.push("zone") })
	}
	now := time.Now()
	if loc, err := time.LoadLocation(c.Zone); err == nil {
		now = now.In(loc)
	}
	return scroll(
		section("", "The clock is set from time servers on the network. A device without a clock of its own starts with the last time it saved until it reaches one.",
			ntp, value("Synchronized", sync)),
		section("", "", zone, value("Time", now.Format("2 Jan 2006, 15:04"))),
		status(msg.Get(), true),
	)
}

func (k *Kiosk) zonePage(ctx *gift.Context, n *navigator, _ string) gift.View {
	zones := use(ctx, k, "zones", k.sys.TimeZones)
	clock := use(ctx, k, "clock", k.sys.Clock)
	q := ui.Editor(ctx, "q", "")
	query := ctx.State("query", "")
	ctx.Read(query)
	var rows []gift.View
	needle := strings.ToLower(strings.TrimSpace(query.Get()))
	for _, z := range zones.val {
		if needle != "" && !strings.Contains(strings.ToLower(z), needle) {
			continue
		}
		zone := z
		r := ui.Row(strings.ReplaceAll(z, "_", " ")).Key("kiosk.zone." + z).OnTap(func() {
			k.do(background(func(ctx context.Context) error { return k.sys.SetTimeZone(ctx, zone) }), func(err error) {
				if err == nil {
					n.pop()
				}
			})
		})
		if z == clock.val.Zone {
			r = r.Accessory(ui.Icon(outline.Check).Size(18).Foreground(ui.ColorAccent))
		}
		rows = append(rows, r)
		if len(rows) >= 200 {
			break
		}
	}
	return scroll(
		ui.SearchField(q).Placeholder("Search").Bind(query.Binding()).Key("kiosk.zone.search"),
		section("", "", rows...),
	)
}

func (k *Kiosk) displayPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	disp := use(ctx, k, "display", k.sys.Display)
	msg := ctx.State("msg", "")
	appearance := ctx.State("appearance", k.store.appearance())
	ctx.Read(msg)
	ctx.Read(appearance)
	d := disp.val
	var kids []gift.View
	modes := []string{"Light", "Dark"}
	sel := 0
	if appearance.Get() == "dark" {
		sel = 1
	}
	kids = append(kids, section("Appearance", "", ui.Row("").Accessory(
		ui.SegmentedControl(sel, modes, func(i int) {
			a := strings.ToLower(modes[i])
			appearance.Set(a)
			k.store.setAppearance(a)
			k.applyAppearance()
		}).Flex(1).Key("kiosk.appearance"))))
	if d.HasBrightness {
		kids = append(kids, section("Brightness", "", ui.Row("").Accessory(
			ui.HStack(ui.Icon(outline.Sun).Size(16).Foreground(ui.ColorSecondaryLabel),
				ui.Slider(d.Brightness, func(v float64) {
					d.Brightness = v
					disp.set(d)
					go k.sys.SetBrightness(context.Background(), v)
				}).Range(0.05, 1).Disabled(!k.sys.Can(system.CanBrightness)).Flex(1).Key("kiosk.brightness"),
				ui.Icon(outline.Sun).Size(22).Foreground(ui.ColorSecondaryLabel),
			).Gap(12).Align(geom.Center).Flex(1))))
	}
	if d.Output != "" {
		labels := []string{"0°", "90°", "180°", "270°"}
		cur := int(d.Rotation / 90)
		kids = append(kids, section("Rotation", fmt.Sprintf("%s, %d × %d pixels", d.Output, d.Width, d.Height)+diagonal(d),
			ui.Row("").Accessory(ui.SegmentedControl(cur, labels, func(i int) {
				r := system.Rotation(i * 90)
				k.do(background(func(ctx context.Context) error { return k.sys.SetRotation(ctx, r) }), func(err error) {
					msg.Set(errText(err))
					disp.reload()
				})
			}).Disabled(!k.sys.Can(system.CanRotation)).Flex(1).Key("kiosk.rotation"))))
	}
	if !disp.done {
		kids = append(kids, ui.ProgressBar(0).Indeterminate())
	}
	kids = append(kids, status(msg.Get(), true))
	return scroll(kids...)
}

func diagonal(d system.Display) string {
	if in := d.Diagonal(); in > 0 {
		return fmt.Sprintf(", %.1f inches", in)
	}
	return ""
}
