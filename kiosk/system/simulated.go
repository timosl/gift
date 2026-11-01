package system

import (
	"context"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"sync"
	"time"
)

// Simulated returns a system that only pretends: a Raspberry Pi 5 with
// Raspberry Pi OS, wired to a workshop network with a fixed address and
// joined to a wireless one, with updates waiting and a 7 inch touch display.
// Changes change what it reports and nothing else. Operations that take
// time on a real device, scanning, joining, updating, take delay here.
func Simulated(delay time.Duration) System {
	s := &simulated{delay: delay}
	s.reset()
	return s
}

type simulated struct {
	delay time.Duration

	mu        sync.Mutex
	hostname  string
	ifaces    []Interface
	networks  []AccessPoint
	known     []string
	updates   []Package
	startup   Startup
	display   Display
	clock     Clock
	temporary int64
}

func (s *simulated) reset() {
	s.hostname = "werkbank-1"
	s.ifaces = []Interface{
		{
			Device: "eth0", Kind: Ethernet, MAC: "2c:cf:67:1a:20:4e", State: Connected,
			Connection: "Werkstatt fest",
			Addr: IPConfig{Method: Manual, Address: netip.MustParsePrefix("10.20.1.17/24"),
				Router: netip.MustParseAddr("10.20.1.1"), DNS: []netip.Addr{netip.MustParseAddr("10.20.1.1")}},
		},
		{
			Device: "wlan0", Kind: Wifi, MAC: "2c:cf:67:1a:20:4f", State: Connected, Enabled: true,
			Connection: "Werkstatt", SSID: "Werkstatt", Signal: 78,
			Addr: IPConfig{Method: Automatic, Address: netip.MustParsePrefix("192.168.178.42/24"),
				Router: netip.MustParseAddr("192.168.178.1"), DNS: []netip.Addr{netip.MustParseAddr("192.168.178.1")}},
		},
	}
	s.ifaces[0].Config = s.ifaces[0].Addr
	s.ifaces[1].Config = IPConfig{Method: Automatic}
	s.networks = []AccessPoint{
		{SSID: "Werkstatt", Strength: 78, Secured: true, Known: true, Active: true},
		{SSID: "Halle 2", Strength: 64, Secured: true},
		{SSID: "Gast", Strength: 51, Secured: false},
		{SSID: "Lager", Strength: 33, Secured: true, Known: true},
		{SSID: "DIRECT-5C-Drucker", Strength: 18, Secured: true},
	}
	s.known = []string{"Werkstatt", "Lager"}
	s.updates = []Package{
		{Name: "raspi-firmware", Current: "1:1.20260914-1", Candidate: "1:1.20261021-1"},
		{Name: "linux-image-rpi-2712", Current: "1:6.12.62-1+rpt1", Candidate: "1:6.18.50-1+rpt1"},
		{Name: "libc6", Current: "2.41-12", Candidate: "2.41-12+deb13u1"},
		{Name: "openssl", Current: "3.5.4-1~deb13u1", Candidate: "3.5.4-1~deb13u2"},
		{Name: "labwc", Current: "0.9.2-1+rpt2", Candidate: "0.9.3-1+rpt1"},
	}
	s.startup = Startup{Installed: true, Executable: "/usr/local/bin/werkbank", Boot: BootKiosk, Desktop: true, Kiosk: true}
	s.display = Display{Output: "DSI-1", Width: 1280, Height: 720, WidthMM: 155, HeightMM: 87,
		Rotation: Rotate90, Brightness: 0.8, HasBrightness: true}
	s.clock = Clock{Zone: "Europe/Berlin", NTP: true, Synchronized: true}
	s.temporary = 412 << 20
}

// wait sleeps for the delay of the simulation, or until ctx ends.
func (s *simulated) wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (s *simulated) Platform() Platform {
	return Platform{Kind: SimulatedKind, Name: "Raspberry Pi OS", Version: "13 (trixie)", Root: true}
}

func (s *simulated) Can(c Capability) bool { return c != CanReleaseUpgrade }

func (s *simulated) Info(context.Context) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{
		Hostname: s.hostname, OS: "Debian GNU/Linux 13 (trixie)", Kernel: "6.12.62+rpt-rpi-2712",
		Model: "Raspberry Pi 5 Model B Rev 1.1", Serial: "a7f3c20e19b4d65e", Arch: "arm64",
		CPU: "Cortex-A76", Cores: 4, Memory: 8 << 30, MemoryAvailable: 6 << 30,
		Uptime: 3*24*time.Hour + 5*time.Hour + 12*time.Minute, Temperature: 52.6,
	}, nil
}

func (s *simulated) SetHostname(_ context.Context, name string) error {
	if err := CheckHostname(name); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostname = name
	return nil
}

func (s *simulated) Storage(context.Context) (Storage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const total = 58 << 30
	used := []Usage{
		{Name: "System", Bytes: 9 << 30},
		{Name: "Programs", Bytes: 1200 << 20},
		{Name: "Data", Bytes: 14 << 30},
		{Name: "Temporary", Bytes: s.temporary},
	}
	var sum int64
	for _, u := range used {
		sum += u.Bytes
	}
	return Storage{
		Volumes: []Volume{
			{Path: "/", Device: "/dev/mmcblk0p2", Type: "ext4", Total: total, Free: uint64(total - sum)},
			{Path: "/boot/firmware", Device: "/dev/mmcblk0p1", Type: "vfat", Total: 512 << 20, Free: 438 << 20},
		},
		Usage: used, Cleanable: s.temporary,
	}, nil
}

func (s *simulated) CleanTemporary(ctx context.Context) (int64, error) {
	if err := s.wait(ctx, s.delay); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	freed := s.temporary - 24<<20
	s.temporary = 24 << 20
	return max(freed, 0), nil
}

func (s *simulated) Interfaces(context.Context) ([]Interface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.ifaces), nil
}

func (s *simulated) iface(device string) (*Interface, error) {
	for i := range s.ifaces {
		if s.ifaces[i].Device == device {
			return &s.ifaces[i], nil
		}
	}
	return nil, fmt.Errorf("system: no interface %q", device)
}

func (s *simulated) SetWifi(_ context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.iface("wlan0")
	if err != nil {
		return err
	}
	w.Enabled = on
	if on {
		w.State, w.SSID, w.Connection, w.Signal = Connected, "Werkstatt", "Werkstatt", 78
	} else {
		w.State, w.SSID, w.Connection, w.Signal, w.Addr = Unavailable, "", "", 0, IPConfig{}
	}
	return nil
}

func (s *simulated) Scan(ctx context.Context, device string) ([]AccessPoint, error) {
	if err := s.wait(ctx, s.delay); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.iface(device)
	if err != nil {
		return nil, err
	}
	if !w.Enabled {
		return nil, nil
	}
	out := slices.Clone(s.networks)
	for i := range out {
		out[i].Known = slices.Contains(s.known, out[i].SSID)
		out[i].Active = out[i].SSID == w.SSID
	}
	return out, nil
}

func (s *simulated) Join(ctx context.Context, device string, j Join) error {
	if err := s.wait(ctx, 2*s.delay); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.iface(device)
	if err != nil {
		return err
	}
	strength := 60
	for _, ap := range s.networks {
		if ap.SSID == j.SSID {
			strength = ap.Strength
			if ap.Secured && !ap.Known && len(j.Password) < 8 {
				return ErrWrongPassword
			}
		}
	}
	w.State, w.SSID, w.Connection, w.Signal = Connected, j.SSID, j.SSID, strength
	w.Config = j.Config
	w.Addr = IPConfig{Method: j.Config.Method, Address: netip.MustParsePrefix("192.168.178.57/24"),
		Router: netip.MustParseAddr("192.168.178.1"), DNS: []netip.Addr{netip.MustParseAddr("192.168.178.1")}}
	if j.Config.Method == Manual {
		w.Addr = j.Config
	}
	if !slices.Contains(s.known, j.SSID) {
		s.known = append(s.known, j.SSID)
	}
	return nil
}

func (s *simulated) Forget(_ context.Context, ssid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.known = slices.DeleteFunc(s.known, func(k string) bool { return k == ssid })
	for i := range s.ifaces {
		if w := &s.ifaces[i]; w.Kind == Wifi && w.SSID == ssid {
			w.State, w.SSID, w.Connection, w.Signal, w.Addr = Disconnected, "", "", 0, IPConfig{}
		}
	}
	return nil
}

func (s *simulated) Configure(_ context.Context, device string, c IPConfig) (Change, error) {
	if err := c.Check(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.iface(device)
	if err != nil {
		return nil, err
	}
	old := *w
	w.Config = c
	if c.Method == Manual {
		w.Addr = c
	}
	return &simChange{s: s, device: device, old: old}, nil
}

type simChange struct {
	s      *simulated
	device string
	old    Interface
	once   sync.Once
}

func (c *simChange) Timeout() time.Duration     { return 30 * time.Second }
func (c *simChange) Keep(context.Context) error { c.once.Do(func() {}); return nil }
func (c *simChange) Revert(_ context.Context) error {
	c.once.Do(func() {
		c.s.mu.Lock()
		defer c.s.mu.Unlock()
		if w, err := c.s.iface(c.device); err == nil {
			*w = c.old
		}
	})
	return nil
}

func (s *simulated) CheckUpdates(ctx context.Context) ([]Package, error) {
	if err := s.wait(ctx, 2*s.delay); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.updates), nil
}

func (s *simulated) InstallUpdates(ctx context.Context, progress func(Progress)) error {
	s.mu.Lock()
	pkgs := slices.Clone(s.updates)
	s.mu.Unlock()
	steps := 2 * len(pkgs)
	for i := range steps {
		status := "Downloading " + pkgs[i%len(pkgs)].Name
		if i >= len(pkgs) {
			status = "Unpacking " + pkgs[i-len(pkgs)].Name
		}
		if progress != nil {
			progress(Progress{Fraction: float64(i) / float64(steps), Status: status})
		}
		if err := s.wait(ctx, s.delay); err != nil {
			return err
		}
	}
	if progress != nil {
		progress(Progress{Fraction: 1, Status: "Done"})
	}
	s.mu.Lock()
	s.updates = nil
	s.mu.Unlock()
	return nil
}

func (s *simulated) Release(context.Context) (Release, error) {
	return Release{Note: "Raspberry Pi OS is upgraded to a new release by installing a new image."}, nil
}

func (s *simulated) UpgradeRelease(context.Context, func(Progress)) error { return ErrUnsupported }

func (s *simulated) Reboot(context.Context) error   { return nil }
func (s *simulated) PowerOff(context.Context) error { return nil }

func (s *simulated) Startup(context.Context) (Startup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startup, nil
}

func (s *simulated) Install(ctx context.Context, in Installation) error {
	if err := CheckName(in.Name); err != nil {
		return err
	}
	if err := s.wait(ctx, s.delay); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startup.Installed, s.startup.Executable, s.startup.Boot = true, "/usr/local/bin/"+in.Name, in.Boot
	return nil
}

func (s *simulated) SetBoot(_ context.Context, b Boot, _ bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.startup.Installed && b == BootKiosk {
		return fmt.Errorf("system: the program is not installed as the kiosk")
	}
	s.startup.Boot = b
	return nil
}

func (s *simulated) Display(context.Context) (Display, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.display, nil
}

func (s *simulated) SetBrightness(_ context.Context, level float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.display.Brightness = math.Max(0, math.Min(1, level))
	return nil
}

func (s *simulated) SetRotation(_ context.Context, r Rotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.display.Rotation/90)%2 != (r/90)%2 {
		s.display.Width, s.display.Height = s.display.Height, s.display.Width
	}
	s.display.Rotation = r
	return nil
}

func (s *simulated) Clock(context.Context) (Clock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock, nil
}

func (s *simulated) TimeZones(context.Context) ([]string, error) { return commonZones, nil }

func (s *simulated) SetTimeZone(_ context.Context, zone string) error {
	if !slices.Contains(commonZones, zone) {
		return fmt.Errorf("system: unknown time zone %q", zone)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock.Zone = zone
	return nil
}

func (s *simulated) SetNTP(_ context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock.NTP, s.clock.Synchronized = on, on
	return nil
}

// commonZones are the time zones of the simulation.
var commonZones = []string{
	"America/Chicago", "America/Los_Angeles", "America/New_York", "Asia/Shanghai", "Asia/Tokyo",
	"Australia/Sydney", "Europe/Berlin", "Europe/London", "Europe/Paris", "Europe/Vienna",
	"Europe/Zurich", "UTC",
}
