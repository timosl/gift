//go:build linux

package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

// linuxSystem is Raspberry Pi OS, Ubuntu and their relatives.
type linuxSystem struct {
	plat Platform

	mu  sync.Mutex
	bus *dbus.Conn
}

func open() System {
	osr := map[string]string{}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		osr = parseOSRelease(string(b))
	}
	model := readTrim("/proc/device-tree/model")
	kind := kindOf(osr, strings.HasPrefix(model, "Raspberry Pi") && fileExists("/etc/apt/sources.list.d/raspi.sources", "/etc/apt/sources.list.d/raspi.list", "/etc/rpi-issue"))
	p := Platform{Kind: kind, Name: osr["NAME"], Version: osr["VERSION"], Root: os.Geteuid() == 0}
	if kind == RaspberryPiOS {
		p.Name = "Raspberry Pi OS"
	}
	if p.Name == "" {
		p.Name = "Linux"
	}
	return &linuxSystem{plat: p}
}

func (s *linuxSystem) Platform() Platform { return s.plat }

// conn returns the connection to the system bus, connecting once.
func (s *linuxSystem) conn() (*dbus.Conn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bus != nil && s.bus.Connected() {
		return s.bus, nil
	}
	c, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system: the system bus: %w", err)
	}
	s.bus = c
	return c, nil
}

// hasService reports whether a service owns name on the system bus, or can
// be started by it.
func (s *linuxSystem) hasService(name string) bool {
	c, err := s.conn()
	if err != nil {
		return false
	}
	var names []string
	if c.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names) == nil {
		for _, n := range names {
			if n == name {
				return true
			}
		}
	}
	if c.BusObject().Call("org.freedesktop.DBus.ListActivatableNames", 0).Store(&names) == nil {
		for _, n := range names {
			if n == name {
				return true
			}
		}
	}
	return false
}

func (s *linuxSystem) Can(c Capability) bool {
	root := s.plat.Root
	switch c {
	case CanHostname, CanClock:
		return root && s.hasService("org.freedesktop.hostname1")
	case CanNetwork:
		return root && s.hasService(nmService)
	case CanWifi:
		if !root || !s.hasService(nmService) {
			return false
		}
		ifs, err := s.Interfaces(context.Background())
		if err != nil {
			return false
		}
		for _, i := range ifs {
			if i.Kind == Wifi {
				return true
			}
		}
		return false
	case CanUpdates, CanClean:
		_, err := exec.LookPath("apt-get")
		return root && err == nil
	case CanReleaseUpgrade:
		_, err := exec.LookPath("do-release-upgrade")
		return root && s.plat.Kind == Ubuntu && err == nil
	case CanPower:
		return root && s.hasService("org.freedesktop.login1")
	case CanInstall:
		return root && s.hasService("org.freedesktop.systemd1")
	case CanBrightness:
		return root && len(backlights()) > 0
	case CanRotation:
		_, err := exec.LookPath("wlr-randr")
		return root && err == nil && os.Getenv("WAYLAND_DISPLAY") != ""
	}
	return false
}

// run runs a command with LC_ALL=C and returns its standard output; an
// error carries its standard error, never its arguments, which may hold a
// secret.
func run(ctx context.Context, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(append(os.Environ(), "LC_ALL=C", "LANG=C"), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		if msg == "" {
			return out.String(), fmt.Errorf("system: %s: %w", name, err)
		}
		return out.String(), fmt.Errorf("system: %s: %s", name, msg)
	}
	return out.String(), nil
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(string(b), "\x00\n"))
}

func fileExists(paths ...string) bool {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func (s *linuxSystem) Info(ctx context.Context) (Info, error) {
	in := Info{Arch: runtime.GOARCH, Cores: runtime.NumCPU(), Temperature: math.NaN()}
	in.Hostname, _ = os.Hostname()
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		in.OS = parseOSRelease(string(b))["PRETTY_NAME"]
	}
	var u unix.Utsname
	if unix.Uname(&u) == nil {
		in.Kernel = unix.ByteSliceToString(u.Release[:])
	}
	in.Model = readTrim("/proc/device-tree/model")
	in.Serial = strings.TrimLeft(readTrim("/proc/device-tree/serial-number"), "0")
	if in.Model == "" {
		vendor, product := readTrim("/sys/class/dmi/id/sys_vendor"), readTrim("/sys/class/dmi/id/product_name")
		in.Model = strings.TrimSpace(vendor + " " + product)
		in.Serial = readTrim("/sys/class/dmi/id/product_serial")
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		in.CPU = cpuName(string(b))
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		in.Memory, in.MemoryAvailable = parseMeminfo(string(b))
	}
	if f := strings.Fields(readTrim("/proc/uptime")); len(f) > 0 {
		if v, err := strconv.ParseFloat(f[0], 64); err == nil {
			in.Uptime = time.Duration(v * float64(time.Second))
		}
	}
	if v, err := strconv.ParseFloat(readTrim("/sys/class/thermal/thermal_zone0/temp"), 64); err == nil {
		in.Temperature = v / 1000
	}
	in.Undervoltage, in.Throttled = throttling(ctx)
	in.RebootRequired = fileExists("/run/reboot-required")
	return in, nil
}

// throttling reads what a Raspberry Pi reports about its power and its
// temperature since it started: from the hwmon sensor of its firmware, or
// from vcgencmd.
func throttling(ctx context.Context) (under, throttled bool) {
	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*/name")
	for _, m := range matches {
		if readTrim(m) == "rpi_volt" {
			under = readTrim(filepath.Join(filepath.Dir(m), "in0_lcrit_alarm")) == "1"
		}
	}
	if _, err := exec.LookPath("vcgencmd"); err == nil {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if out, err := run(ctx, nil, "vcgencmd", "get_throttled"); err == nil {
			_, hex, _ := strings.Cut(strings.TrimSpace(out), "=")
			if v, err := strconv.ParseUint(strings.TrimPrefix(hex, "0x"), 16, 32); err == nil {
				under = under || v&(1<<16|1) != 0
				throttled = v&(1<<17|1<<18|1<<19|1<<1|1<<2|1<<3) != 0
			}
		}
	}
	return under, throttled
}

func (s *linuxSystem) SetHostname(ctx context.Context, name string) error {
	if err := CheckHostname(name); err != nil {
		return err
	}
	if !s.plat.Root {
		return ErrReadOnly
	}
	c, err := s.conn()
	if err != nil {
		return err
	}
	o := c.Object("org.freedesktop.hostname1", "/org/freedesktop/hostname1")
	if err := o.CallWithContext(ctx, "org.freedesktop.hostname1.SetStaticHostname", 0, name, false).Err; err != nil {
		return fmt.Errorf("system: setting the host name: %w", err)
	}
	if err := o.CallWithContext(ctx, "org.freedesktop.hostname1.SetHostname", 0, name, false).Err; err != nil {
		return fmt.Errorf("system: setting the host name: %w", err)
	}
	// Debian resolves its own name through a 127.0.1.1 line in /etc/hosts,
	// which hostnamed does not touch; sudo complains without it.
	return rewriteHosts(name)
}

// rewriteHosts points the 127.0.1.1 line of /etc/hosts at the new name.
func rewriteHosts(name string) error {
	b, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	found := false
	for i, l := range lines {
		if f := strings.Fields(l); len(f) >= 2 && f[0] == "127.0.1.1" {
			lines[i] = "127.0.1.1\t" + name
			found = true
		}
	}
	if !found {
		lines = append(lines[:len(lines)-1], "127.0.1.1\t"+name, "")
	}
	return writeAtomic("/etc/hosts", []byte(strings.Join(lines, "\n")), 0o644)
}

// writeAtomic writes a file through a temporary one beside it, so that a
// power cut leaves the old file or the new one, never half of one.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

func (s *linuxSystem) Storage(ctx context.Context) (Storage, error) {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return Storage{}, err
	}
	var st Storage
	for _, m := range parseMountinfo(string(b)) {
		var fs unix.Statfs_t
		if unix.Statfs(m.point, &fs) != nil {
			continue
		}
		v := Volume{Path: m.point, Device: m.source, Type: m.fstype,
			Total: fs.Blocks * uint64(fs.Bsize), Free: fs.Bavail * uint64(fs.Bsize)}
		if m.point == "/" {
			st.Volumes = append([]Volume{v}, st.Volumes...)
		} else {
			st.Volumes = append(st.Volumes, v)
		}
	}
	if len(st.Volumes) == 0 || st.Volumes[0].Path != "/" {
		return st, nil
	}
	root := st.Volumes[0]
	tmp := dirSize("/var/cache/apt/archives") + dirSize("/var/tmp") + dirSize("/var/log/journal") + dirSize("/var/crash")
	used := int64(root.Total - root.Free)
	st.Usage = []Usage{{Name: "System", Bytes: max(used-tmp, 0)}, {Name: "Temporary", Bytes: tmp}}
	st.Cleanable = dirSize("/var/cache/apt/archives") + max(dirSize("/var/log/journal")-50<<20, 0) + dirSize("/var/crash")
	return st, nil
}

// dirSize is the size of the regular files under dir, on its file system.
func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func (s *linuxSystem) CleanTemporary(ctx context.Context) (int64, error) {
	if !s.plat.Root {
		return 0, ErrReadOnly
	}
	free := func() int64 {
		var fs unix.Statfs_t
		if unix.Statfs("/", &fs) != nil {
			return 0
		}
		return int64(fs.Bavail * uint64(fs.Bsize))
	}
	before := free()
	var errs []error
	if _, err := exec.LookPath("apt-get"); err == nil {
		if _, err := run(ctx, nil, "apt-get", "-o", "DPkg::Lock::Timeout=60", "clean"); err != nil {
			errs = append(errs, err)
		}
	}
	// Vacuuming removes archived journal files only, so the active one is
	// rotated first.
	if _, err := run(ctx, nil, "journalctl", "--rotate"); err == nil {
		if _, err := run(ctx, nil, "journalctl", "--vacuum-size=50M"); err != nil {
			errs = append(errs, err)
		}
	}
	// The ages the system configures, 10 days for /tmp and 30 for /var/tmp:
	// deleting everything in /tmp would take the sockets of the running
	// compositor and Xwayland along.
	if _, err := run(ctx, nil, "systemd-tmpfiles", "--clean"); err != nil {
		errs = append(errs, err)
	}
	if entries, err := os.ReadDir("/var/crash"); err == nil {
		for _, e := range entries {
			os.RemoveAll(filepath.Join("/var/crash", e.Name()))
		}
	}
	return max(free()-before, 0), errors.Join(errs...)
}
