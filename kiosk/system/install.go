package system

import (
	"fmt"
	"path"
	"strings"
)

// The files of a kiosk, rendered apart from writing them so that they are
// tested on any platform.
//
//	/usr/local/bin/<name>                     the program
//	/etc/<name>/kiosk.conf                    what the kiosk remembers
//	/etc/<name>/labwc/rc.xml                  the compositor: no decorations, no keys, touch as mouse
//	/etc/<name>/labwc/autostart               turns the screen as the settings say
//	/etc/<name>/labwc/environment             the keyboard layout
//	/etc/systemd/system/<name>-kiosk.service  the kiosk, in the place of the display manager

// kioskMarker is the first line of every file the kiosk writes, by which
// [System.Startup] finds the unit again.
const kioskMarker = "# Written by the kiosk of gift (github.com/worldiety/gift/kiosk/system)."

// kioskEnv is set for the program the kiosk starts, so that it knows it is
// the kiosk; its value is the name of the kiosk.
const kioskEnv = "GIFT_KIOSK"

func unitName(name string) string    { return name + "-kiosk.service" }
func unitPath(name string) string    { return "/etc/systemd/system/" + unitName(name) }
func configDir(name string) string   { return "/etc/" + name }
func programPath(name string) string { return "/usr/local/bin/" + name }

// renderUnit is the systemd unit of a kiosk. It takes the place of the
// display manager through its alias, so that enabling it disables the one of
// the desktop, and runs labwc as root on the first virtual terminal, in a
// session of its own that logind gives the seat to. labwc ends when the
// program ends, and the unit starts both again.
func renderUnit(name string, args []string) string {
	cmd := shellQuote(append([]string{programPath(name)}, args...))
	return kioskMarker + `
# The kiosk of ` + name + `: it takes the place of the display manager and
# runs the program full screen, as root. Turn it off and the display manager
# of the desktop on again to boot into the desktop.
[Unit]
Description=Kiosk of ` + name + `
After=systemd-user-sessions.service plymouth-quit-wait.service getty@tty1.service
Conflicts=getty@tty1.service

[Service]
Type=simple
User=root
PAMName=login
TTYPath=/dev/tty1
TTYReset=yes
TTYVHangup=yes
TTYVTDisallocate=yes
StandardInput=tty-fail
StandardOutput=journal
StandardError=journal
UtmpIdentifier=tty1
UtmpMode=user
Environment=` + kioskEnv + `=` + name + `
Environment=XDG_SESSION_TYPE=wayland
Environment=WLR_DRM_FORCE_LIBLIFTOFF=1
# logind gives the seat to the session of the terminal in front; when the
# kiosk starts while the desktop is in front on its own terminal, labwc
# would wait for the seat until somebody switched.
ExecStartPre=-/usr/bin/chvt 1
ExecStart=/usr/bin/labwc --config-dir ` + configDir(name) + `/labwc --session ` + systemdQuote(cmd) + `
Restart=always
RestartSec=2

[Install]
Alias=display-manager.service
WantedBy=graphical.target
`
}

// renderRCXML is the labwc configuration of a kiosk: no window decorations,
// no menus and no key bindings, which would otherwise open a terminal, and
// every touch screen sent as a mouse to output, if one is named, since an
// X11 program under Xwayland only understands the mouse.
func renderRCXML(output string) string {
	touch := `<touch mouseEmulation="yes"/>`
	if output != "" {
		touch = `<touch mapToOutput="` + xmlEscape(output) + `" mouseEmulation="yes"/>`
	}
	return `<?xml version="1.0"?>
<!-- ` + strings.TrimPrefix(kioskMarker, "# ") + ` -->
<labwc_config>
  <core><decoration>server</decoration><gap>0</gap></core>
  <windowRules>
    <windowRule identifier="*" serverDecoration="no"><action name="Maximize"/></windowRule>
  </windowRules>
  <keyboard><default>no</default><keybind key="W-F12"><action name="None"/></keybind></keyboard>
  <mouse><default>no</default>
    <context name="Client"><mousebind button="Left" action="Press"><action name="Focus"/></mousebind></context>
  </mouse>
  ` + touch + `
</labwc_config>
`
}

// renderAutostart is the labwc autostart of a kiosk: it turns the screen as
// the settings say before the program draws its first frame.
func renderAutostart(output string, r Rotation) string {
	s := kioskMarker + "\n"
	if output != "" {
		s += fmt.Sprintf("wlr-randr --output %s --transform %s\n", shellQuote([]string{output}), transformOf(r))
	}
	return s
}

// renderEnvironment is the labwc environment of a kiosk: the keyboard
// layout of the console, which a desktop would take from its own settings.
func renderEnvironment(keyboard map[string]string) string {
	s := kioskMarker + "\n"
	for _, k := range []string{"XKBMODEL", "XKBLAYOUT", "XKBVARIANT", "XKBOPTIONS"} {
		if v := keyboard[k]; v != "" {
			s += "XKB_DEFAULT_" + strings.TrimPrefix(k, "XKB") + "=" + v + "\n"
		}
	}
	return s
}

// transformOf is the name wlr-randr gives a rotation.
func transformOf(r Rotation) string {
	switch r {
	case Rotate90, Rotate180, Rotate270:
		return fmt.Sprint(int(r))
	}
	return "normal"
}

// parseUnit returns the program a kiosk unit starts, from its ExecStart.
func parseUnit(s string) (exe string, ok bool) {
	if !strings.HasPrefix(s, kioskMarker) {
		return "", false
	}
	for _, line := range strings.Split(s, "\n") {
		if v, found := strings.CutPrefix(line, "Environment="+kioskEnv+"="); found {
			return programPath(path.Base(strings.TrimSpace(v))), true
		}
	}
	return "", true
}

// shellQuote joins args into one command line for a shell, each argument in
// single quotes unless it needs none.
func shellQuote(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,+") == "" {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

// systemdQuote quotes s as one argument of an ExecStart line.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(s) + `"`
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
