package kiosk

import (
	"context"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/kiosk/system"
	"github.com/worldiety/gift/ui"
)

// The tints of the tiles, after the colours of the settings of iOS.
var (
	tintBlue   = ui.RGB(0x0A, 0x84, 0xFF)
	tintGray   = ui.RGB(0x8E, 0x8E, 0x93)
	tintGreen  = ui.RGB(0x30, 0xD1, 0x58)
	tintRed    = ui.RGB(0xFF, 0x45, 0x3A)
	tintOrange = ui.RGB(0xFF, 0x9F, 0x0A)
	tintIndigo = ui.RGB(0x5E, 0x5C, 0xE6)
	tintTeal   = ui.RGB(0x30, 0xB0, 0xC7)
)

// View is the kiosk: the program, and over it the settings while they are
// open. It is the root view of the window, with the on-screen keyboard.
func (k *Kiosk) View(ctx *gift.Context) gift.View {
	open := ctx.State("kiosk.open", false)
	path := ctx.State("kiosk.path", "")
	unlocked := ctx.State("kiosk.unlocked", false)
	asking := ctx.State("kiosk.asking", "")
	class := ctx.State("kiosk.class", Regular)
	ctx.Read(open)
	ctx.Read(path)
	ctx.Read(unlocked)
	ctx.Read(asking)
	ctx.Read(class)
	k.open, k.path = open, path

	nav := &navigator{k: k, path: path, unlocked: unlocked, asking: asking}
	k.toggle = func(on bool) {
		if !on {
			open.Set(false)
			path.Set("")
			unlocked.Set(false)
			asking.Set("")
			return
		}
		if k.guarded(AreaSettings) && !unlocked.Get() {
			asking.Set(openSettings)
			return
		}
		open.Set(true)
	}
	nav.opened = func() { open.Set(true) }

	kids := []gift.View{fill{gift.Component("kiosk.app", func(ctx *gift.Context) gift.View { return k.cfg.App(ctx, k) })}}
	switch k.cfg.Entry {
	case EntryButton:
		kids = append(kids, fill{ui.ZStack(
			ui.Button(ui.Icon(outline.Cog).Size(20).Foreground(ui.ColorSecondaryLabel), k.OpenSettings).
				Background(ui.ColorClear).Border(ui.Border{}).Padding(10).Label("Settings").Key("kiosk.entry"),
		).Align(geom.TopTrailing)})
	case EntryCorner:
		kids = append(kids, fill{ui.ZStack(tapGate{onOpen: k.OpenSettings}).Align(geom.TopLeading)})
	}
	if open.Get() {
		sc := k.cfg.SizeClass
		if sc == Automatic {
			sc = class.Get()
		}
		kids = append(kids, fill{gift.Component("kiosk.settings", func(ctx *gift.Context) gift.View {
			return k.settings(ctx, nav, sc)
		})})
	}
	if a := asking.Get(); a != "" {
		kids = append(kids, fill{gift.Component("kiosk.pin", func(ctx *gift.Context) gift.View {
			return k.pinPrompt(ctx, func(ok bool) {
				asking.Set("")
				if !ok {
					return
				}
				unlocked.Set(true)
				if a == openSettings {
					open.Set(true)
				} else {
					path.Set(a)
				}
			})
		})})
	}
	body := sizer{class: class.Get(), set: class.Set, child: ui.ZStack(kids...)}
	return ui.Window(body, ui.OnScreenKeyboard()).Align(geom.Bottom)
}

// openSettings is what the PIN prompt asks for when it guards the settings
// as a whole rather than a page.
const openSettings = "\x00open"

// guarded reports whether the PIN protects area.
func (k *Kiosk) guarded(area Area) bool {
	if k.store.digits(k.cfg.PIN) == 0 {
		return false
	}
	prot := k.cfg.Protect
	if k.cfg.PIN == "" {
		prot = k.store.protects()
	}
	return prot&area != 0
}

// navigator moves through the pages of the settings. A path is the keys of
// the pages from the list down, joined by slashes; a key may carry a
// parameter after a colon, the SSID of a network say.
type navigator struct {
	k        *Kiosk
	path     *gift.State[string]
	unlocked *gift.State[bool]
	asking   *gift.State[string]
	opened   func()
}

func (n *navigator) segments() []string {
	if n.path.Get() == "" {
		return nil
	}
	return strings.Split(n.path.Get(), "/")
}

// push opens the page key below the current one, after the PIN if the page
// needs it.
func (n *navigator) push(key string) {
	n.go_(strings.Trim(n.path.Get()+"/"+key, "/"))
}

// show opens the page key at the top, below the list.
func (n *navigator) show(key string) { n.go_(key) }

func (n *navigator) go_(path string) {
	segs := strings.Split(path, "/")
	p, ok := n.k.page(segs[len(segs)-1])
	if ok && p.area != 0 && n.k.guarded(p.area) && !n.unlocked.Get() {
		n.asking.Set(path)
		return
	}
	n.path.Set(path)
}

func (n *navigator) pop() {
	segs := n.segments()
	if len(segs) == 0 {
		return
	}
	n.path.Set(strings.Join(segs[:len(segs)-1], "/"))
}

func (n *navigator) close() { n.k.CloseSettings() }

// page is one page of the settings.
type page struct {
	title string
	icon  ui.Symbol
	tint  ui.Color
	area  Area
	view  func(ctx *gift.Context, n *navigator, param string) gift.View
}

// settings is the settings app in the layout of the size class.
func (k *Kiosk) settings(ctx *gift.Context, n *navigator, sc SizeClass) gift.View {
	segs := n.segments()
	screen := func(seg string) ui.ScreenSpec {
		p, _ := k.page(seg)
		key, param, _ := strings.Cut(seg, ":")
		title := p.title
		if param != "" {
			title = param
		}
		return ui.Screen(title, gift.Component("page:"+seg, func(ctx *gift.Context) gift.View {
			return p.view(ctx, n, param)
		})).Key(key + ":" + param)
	}
	done := ui.Button(ui.Text("Done").FontSize(17).Foreground(ui.ColorAccent), n.close).
		Background(ui.ColorClear).Border(ui.Border{}).PaddingInsets(geom.Insets{Left: 12, Right: 16, Top: 12, Bottom: 12}).
		Key("kiosk.done")
	if sc == Compact {
		screens := []ui.ScreenSpec{ui.Screen("Settings", gift.Component("kiosk.list", func(ctx *gift.Context) gift.View {
			return k.list(ctx, n, "")
		})).Key("list")}
		for _, s := range segs {
			screens = append(screens, screen(s))
		}
		// Done leaves the settings from the list; a page below it has the
		// back button there, and the bar has no room for both.
		kids := []gift.View{fill{ui.NavigationStack(n.pop, screens...).BackIcon(outline.AngleLeft)}}
		if len(segs) == 0 {
			kids = append(kids, fill{ui.ZStack(done).Align(geom.TopTrailing)})
		}
		return ui.ZStack(kids...).Background(ui.ColorBackground)
	}
	// Regular: the list beside the page, as on an iPad. The list selects the
	// first segment; the page stacks what was opened below it.
	if len(segs) == 0 {
		first := "general"
		if k.sys.Can(system.CanNetwork) {
			first = "wifi"
		}
		segs = []string{first}
	}
	var screens []ui.ScreenSpec
	for _, s := range segs {
		screens = append(screens, screen(s))
	}
	sidebar := ui.VStack(
		ui.HStack(ui.Text("Settings").FontSize(28).Font(bold()), ui.Spacer(), done).
			Align(geom.Center).PaddingInsets(geom.Insets{Left: 20, Top: 8}),
		gift.Component("kiosk.list", func(ctx *gift.Context) gift.View { return k.list(ctx, n, segs[0]) }),
	).Gap(0).Frame(320, geom.Unbounded()).Background(ui.ColorBackground)
	return ui.HStack(
		sidebar,
		ui.Box().Frame(1, geom.Unbounded()).Background(ui.ColorSeparator),
		// Beside the list a page replaces the one before without moving, as
		// in the settings of macOS; sliding would look like the list moves.
		ui.NavigationStack(n.pop, screens...).BackIcon(outline.AngleLeft).Animated(false).Flex(1),
	).Gap(0).Background(ui.ColorBackground)
}

// list is the list of pages: the device, the network, the system and the
// pages of the program. selected marks the page shown beside it.
func (k *Kiosk) list(ctx *gift.Context, n *navigator, selected string) gift.View {
	ifs := use(ctx, k, "ifaces", k.sys.Interfaces)
	info := use(ctx, k, "info", k.sys.Info)
	sel, _, _ := strings.Cut(selected, ":")
	row := func(key, value string) gift.View {
		p, _ := k.page(key)
		r := ui.Row(p.title).Leading(tile(p.icon, p.tint)).Value(value).Key("kiosk.row." + key)
		if selected != "" {
			r = r.Selected(sel == key).OnTap(func() { n.show(key) })
		} else {
			r = r.Chevron(outline.AngleRight).OnTap(func() { n.push(key) })
		}
		return r
	}
	var wifiValue, ethValue string
	var hasWifi, hasEth bool
	for _, i := range ifs.val {
		switch i.Kind {
		case system.Wifi:
			hasWifi = true
			switch {
			case !i.Enabled:
				wifiValue = "Off"
			case i.SSID != "":
				wifiValue = i.SSID
			default:
				wifiValue = "Not Connected"
			}
		case system.Ethernet:
			hasEth = true
			if i.State == system.Connected {
				ethValue = "Connected"
			} else if ethValue == "" {
				ethValue = "Not Connected"
			}
		}
	}
	model := info.val.Model
	if model == "" {
		model = k.sys.Platform().Name
	}
	device := ui.Row(k.cfg.Title).Subtitle(model).Leading(tile(outline.DesktopPc, tintGray)).
		Key("kiosk.device")
	if selected != "" {
		device = device.Selected(sel == "about").OnTap(func() { n.show("about") })
	} else {
		device = device.Chevron(outline.AngleRight).OnTap(func() { n.push("about") })
	}
	var groups []gift.View
	groups = append(groups, card(device))
	var netRows []gift.View
	if hasWifi {
		netRows = append(netRows, row("wifi", wifiValue))
	}
	if hasEth {
		netRows = append(netRows, row("ethernet", ethValue))
	}
	if len(netRows) > 0 {
		groups = append(groups, card(netRows...))
	}
	sys := []gift.View{row("general", "")}
	if k.sys.Can(system.CanBrightness) || k.sys.Can(system.CanRotation) || k.sys.Platform().Kind == system.SimulatedKind {
		sys = append(sys, row("display", ""))
	}
	sys = append(sys, row("passcode", ""))
	groups = append(groups, card(sys...))
	if len(k.cfg.Pages) > 0 {
		var app []gift.View
		for i := range k.cfg.Pages {
			app = append(app, row(appKey(i), ""))
		}
		groups = append(groups, card(app...))
	}
	return ui.VScroll(groups...).Gap(20).PaddingInsets(geom.Insets{Left: 16, Right: 16, Top: 12, Bottom: 24}).
		Key("kiosk.list").Flex(1)
}

func appKey(i int) string { return "app" + string(rune('a'+i)) }

// page returns the page of a key, with its parameter after a colon.
func (k *Kiosk) page(seg string) (page, bool) {
	key, _, _ := strings.Cut(seg, ":")
	switch key {
	case "about":
		return page{title: "About", icon: outline.InfoCircle, tint: tintGray, view: k.aboutPage}, true
	case "name":
		return page{title: "Name", area: AreaSystem, view: k.namePage}, true
	case "wifi":
		return page{title: "Wi-Fi", icon: wifiSymbol, tint: tintBlue, area: AreaNetwork, view: k.wifiPage}, true
	case "network":
		return page{title: "Network", area: AreaNetwork, view: k.networkPage}, true
	case "join":
		return page{title: "Other Network", area: AreaNetwork, view: k.joinPage}, true
	case "ethernet":
		return page{title: "Ethernet", icon: networkSymbol, tint: tintBlue, area: AreaNetwork, view: k.ethernetPage}, true
	case "interface":
		return page{title: "Interface", area: AreaNetwork, view: k.interfacePage}, true
	case "general":
		return page{title: "General", icon: outline.Cog, tint: tintGray, view: k.generalPage}, true
	case "updates":
		return page{title: "Software Update", icon: outline.Download, tint: tintGray, area: AreaUpdates, view: k.updatesPage}, true
	case "storage":
		return page{title: "Storage", icon: outline.Database, tint: tintGray, area: AreaSystem, view: k.storagePage}, true
	case "datetime":
		return page{title: "Date & Time", icon: outline.Clock, tint: tintBlue, area: AreaSystem, view: k.dateTimePage}, true
	case "zone":
		return page{title: "Time Zone", area: AreaSystem, view: k.zonePage}, true
	case "startup":
		return page{title: "Startup", icon: outline.Rocket, tint: tintIndigo, area: AreaSystem, view: k.startupPage}, true
	case "display":
		return page{title: "Display", icon: outline.Sun, tint: tintBlue, area: AreaSystem, view: k.displayPage}, true
	case "passcode":
		return page{title: "Passcode", icon: outline.Lock, tint: tintRed, area: AreaSettings | AreaSystem, view: k.passcodePage}, true
	}
	if strings.HasPrefix(key, "app") && len(key) == 4 {
		i := int(key[3] - 'a')
		if i >= 0 && i < len(k.cfg.Pages) {
			p := k.cfg.Pages[i]
			area := Area(0)
			if p.Protected {
				area = AreaApp
			}
			tint := p.Tint
			if tint == (ui.Color{}) {
				tint = tintTeal
			}
			return page{title: p.Title, icon: p.Icon, tint: tint, area: area,
				view: func(ctx *gift.Context, _ *navigator, _ string) gift.View {
					return scroll(p.View(ctx, k))
				}}, true
		}
	}
	return page{title: key, view: func(*gift.Context, *navigator, string) gift.View { return ui.Text("") }}, false
}

// tile is the white symbol on a coloured, rounded square that leads a row
// of the settings of iOS.
func tile(sym ui.Symbol, tint ui.Color) gift.View {
	return ui.ZStack(ui.Icon(sym).Size(17).Foreground(ui.RGB(255, 255, 255))).
		Align(geom.Center).Frame(29, 29).Background(tint).CornerRadius(7)
}

// card is a group of rows on a rounded surface, a section of a grouped list.
func card(rows ...gift.View) gift.View {
	return ui.List(rows...).Background(ui.ColorSurface).CornerRadius(10).Clip(true).
		SeparatorInsets(ui.RowPadding+29+12, 0)
}

// section is a card with a header above it and a footer below, either
// empty.
func section(header, footer string, rows ...gift.View) gift.View {
	kids := []gift.View{}
	if header != "" {
		kids = append(kids, ui.Text(strings.ToUpper(header)).FontSize(13).Foreground(ui.ColorSecondaryLabel).
			PaddingInsets(geom.Insets{Left: 16}))
	}
	kids = append(kids, ui.List(rows...).Background(ui.ColorSurface).CornerRadius(10).Clip(true))
	if footer != "" {
		kids = append(kids, ui.Text(footer).FontSize(13).Foreground(ui.ColorSecondaryLabel).
			PaddingInsets(geom.Insets{Left: 16, Right: 16}))
	}
	return ui.VStack(kids...).Gap(6).Align(geom.Alignment{X: 0, Y: 0})
}

// scroll is the body of a page: its sections in a column that scrolls, as
// wide as the page, with room for the on-screen keyboard.
func scroll(kids ...gift.View) gift.View {
	kids = append(kids, ui.Box().Frame(1, ui.OnScreenKeyboardHeight()))
	return ui.VScroll(ui.VStack(kids...).Gap(28).Align(geom.Alignment{X: 0, Y: 0})).
		PaddingInsets(geom.Insets{Left: 16, Right: 16, Top: 20}).
		Background(ui.ColorBackground).Flex(1)
}

// bold is the bold face of the family of the default font, or the default
// font where it has none.
func bold() ui.Font {
	def := ui.DefaultFont()
	for _, f := range ui.RegisteredFonts(nil) {
		if f.Font == def {
			if b, ok := ui.ResolveFont(ui.FontQuery{Family: f.Family, Weight: ui.WeightBold}); ok {
				return b
			}
		}
	}
	return def
}

// sizer lays out its child in all the window offers and tells the kiosk the
// size class of that width when it changes, from a timer of its node, where
// state may be written.
type sizer struct {
	class SizeClass
	set   func(SizeClass)
	child gift.View
}

var sizerType = gift.RegisterType("kiosk.sizer")

func (sizer) ViewType() gift.TypeID { return sizerType }

func (s sizer) Build(*gift.BuildContext) gift.Element {
	n := &sizerNode{s: s}
	return gift.Element{Layouter: n, Interactor: n, Children: []gift.View{s.child}, Flex: 1}
}

type sizerNode struct {
	s    sizer
	want SizeClass
}

func (n *sizerNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	size := c.Constrain(c.Max)
	n.want = Compact
	if size.W >= RegularWidth {
		n.want = Regular
	}
	if n.want != n.s.class {
		ctx.After(time.Millisecond)
	}
	ctx.Measure(0, geom.Tight(size))
	ctx.Place(0, geom.Point{})
	return size
}

func (n *sizerNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind == gift.EventTimer && n.want != n.s.class {
		n.s.set(n.want)
		return true
	}
	return false
}

// tapGate is the corner that opens the settings after five taps within two
// seconds.
type tapGate struct{ onOpen func() }

var tapGateType = gift.RegisterType("kiosk.tapGate")

func (tapGate) ViewType() gift.TypeID { return tapGateType }

func (t tapGate) Build(*gift.BuildContext) gift.Element {
	n := &tapGateNode{t: t}
	return gift.Element{Layouter: n, Interactor: n, Key: "kiosk.corner"}
}

type tapGateNode struct {
	t    tapGate
	taps []time.Duration
}

func (n *tapGateNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(64, 64))
}

func (n *tapGateNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventPointerDown {
		return e.Kind == gift.EventPointerUp
	}
	n.taps = append(n.taps, e.Time)
	for len(n.taps) > 0 && e.Time-n.taps[0] > 2*time.Second {
		n.taps = n.taps[1:]
	}
	if len(n.taps) >= 5 {
		n.taps = nil
		n.t.onOpen()
	}
	return true
}

// background runs fn on a goroutine with a deadline.
func background(fn func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		return fn(ctx)
	}
}

// fill lays its child out in all the room it is offered, where a stack
// would give it only what it needs.
type fill struct{ child gift.View }

var fillType = gift.RegisterType("kiosk.fill")

func (fill) ViewType() gift.TypeID { return fillType }

func (f fill) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: fillLayout{}, Children: []gift.View{f.child}, Flex: 1}
}

type fillLayout struct{}

func (fillLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	size := c.Max
	if size.W > 1e6 || size.H > 1e6 {
		s := ctx.Measure(0, c)
		if size.W > 1e6 {
			size.W = s.W
		}
		if size.H > 1e6 {
			size.H = s.H
		}
	}
	size = c.Constrain(size)
	ctx.Measure(0, geom.Tight(size))
	ctx.Place(0, geom.Point{})
	return size
}
