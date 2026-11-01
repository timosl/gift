package kiosk

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// firstOf is the first interface of a kind.
func firstOf(ifs []system.Interface, kind system.InterfaceKind) (system.Interface, bool) {
	for _, i := range ifs {
		if i.Kind == kind {
			return i, true
		}
	}
	return system.Interface{}, false
}

// bars draws the strength of a wireless signal as four bars.
func bars(strength int) gift.View {
	n := 0
	switch {
	case strength >= 75:
		n = 4
	case strength >= 50:
		n = 3
	case strength >= 25:
		n = 2
	case strength > 0:
		n = 1
	}
	var kids []gift.View
	for i := range 4 {
		c := ui.Color(ui.Fade(ui.ColorLabel, 0.18))
		if i < n {
			c = ui.ColorLabel
		}
		kids = append(kids, ui.Box().Frame(3, float32(4+3*i)).CornerRadius(1).Background(c))
	}
	return ui.HStack(kids...).Gap(2).Align(geom.Alignment{X: 0, Y: 1})
}

func (k *Kiosk) wifiPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	if !ifs.done {
		return scroll(ui.ProgressBar(0).Indeterminate())
	}
	w, ok := firstOf(ifs.val, system.Wifi)
	if !ok {
		return scroll(status("This device has no wireless interface.", false))
	}
	return gift.Component("wifi:"+w.Device, func(ctx *gift.Context) gift.View { return k.wifiDevice(ctx, n, w.Device, ifs) })
}

// wifiDevice is the Wi-Fi page of one wireless interface, once it is known.
func (k *Kiosk) wifiDevice(ctx *gift.Context, n *navigator, device string, ifs *loader[[]system.Interface]) gift.View {
	w, _ := firstOf(ifs.val, system.Wifi)
	scan := use(ctx, k, "scan", func(ctx context.Context) ([]system.AccessPoint, error) {
		return k.sys.Scan(ctx, device)
	})
	msg := ctx.State("msg", "")
	joining := ctx.State("joining", "")
	asking := ctx.State("asking", "")
	ctx.Read(msg)
	ctx.Read(joining)
	ctx.Read(asking)
	pw := ui.Editor(ctx, "password", "")
	can := k.sys.Can(system.CanWifi)
	join := func(ap system.AccessPoint, password string) {
		joining.Set(ap.SSID)
		msg.Set("")
		k.do(background(func(ctx context.Context) error {
			return k.sys.Join(ctx, w.Device, system.Join{SSID: ap.SSID, Password: password})
		}), func(err error) {
			joining.Set("")
			pw.SetText("")
			if errors.Is(err, system.ErrWrongPassword) {
				asking.Set(ap.SSID)
			}
			msg.Set(errText(err))
			ifs.reload()
			scan.reload()
		})
	}
	toggle := ui.Row("Wi-Fi").Leading(tile(wifiSymbol, tintBlue)).Accessory(
		ui.Toggle(w.Enabled, func(on bool) {
			k.do(background(func(ctx context.Context) error { return k.sys.SetWifi(ctx, on) }), func(err error) {
				msg.Set(errText(err))
				ifs.reload()
				scan.reload()
			})
		}).Disabled(!can).Key("kiosk.wifi.toggle"))
	top := []gift.View{toggle}
	if w.Enabled && w.SSID != "" {
		top = append(top, ui.Row(w.SSID).Leading(ui.Icon(outline.Check).Size(18).Foreground(ui.ColorAccent)).
			Accessory(bars(w.Signal)).Chevron(outline.AngleRight).OnTap(func() { n.push("network:" + w.SSID) }).
			Key("kiosk.wifi.current"))
	}
	kids := []gift.View{section("", "", top...)}
	if w.Enabled {
		var rows []gift.View
		for _, ap := range scan.val {
			if ap.Active {
				continue
			}
			ap := ap
			acc := []gift.View{}
			if joining.Get() == ap.SSID {
				acc = append(acc, ui.Text("Joining…").FontSize(15).Foreground(ui.ColorSecondaryLabel))
			}
			if ap.Secured {
				acc = append(acc, ui.Icon(outline.Lock).Size(14).Foreground(ui.ColorLabel))
			}
			acc = append(acc, bars(ap.Strength))
			r := ui.Row(ap.SSID).Accessory(ui.HStack(acc...).Gap(8).Align(geom.Center)).Key("kiosk.wifi." + ap.SSID).
				Disabled(!can).OnTap(func() {
				if ap.Secured && !ap.Known {
					asking.Set(ap.SSID)
					return
				}
				join(ap, "")
			})
			if ap.Known {
				r = r.Subtitle("Known network")
			}
			rows = append(rows, r)
		}
		rows = append(rows, ui.Row("Other…").Disabled(!can).OnTap(func() { n.push("join") }).Key("kiosk.wifi.other"))
		header := "Networks"
		if scan.busy {
			header = "Networks — searching…"
		}
		kids = append(kids, section(header, "", rows...),
			section("", "", action("Search Again", false, scan.busy, scan.reload)))
	}
	kids = append(kids, status(msg.Get(), true))
	page := scroll(kids...)
	ssid := asking.Get()
	sheet := ui.VStack(
		ui.Text(fmt.Sprintf("Enter the password for “%s”", ssid)).FontSize(17),
		ui.SecureField(pw).Placeholder("Password").OnSubmit(func(s string) {
			asking.Set("")
			join(system.AccessPoint{SSID: ssid}, s)
		}).Key("kiosk.wifi.password"),
		ui.HStack(
			ui.Button(ui.Text("Cancel"), func() { asking.Set(""); pw.SetText("") }).Key("kiosk.wifi.cancel"),
			ui.Spacer(),
			ui.Button(ui.Text("Join"), func() {
				asking.Set("")
				join(system.AccessPoint{SSID: ssid}, pw.Text())
			}).Key("kiosk.wifi.join"),
		),
	).Gap(16).Padding(24).Frame(420, geom.Unbounded()).Background(ui.ColorSurface).CornerRadius(14)
	return ui.Modal(page, sheet).Presented(ssid != "").OnDismiss(func() { asking.Set("") })
}

// networkPage is a wireless network the device is joined to or knows: its
// address, and forgetting it.
func (k *Kiosk) networkPage(ctx *gift.Context, n *navigator, ssid string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	msg := ctx.State("msg", "")
	ctx.Read(msg)
	if !ifs.done {
		return scroll(ui.ProgressBar(0).Indeterminate())
	}
	w, _ := firstOf(ifs.val, system.Wifi)
	kids := []gift.View{}
	if w.SSID == ssid {
		kids = append(kids, gift.Component("ip", func(ctx *gift.Context) gift.View { return k.ipSection(ctx, w, ifs) }))
	}
	kids = append(kids, section("", "", action("Forget This Network", true, !k.sys.Can(system.CanWifi), func() {
		k.do(background(func(ctx context.Context) error { return k.sys.Forget(ctx, ssid) }), func(err error) {
			if err != nil {
				msg.Set(errText(err))
				return
			}
			n.pop()
		})
	})), status(msg.Get(), true))
	return scroll(kids...)
}

// joinPage joins a network that does not announce itself.
func (k *Kiosk) joinPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	name := ui.Editor(ctx, "ssid", "")
	pw := ui.Editor(ctx, "password", "")
	msg := ctx.State("msg", "")
	busy := ctx.State("busy", false)
	ctx.Read(msg)
	ctx.Read(busy)
	w, _ := firstOf(ifs.val, system.Wifi)
	join := func() {
		busy.Set(true)
		j := system.Join{SSID: strings.TrimSpace(name.Text()), Password: pw.Text(), Hidden: true}
		k.do(background(func(ctx context.Context) error { return k.sys.Join(ctx, w.Device, j) }), func(err error) {
			busy.Set(false)
			if err != nil {
				msg.Set(errText(err))
				return
			}
			n.pop()
		})
	}
	field := func(v ui.TextFieldView) gift.View {
		return v.Border(ui.Border{Width: 1, Color: ui.ColorClear}).Background(ui.ColorClear)
	}
	label := "Join"
	if busy.Get() {
		label = "Joining…"
	}
	return scroll(
		section("", "For a network that does not show in the list. Leave the password empty for an open one.",
			field(ui.TextField(name).Placeholder("Name").Key("kiosk.join.ssid")),
			field(ui.SecureField(pw).Placeholder("Password").OnSubmit(func(string) { join() }).Key("kiosk.join.password")),
		),
		section("", "", action(label, false, busy.Get() || strings.TrimSpace(name.Text()) == "", join)),
		status(msg.Get(), true),
	)
}

func (k *Kiosk) ethernetPage(ctx *gift.Context, n *navigator, _ string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	var rows []gift.View
	for _, i := range ifs.val {
		if i.Kind != system.Ethernet {
			continue
		}
		dev := i.Device
		v := "Not Connected"
		if i.State == system.Connected {
			v = "Connected"
			if i.Addr.Address.IsValid() {
				v = i.Addr.Address.Addr().String()
			}
		}
		sub := i.Connection
		if i.Addr.Method == system.Manual || i.Config.Method == system.Manual {
			sub = strings.TrimSpace(sub + " · Manual")
		}
		r := ui.Row(dev).Value(v).Chevron(outline.AngleRight).OnTap(func() { n.push("interface:" + dev) }).Key("kiosk.eth." + dev)
		if sub != "" {
			r = r.Subtitle(sub)
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 && ifs.done {
		return scroll(status("This device has no wired interface.", false))
	}
	return scroll(section("Interfaces", "", rows...))
}

func (k *Kiosk) interfacePage(ctx *gift.Context, n *navigator, dev string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	if !ifs.done {
		return scroll(ui.ProgressBar(0).Indeterminate())
	}
	var in system.Interface
	for _, i := range ifs.val {
		if i.Device == dev {
			in = i
		}
	}
	state := map[system.LinkState]string{system.Unavailable: "Cable unplugged", system.Disconnected: "Not connected",
		system.Connecting: "Connecting…", system.Connected: "Connected"}[in.State]
	return scroll(
		section("", "", value("Status", state), value("Connection", in.Connection), value("Hardware Address", in.MAC)),
		gift.Component("ip", func(ctx *gift.Context) gift.View { return k.ipSection(ctx, in, ifs) }),
	)
}

// ipSection configures the IPv4 address of an interface: automatic, or by
// hand with the address, the subnet mask and the router, as the network of
// a building gives them, and the name servers. A change takes effect at
// once and is undone after the timeout of the system unless it is kept.
func (k *Kiosk) ipSection(ctx *gift.Context, in system.Interface, ifs *loader[[]system.Interface]) gift.View {
	for _, i := range ifs.val {
		if i.Device == in.Device {
			in = i
		}
	}
	cfg := in.Config
	manual := ctx.State("manual", cfg.Method == system.Manual)
	msg := ctx.State("msg", "")
	pending := ctx.State("pending", (*pendingChange)(nil))
	ctx.Read(manual)
	ctx.Read(msg)
	ctx.Read(pending)
	src := cfg
	if src.Method != system.Manual {
		src = in.Addr
	}
	addr, mask, router := "", "", ""
	if src.Address.IsValid() {
		addr, mask = src.Address.Addr().String(), system.Mask(src.Address.Bits())
	}
	if src.Router.IsValid() {
		router = src.Router.String()
	}
	var dns []string
	for _, d := range cfg.DNS {
		dns = append(dns, d.String())
	}
	edAddr := ui.Editor(ctx, "addr", addr)
	edMask := ui.Editor(ctx, "mask", mask)
	edRouter := ui.Editor(ctx, "router", router)
	edDNS := ui.Editor(ctx, "dns", strings.Join(dns, ", "))
	can := k.sys.Can(system.CanNetwork)

	field := func(label string, ed *ui.TextEditor, placeholder string) gift.View {
		return ui.Row(label).Accessory(ui.TextField(ed).Placeholder(placeholder).Disabled(!can || !manual.Get() && label != "DNS").
			Border(ui.Border{Width: 1, Color: ui.ColorClear}).Background(ui.ColorClear).Frame(220, geom.Unbounded()).
			Key("kiosk.ip." + strings.ToLower(strings.ReplaceAll(label, " ", ""))))
	}
	apply := func() {
		c, err := parseConfig(manual.Get(), edAddr.Text(), edMask.Text(), edRouter.Text(), edDNS.Text())
		if err != nil {
			msg.Set(err.Error())
			return
		}
		msg.Set("Applying…")
		dev := in.Device
		go func() {
			ch, err := k.sys.Configure(context.Background(), dev, c)
			k.post(func() {
				if err != nil {
					msg.Set(errText(err))
					return
				}
				msg.Set("")
				pending.Set(&pendingChange{ch: ch, until: time.Now().Add(ch.Timeout())})
				ifs.reload()
			})
		}()
	}
	rows := []gift.View{
		ui.Row("Configure IP").Accessory(ui.SegmentedControl(boolIndex(manual.Get()), []string{"Automatic", "Manual"},
			func(i int) { manual.Set(i == 1) }).Disabled(!can).Key("kiosk.ip.method")),
		field("IP Address", edAddr, "192.168.1.10"),
		field("Subnet Mask", edMask, "255.255.255.0"),
		field("Router", edRouter, "192.168.1.1"),
	}
	page := ui.VStack(
		section("IPv4 Address", "With Automatic the address comes from the router of the network (DHCP).", rows...),
		section("DNS", "Name servers separated by commas; empty takes those of the network.", field("DNS", edDNS, "Automatic")),
		section("", "", action("Apply", false, !can, apply)),
		status(msg.Get(), strings.HasPrefix(msg.Get(), "system:") || !strings.HasSuffix(msg.Get(), "…")),
	).Gap(28)
	p := pending.Get()
	if p == nil {
		return page
	}
	keep := func(revert bool) {
		pending.Set(nil)
		k.do(background(func(ctx context.Context) error {
			if revert {
				return p.ch.Revert(ctx)
			}
			return p.ch.Keep(ctx)
		}), func(err error) {
			msg.Set(errText(err))
			ifs.reload()
		})
	}
	secs := int(time.Until(p.until).Seconds())
	question := section("Keep these network settings?",
		fmt.Sprintf("If the device cannot be reached any more, the old settings come back by themselves after %d seconds.", max(secs, 0)),
		action("Keep", false, false, func() { keep(false) }),
		action("Revert", true, false, func() { keep(true) }),
	)
	return ui.VStack(question, page).Gap(28)
}

type pendingChange struct {
	ch    system.Change
	until time.Time
}

func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}

// parseConfig reads the fields of an IPv4 configuration, the subnet mask
// in either form: 255.255.255.0 or 24.
func parseConfig(manual bool, addr, mask, router, dns string) (system.IPConfig, error) {
	var c system.IPConfig
	for _, s := range strings.FieldsFunc(dns, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		a, err := netip.ParseAddr(s)
		if err != nil || !a.Is4() {
			return c, fmt.Errorf("%q is not an IPv4 address.", s)
		}
		c.DNS = append(c.DNS, a)
	}
	if !manual {
		return c, nil
	}
	c.Method = system.Manual
	a, err := netip.ParseAddr(strings.TrimSpace(addr))
	if err != nil || !a.Is4() {
		return c, fmt.Errorf("%q is not an IPv4 address.", addr)
	}
	mask = strings.TrimSpace(strings.TrimPrefix(mask, "/"))
	bits, ok := system.MaskBits(mask)
	if !ok {
		var n int
		if _, err := fmt.Sscan(mask, &n); err == nil && n >= 1 && n <= 32 && fmt.Sprint(n) == mask {
			bits, ok = n, true
		}
	}
	if !ok {
		return c, fmt.Errorf("%q is not a subnet mask.", mask)
	}
	c.Address = netip.PrefixFrom(a, bits)
	if r := strings.TrimSpace(router); r != "" {
		ra, err := netip.ParseAddr(r)
		if err != nil || !ra.Is4() {
			return c, fmt.Errorf("%q is not an IPv4 address.", router)
		}
		c.Router = ra
	}
	if err := c.Check(); err != nil {
		return c, errors.New(strings.TrimPrefix(err.Error(), "system: "))
	}
	return c, nil
}
