//go:build linux

package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// systemd and logind on D-Bus; see org.freedesktop.systemd1(5) and
// org.freedesktop.login1(5).
const (
	sdService = "org.freedesktop.systemd1"
	sdPath    = "/org/freedesktop/systemd1"
	sdManager = "org.freedesktop.systemd1.Manager"
)

func (s *linuxSystem) systemd() (dbus.BusObject, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	return c.Object(sdService, sdPath), nil
}

func (s *linuxSystem) login(ctx context.Context, method string) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	c, err := s.conn()
	if err != nil {
		return err
	}
	return c.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		CallWithContext(ctx, "org.freedesktop.login1.Manager."+method, 0, false).Err
}

func (s *linuxSystem) Reboot(ctx context.Context) error   { return s.login(ctx, "Reboot") }
func (s *linuxSystem) PowerOff(ctx context.Context) error { return s.login(ctx, "PowerOff") }

// desktopManagers are the display managers of the desktops gift knows, in
// the order they are looked for.
var desktopManagers = []string{"lightdm.service", "gdm3.service", "gdm.service", "sddm.service"}

// currentManager is the unit display-manager.service points to now.
func currentManager() string {
	target, err := os.Readlink("/etc/systemd/system/display-manager.service")
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// desktopManager is the display manager of the desktop: the one the kiosk
// remembered when it took its place, or the first one installed.
func desktopManager(name string) string {
	if name != "" {
		if b, err := os.ReadFile(configDir(name) + "/kiosk.conf"); err == nil {
			if dm := parseKeyFile(string(b))["desktop"]; dm != "" {
				return dm
			}
		}
	}
	if dm := currentManager(); dm != "" && !strings.HasSuffix(dm, "-kiosk.service") {
		return dm
	}
	for _, dm := range desktopManagers {
		if fileExists("/usr/lib/systemd/system/"+dm, "/lib/systemd/system/"+dm) {
			return dm
		}
	}
	return ""
}

// installedKiosk is the name of the kiosk installed on this system, found
// by the marker in its unit.
func installedKiosk() (name, exe string) {
	matches, _ := filepath.Glob("/etc/systemd/system/*-kiosk.service")
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if e, ok := parseUnit(string(b)); ok {
			return strings.TrimSuffix(filepath.Base(m), "-kiosk.service"), e
		}
	}
	return "", ""
}

func (s *linuxSystem) Startup(ctx context.Context) (Startup, error) {
	name, exe := installedKiosk()
	st := Startup{Installed: name != "", Executable: exe, Kiosk: os.Getenv(kioskEnv) != ""}
	st.Desktop = desktopManager(name) != ""
	if name != "" && currentManager() == unitName(name) {
		st.Boot = BootKiosk
	}
	return st, nil
}

func (s *linuxSystem) Install(ctx context.Context, in Installation) error {
	if err := CheckName(in.Name); err != nil {
		return err
	}
	if !s.plat.Root {
		return ErrReadOnly
	}
	// The compositor and Xwayland, for Ebiten's X11 window. Raspberry Pi OS
	// has them; Ubuntu has them in universe.
	var missing []string
	for bin, pkg := range map[string]string{"/usr/bin/labwc": "labwc", "/usr/bin/Xwayland": "xwayland", "/usr/bin/wlr-randr": "wlr-randr"} {
		if !fileExists(bin) {
			missing = append(missing, pkg)
		}
	}
	if len(missing) > 0 {
		args := append([]string{"-y", "-q", "-o", "DPkg::Lock::Timeout=300", "install", "--no-install-recommends"}, missing...)
		if _, err := run(ctx, aptEnv, "apt-get", args...); err != nil {
			return fmt.Errorf("system: installing %s: %w", strings.Join(missing, ", "), err)
		}
	}
	src := in.Executable
	if src == "" {
		var err error
		if src, err = os.Executable(); err != nil {
			return err
		}
	}
	if err := copyExecutable(src, programPath(in.Name)); err != nil {
		return err
	}
	dir := configDir(in.Name)
	conf := map[string]string{}
	if b, err := os.ReadFile(dir + "/kiosk.conf"); err == nil {
		conf = parseKeyFile(string(b))
	}
	if conf["desktop"] == "" {
		conf["desktop"] = desktopManager("")
	}
	output := conf["output"]
	if output == "" {
		output = panelOutput()
		conf["output"] = output
	}
	keyboard := map[string]string{}
	if b, err := os.ReadFile("/etc/default/keyboard"); err == nil {
		keyboard = parseKeyFile(strings.ReplaceAll(string(b), `"`, ""))
	}
	files := map[string]string{
		dir + "/kiosk.conf":        renderKeyFile(conf),
		dir + "/labwc/rc.xml":      renderRCXML(output),
		dir + "/labwc/autostart":   renderAutostart(output, rotationOf(conf["rotation"])),
		dir + "/labwc/environment": renderEnvironment(keyboard),
		unitPath(in.Name):          renderUnit(in.Name, in.Args),
	}
	for p, content := range files {
		if err := writeAtomic(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	sd, err := s.systemd()
	if err != nil {
		return err
	}
	if err := sd.CallWithContext(ctx, sdManager+".Reload", 0).Err; err != nil {
		return fmt.Errorf("system: reloading systemd: %w", err)
	}
	return s.SetBoot(ctx, in.Boot, false)
}

// copyExecutable copies a program into place through a temporary file, so
// that the running copy, if it is the one replaced, keeps running.
func copyExecutable(src, dst string) error {
	if same, _ := filepath.EvalSymlinks(src); same == dst {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dst)
}

// panelOutput is the name of the DSI panel, the screen a kiosk's housing
// has, or empty without one.
func panelOutput() string {
	matches, _ := filepath.Glob("/sys/class/drm/card*-DSI-*")
	for _, m := range matches {
		if readTrim(m+"/status") == "connected" {
			_, out, _ := strings.Cut(filepath.Base(m), "-")
			return out
		}
	}
	return ""
}

func renderKeyFile(m map[string]string) string {
	s := kioskMarker + "\n"
	for _, k := range []string{"desktop", "output", "rotation"} {
		if v := m[k]; v != "" {
			s += k + "=" + v + "\n"
		}
	}
	return s
}

func rotationOf(s string) Rotation {
	switch s {
	case "90":
		return Rotate90
	case "180":
		return Rotate180
	case "270":
		return Rotate270
	}
	return Rotate0
}

func (s *linuxSystem) SetBoot(ctx context.Context, b Boot, now bool) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	name, _ := installedKiosk()
	if name == "" && b == BootKiosk {
		return errors.New("system: the program is not installed as the kiosk")
	}
	desktop := desktopManager(name)
	sd, err := s.systemd()
	if err != nil {
		return err
	}
	var changes [][]any
	enable, disable := desktop, unitName(name)
	if b == BootKiosk {
		enable, disable = unitName(name), desktop
	}
	// Enabling installs the alias display-manager.service; the unit that
	// had it must be disabled first, or systemd refuses to overwrite it.
	if disable != "" && disable != unitName("") {
		if err := sd.CallWithContext(ctx, sdManager+".DisableUnitFiles", 0, []string{disable}, false).Store(&changes); err != nil {
			return fmt.Errorf("system: disabling %s: %w", disable, err)
		}
	}
	if enable != "" {
		var carries bool
		if err := sd.CallWithContext(ctx, sdManager+".EnableUnitFiles", 0, []string{enable}, false, true).Store(&carries, &changes); err != nil {
			return fmt.Errorf("system: enabling %s: %w", enable, err)
		}
	}
	if err := sd.CallWithContext(ctx, sdManager+".SetDefaultTarget", 0, "graphical.target", true).Store(&changes); err != nil {
		return fmt.Errorf("system: setting the default target: %w", err)
	}
	if err := sd.CallWithContext(ctx, sdManager+".Reload", 0).Err; err != nil {
		return err
	}
	if !now || enable == "" {
		return nil
	}
	// The jobs are queued and run on their own; stopping the kiosk ends this
	// process when it is the kiosk, after the desktop was asked to start.
	var job dbus.ObjectPath
	if err := sd.CallWithContext(ctx, sdManager+".StartUnit", 0, enable, "replace").Store(&job); err != nil {
		return fmt.Errorf("system: starting %s: %w", enable, err)
	}
	if disable != "" && disable != unitName("") {
		time.Sleep(200 * time.Millisecond)
		sd.CallWithContext(ctx, sdManager+".StopUnit", 0, disable, "replace").Store(&job)
	}
	return nil
}

func (s *linuxSystem) Display(ctx context.Context) (Display, error) {
	var d Display
	if _, err := exec.LookPath("wlr-randr"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		if out, err := run(ctx, nil, "wlr-randr", "--json"); err == nil {
			if ds, err := parseOutputs(out); err == nil && len(ds) > 0 {
				d = ds[0]
			}
		}
	}
	if b, ok := backlight(d.Output); ok {
		d.Brightness, d.HasBrightness = b.level(), true
	}
	if d.Output == "" && !d.HasBrightness {
		return d, ErrUnsupported
	}
	return d, nil
}

func (s *linuxSystem) SetBrightness(ctx context.Context, level float64) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	d, _ := s.Display(ctx)
	b, ok := backlight(d.Output)
	if !ok {
		return ErrUnsupported
	}
	return b.set(level)
}

func (s *linuxSystem) SetRotation(ctx context.Context, r Rotation) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	d, err := s.Display(ctx)
	if err != nil || d.Output == "" {
		return ErrUnsupported
	}
	if _, err := run(ctx, nil, "wlr-randr", "--output", d.Output, "--transform", transformOf(r)); err != nil {
		return err
	}
	// The kiosk turns the screen again when it starts, from its autostart.
	if name, _ := installedKiosk(); name != "" {
		dir := configDir(name)
		conf := map[string]string{}
		if b, err := os.ReadFile(dir + "/kiosk.conf"); err == nil {
			conf = parseKeyFile(string(b))
		}
		conf["output"], conf["rotation"] = d.Output, fmt.Sprint(int(r))
		if err := writeAtomic(dir+"/kiosk.conf", []byte(renderKeyFile(conf)), 0o644); err != nil {
			return err
		}
		return writeAtomic(dir+"/labwc/autostart", []byte(renderAutostart(d.Output, r)), 0o644)
	}
	return nil
}

func (s *linuxSystem) timedate() (dbus.BusObject, error) {
	c, err := s.conn()
	if err != nil {
		return nil, err
	}
	return c.Object("org.freedesktop.timedate1", "/org/freedesktop/timedate1"), nil
}

func (s *linuxSystem) Clock(ctx context.Context) (Clock, error) {
	o, err := s.timedate()
	if err != nil {
		return Clock{}, err
	}
	var c Clock
	prop(o, "org.freedesktop.timedate1.Timezone", &c.Zone)
	prop(o, "org.freedesktop.timedate1.NTP", &c.NTP)
	prop(o, "org.freedesktop.timedate1.NTPSynchronized", &c.Synchronized)
	return c, nil
}

func (s *linuxSystem) TimeZones(ctx context.Context) ([]string, error) {
	o, err := s.timedate()
	if err != nil {
		return nil, err
	}
	var zones []string
	if err := o.CallWithContext(ctx, "org.freedesktop.timedate1.ListTimezones", 0).Store(&zones); err != nil {
		return nil, fmt.Errorf("system: time zones: %w", err)
	}
	return zones, nil
}

func (s *linuxSystem) SetTimeZone(ctx context.Context, zone string) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	o, err := s.timedate()
	if err != nil {
		return err
	}
	return o.CallWithContext(ctx, "org.freedesktop.timedate1.SetTimezone", 0, zone, false).Err
}

func (s *linuxSystem) SetNTP(ctx context.Context, on bool) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	o, err := s.timedate()
	if err != nil {
		return err
	}
	return o.CallWithContext(ctx, "org.freedesktop.timedate1.SetNTP", 0, on, false).Err
}

// bl is one backlight of /sys/class/backlight.
type bl struct{ dir string }

func (b bl) level() float64 {
	var cur, maxv float64
	fmt.Sscan(readTrim(b.dir+"/brightness"), &cur)
	fmt.Sscan(readTrim(b.dir+"/max_brightness"), &maxv)
	if maxv <= 0 {
		return 0
	}
	return cur / maxv
}

func (b bl) set(level float64) error {
	var maxv float64
	fmt.Sscan(readTrim(b.dir+"/max_brightness"), &maxv)
	v := int(max(0, min(level, 1))*maxv + 0.5)
	return os.WriteFile(b.dir+"/brightness", []byte(fmt.Sprint(v)), 0)
}

func backlights() []string {
	m, _ := filepath.Glob("/sys/class/backlight/*")
	return m
}

// backlight is the backlight of output: the one the Raspberry Pi kernel
// names after it, or the only one there is.
func backlight(output string) (bl, bool) {
	all := backlights()
	for _, d := range all {
		if output != "" && readTrim(d+"/display_name") == output {
			return bl{d}, true
		}
	}
	if len(all) == 1 {
		return bl{all[0]}, true
	}
	return bl{}, false
}
