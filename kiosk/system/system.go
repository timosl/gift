package system

import (
	"context"
	"errors"
	"math"
	"net/netip"
	"time"
)

// ErrReadOnly reports a change the system does not allow: the process does
// not run as root, or the platform is one gift only reads, such as macOS.
var ErrReadOnly = errors.New("system: read-only")

// ErrUnsupported reports something the system does not have, such as the
// brightness of an HDMI monitor or a release upgrade on Raspberry Pi OS.
var ErrUnsupported = errors.New("system: not supported")

// ErrWrongPassword reports that a wireless network rejected its password.
var ErrWrongPassword = errors.New("system: wrong password")

// System is the operating system of the device, as far as a settings app
// needs it. [Open] returns the one the program runs on, [Simulated] one that
// only pretends.
//
// Every method may block, on a D-Bus call, on a file or on a command, and is
// meant for a goroutine of its own; none of them touches the user interface.
// A method of something the system cannot do returns [ErrUnsupported] or
// [ErrReadOnly]; [System.Can] tells beforehand.
type System interface {
	// Platform describes the system.
	Platform() Platform
	// Can reports whether the system can do c.
	Can(c Capability) bool

	// Info describes the device and its operating system.
	Info(ctx context.Context) (Info, error)
	// SetHostname renames the device.
	SetHostname(ctx context.Context, name string) error

	// Storage reports the file systems and what takes their space.
	Storage(ctx context.Context) (Storage, error)
	// CleanTemporary deletes what the system can do without: downloaded
	// packages, old journal files, expired temporary files. It reports the
	// bytes it freed.
	CleanTemporary(ctx context.Context) (int64, error)

	// Interfaces lists the network interfaces, wired and wireless.
	Interfaces(ctx context.Context) ([]Interface, error)
	// SetWifi turns the wireless radio on or off.
	SetWifi(ctx context.Context, on bool) error
	// Scan searches for wireless networks with the device named device, and
	// lists each network once, by its strongest access point.
	Scan(ctx context.Context, device string) ([]AccessPoint, error)
	// Join connects device to a wireless network, remembering it.
	Join(ctx context.Context, device string, j Join) error
	// Forget deletes what the system remembers of a wireless network.
	Forget(ctx context.Context, ssid string) error
	// Configure sets the IPv4 configuration of the connection of device. The
	// change takes effect at once and is undone after the timeout of the
	// returned [Change] unless it is kept: a wrong address must not cut off
	// a device for good.
	Configure(ctx context.Context, device string, c IPConfig) (Change, error)

	// CheckUpdates refreshes the package lists and returns the packages that
	// have a newer version.
	CheckUpdates(ctx context.Context) ([]Package, error)
	// InstallUpdates installs the newer versions, reporting its progress.
	InstallUpdates(ctx context.Context, progress func(Progress)) error
	// Release reports whether a newer release of the operating system is
	// available, and whether it can be upgraded to in place.
	Release(ctx context.Context) (Release, error)
	// UpgradeRelease upgrades to the newer release.
	UpgradeRelease(ctx context.Context, progress func(Progress)) error

	// Reboot restarts the device, PowerOff shuts it down.
	Reboot(ctx context.Context) error
	PowerOff(ctx context.Context) error

	// Startup reports how the device starts.
	Startup(ctx context.Context) (Startup, error)
	// Install installs the program as the kiosk of the device; see
	// [Installation].
	Install(ctx context.Context, in Installation) error
	// SetBoot chooses what the device starts into the next time, and with
	// now set switches at once.
	SetBoot(ctx context.Context, b Boot, now bool) error

	// Display reports the screen.
	Display(ctx context.Context) (Display, error)
	// SetBrightness sets the backlight, from 0 to 1.
	SetBrightness(ctx context.Context, level float64) error
	// SetRotation turns the screen.
	SetRotation(ctx context.Context, r Rotation) error

	// Clock reports the time zone and the synchronisation of the clock.
	Clock(ctx context.Context) (Clock, error)
	// TimeZones lists the time zones the system knows, such as
	// "Europe/Berlin".
	TimeZones(ctx context.Context) ([]string, error)
	// SetTimeZone and SetNTP change them.
	SetTimeZone(ctx context.Context, zone string) error
	SetNTP(ctx context.Context, on bool) error
}

// Kind is the family of an operating system.
type Kind uint8

// The kinds of [Platform].
const (
	OtherKind Kind = iota
	RaspberryPiOS
	Ubuntu
	Debian
	MacOS
	SimulatedKind
)

// Platform describes the system a program runs on.
type Platform struct {
	Kind Kind
	// Name and Version are as the system names itself, such as "Raspberry
	// Pi OS" and "13 (trixie)".
	Name, Version string
	// Root reports whether the process may change the system.
	Root bool
}

// Capability is something a system may or may not be able to do.
type Capability uint16

// The capabilities of [System.Can].
const (
	CanHostname Capability = iota
	CanNetwork
	CanWifi
	CanUpdates
	CanReleaseUpgrade
	CanPower
	CanInstall
	CanBrightness
	CanRotation
	CanClock
	CanClean
)

// Info describes a device and its operating system.
type Info struct {
	Hostname string
	// OS is the full name of the operating system, such as "Debian
	// GNU/Linux 13 (trixie)", and Kernel the release of its kernel.
	OS, Kernel string
	// Model is the hardware, such as "Raspberry Pi 5 Model B Rev 1.0", and
	// Serial its serial number, if the system tells.
	Model, Serial string
	Arch          string
	// CPU names the processor, Cores counts its cores.
	CPU   string
	Cores int
	// Memory is the RAM in bytes, MemoryAvailable what is free for programs.
	Memory, MemoryAvailable uint64
	Uptime                  time.Duration
	// Temperature is the temperature of the processor in degrees Celsius,
	// or NaN if the system does not tell.
	Temperature float64
	// Undervoltage reports that the supply of a Raspberry Pi dropped below
	// its limit since it started, Throttled that it slowed down for that or
	// for the heat.
	Undervoltage, Throttled bool
	// RebootRequired reports that an update waits for a restart to take
	// effect.
	RebootRequired bool
}

// Storage is the file systems of a device and what takes their space.
type Storage struct {
	Volumes []Volume
	// Usage divides the space of the first volume into categories, such as
	// "System", "Temporary" and the data of the program.
	Usage []Usage
	// Cleanable is what [System.CleanTemporary] would free, about.
	Cleanable int64
}

// Volume is one mounted file system.
type Volume struct {
	Path, Device, Type string
	Total, Free        uint64
}

// Usage is the space one category takes.
type Usage struct {
	Name  string
	Bytes int64
}

// InterfaceKind is the kind of a network interface.
type InterfaceKind uint8

// The kinds of [Interface].
const (
	Ethernet InterfaceKind = iota + 1
	Wifi
)

// LinkState is the state of a network interface.
type LinkState uint8

// The states of [Interface].
const (
	Unavailable LinkState = iota
	Disconnected
	Connecting
	Connected
)

// Interface is a network interface and its connection.
type Interface struct {
	// Device is the name of the interface, such as "eth0" or "wlan0".
	Device string
	Kind   InterfaceKind
	MAC    string
	State  LinkState
	// Connection names the profile the interface is connected with, and
	// SSID the wireless network it is joined to.
	Connection string
	SSID       string
	// Signal is the strength of the wireless network, 0 to 100.
	Signal int
	// Enabled reports whether the radio of a wireless interface is on.
	Enabled bool
	// Addr is the address the interface has now, Config the configuration
	// it is set to.
	Addr   IPConfig
	Config IPConfig
}

// Method is how an interface gets its IPv4 address.
type Method uint8

// The methods of [IPConfig].
const (
	Automatic Method = iota
	Manual
)

// IPConfig is an IPv4 configuration. With [Automatic] the address, the
// router and, unless DNS is set, the name servers come from DHCP.
type IPConfig struct {
	Method  Method
	Address netip.Prefix
	Router  netip.Addr
	DNS     []netip.Addr
}

// AccessPoint is a wireless network in reach.
type AccessPoint struct {
	SSID string
	// Strength is the signal, 0 to 100.
	Strength int
	Secured  bool
	// Known reports that the system remembers the network, Active that the
	// interface is joined to it.
	Known, Active bool
}

// Join is what joining a wireless network takes.
type Join struct {
	SSID, Password string
	// Hidden is set for a network that does not announce itself.
	Hidden bool
	// Config is the IPv4 configuration of the new connection.
	Config IPConfig
}

// Change is a network configuration that is in effect but not yet kept.
type Change interface {
	// Timeout is how long after the change it is undone unless kept.
	Timeout() time.Duration
	// Keep makes the change permanent, Revert undoes it at once.
	Keep(ctx context.Context) error
	Revert(ctx context.Context) error
}

// Package is a package with a newer version.
type Package struct {
	Name, Current, Candidate string
}

// Progress is how far an installation is.
type Progress struct {
	// Fraction is from 0 to 1, Status says what happens, such as
	// "Unpacking libc6".
	Fraction float64
	Status   string
}

// Release is the next release of the operating system.
type Release struct {
	// Available reports a newer release, named Name.
	Available bool
	Name      string
	// Supported reports that it can be upgraded to in place; Note says why
	// not, such as "Raspberry Pi OS is upgraded by installing a new image".
	Supported bool
	Note      string
}

// Boot is what a device starts into.
type Boot uint8

// The ways of [Startup].
const (
	// BootDesktop starts the desktop of the operating system.
	BootDesktop Boot = iota
	// BootKiosk starts the installed program full screen, as root.
	BootKiosk
)

// Startup is how a device starts.
type Startup struct {
	// Installed reports that the program is installed as the kiosk, as
	// Executable, and Boot what the device starts into.
	Installed  bool
	Executable string
	Boot       Boot
	// Desktop reports that the system has a desktop to boot into.
	Desktop bool
	// Kiosk reports that this process runs as the kiosk.
	Kiosk bool
}

// Installation describes the program to install as the kiosk.
type Installation struct {
	// Name identifies the kiosk in file and unit names, such as "fotobox".
	Name string
	// Executable is the program to copy; empty is the running one.
	Executable string
	// Args are passed to the program when the kiosk starts it.
	Args []string
	// Boot is what the device starts into from now on.
	Boot Boot
}

// Rotation is the orientation of a screen, clockwise.
type Rotation uint16

// The rotations of [Display].
const (
	Rotate0   Rotation = 0
	Rotate90  Rotation = 90
	Rotate180 Rotation = 180
	Rotate270 Rotation = 270
)

// Display describes the screen.
type Display struct {
	// Output names the screen, such as "DSI-1".
	Output string
	// Width and Height are in pixels, as the screen is turned; WidthMM and
	// HeightMM its physical size, zero if unknown.
	Width, Height     int
	WidthMM, HeightMM int
	Rotation          Rotation
	// Brightness is from 0 to 1, valid if HasBrightness.
	Brightness    float64
	HasBrightness bool
}

// Diagonal is the size of the screen in inches, or 0 if unknown.
func (d Display) Diagonal() float64 {
	if d.WidthMM <= 0 || d.HeightMM <= 0 {
		return 0
	}
	w, h := float64(d.WidthMM), float64(d.HeightMM)
	return math.Sqrt(w*w+h*h) / 25.4
}

// Clock is the time zone of a device and the synchronisation of its clock.
type Clock struct {
	Zone string
	// NTP reports that the clock is synchronised from the network, and
	// Synchronized that it is now.
	NTP, Synchronized bool
}
