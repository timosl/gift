package system

import (
	"bufio"
	"encoding/json"
	"strconv"
	"strings"
)

// The parsers of this file read what Linux and its tools write, and are
// kept apart from the code that runs the tools, so that they are tested on
// any platform. Every tool is run with LC_ALL=C, so no parser has to know a
// translation.

// parseOSRelease reads /etc/os-release: KEY=value lines, values optionally
// in double or single quotes.
func parseOSRelease(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
			v = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\$`, `$`, "\\`", "`").Replace(v)
		}
		m[k] = v
	}
	return m
}

// kindOf is the kind of an operating system by its os-release and whether
// it runs on a Raspberry Pi with the Raspberry Pi archive.
func kindOf(osr map[string]string, raspberry bool) Kind {
	switch {
	case osr["ID"] == "ubuntu":
		return Ubuntu
	case osr["ID"] == "raspbian" || osr["ID"] == "debian" && raspberry:
		return RaspberryPiOS
	case osr["ID"] == "debian" || strings.Contains(osr["ID_LIKE"], "debian"):
		return Debian
	}
	return OtherKind
}

// parseMeminfo returns MemTotal and MemAvailable of /proc/meminfo in bytes.
func parseMeminfo(s string) (total, available uint64) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			available = v * 1024
		}
	}
	return total, available
}

// cpuName is the processor of /proc/cpuinfo: the model name an x86 or a
// RISC-V kernel writes, or the core an ARM kernel only writes as a part
// number.
func cpuName(s string) string {
	parts := map[string]string{
		"0xd03": "Cortex-A53", "0xd04": "Cortex-A35", "0xd05": "Cortex-A55", "0xd07": "Cortex-A57",
		"0xd08": "Cortex-A72", "0xd09": "Cortex-A73", "0xd0a": "Cortex-A75", "0xd0b": "Cortex-A76",
		"0xd0c": "Neoverse-N1", "0xd0d": "Cortex-A77", "0xd41": "Cortex-A78", "0xd44": "Cortex-X1",
		"0xd46": "Cortex-A510", "0xd47": "Cortex-A710", "0xd4d": "Cortex-A715", "0xd80": "Cortex-A520",
		"0xd81": "Cortex-A720",
	}
	part := ""
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "model name":
			return v
		case "CPU part":
			part = v
		}
	}
	return parts[part]
}

// mount is one line of /proc/self/mountinfo.
type mount struct {
	point, fstype, source string
}

// parseMountinfo returns the mounts of real file systems: those on a block
// device, without the read-only squashfs of a snap.
func parseMountinfo(s string) []mount {
	var out []mount
	seen := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		pre, post, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		a, b := strings.Fields(pre), strings.Fields(post)
		if len(a) < 5 || len(b) < 2 {
			continue
		}
		m := mount{point: unescapeMount(a[4]), fstype: b[0], source: b[1]}
		switch m.fstype {
		case "ext2", "ext3", "ext4", "vfat", "exfat", "btrfs", "xfs", "f2fs", "ntfs", "ntfs3":
		default:
			continue
		}
		if !strings.HasPrefix(m.source, "/dev/") || strings.HasPrefix(m.source, "/dev/loop") || seen[m.source] {
			continue
		}
		seen[m.source] = true
		out = append(out, m)
	}
	return out
}

// unescapeMount undoes the octal escapes of mountinfo, \040 for a space.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseAptSimulation returns the packages a simulated upgrade would install,
// from its "Inst name [old] (new archive [arch])" lines. A package that is
// new has no old version.
func parseAptSimulation(s string) []Package {
	var out []Package
	for _, line := range strings.Split(s, "\n") {
		rest, ok := strings.CutPrefix(line, "Inst ")
		if !ok {
			continue
		}
		name, rest, _ := strings.Cut(rest, " ")
		p := Package{Name: name}
		if strings.HasPrefix(rest, "[") {
			if i := strings.IndexByte(rest, ']'); i > 0 {
				p.Current = rest[1:i]
				rest = strings.TrimSpace(rest[i+1:])
			}
		}
		if strings.HasPrefix(rest, "(") {
			p.Candidate, _, _ = strings.Cut(rest[1:], " ")
		}
		out = append(out, p)
	}
	return out
}

// aptStatus is one line apt writes to its APT::Status-Fd: the phase,
// "dlstatus" for downloading and "pmstatus" for installing, how far the
// phase is in percent, and what happens.
type aptStatus struct {
	phase   string
	percent float64
	desc    string
	err     bool
}

// parseAptStatus reads one line of APT::Status-Fd, such as
// "pmstatus:libc6:42.8571:Unpacking libc6 (arm64)".
func parseAptStatus(line string) (aptStatus, bool) {
	f := strings.SplitN(strings.TrimSpace(line), ":", 4)
	if len(f) < 4 {
		return aptStatus{}, false
	}
	pct, err := strconv.ParseFloat(f[2], 64)
	if err != nil {
		return aptStatus{}, false
	}
	switch f[0] {
	case "dlstatus", "pmstatus":
		return aptStatus{phase: f[0], percent: pct, desc: f[3]}, true
	case "pmerror":
		return aptStatus{phase: f[0], percent: pct, desc: f[1] + ": " + f[3], err: true}, true
	}
	return aptStatus{}, false
}

// fraction is how far an installation is, downloading taking the first
// fifth and installing the rest.
func (s aptStatus) fraction() float64 {
	p := max(0, min(s.percent, 100)) / 100
	if s.phase == "dlstatus" {
		return 0.2 * p
	}
	return 0.2 + 0.8*p
}

// output is one output of `wlr-randr --json`.
type output struct {
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	PhysicalSize struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"physical_size"`
	Modes []struct {
		Width     int  `json:"width"`
		Height    int  `json:"height"`
		Preferred bool `json:"preferred"`
		Current   bool `json:"current"`
	} `json:"modes"`
	Transform string `json:"transform"`
}

// parseOutputs reads `wlr-randr --json` into displays, the enabled ones
// first, a DSI panel before anything else, as a kiosk's own screen is the
// panel in its housing.
func parseOutputs(s string) ([]Display, error) {
	var outs []output
	if err := json.Unmarshal([]byte(s), &outs); err != nil {
		return nil, err
	}
	var on, off []Display
	for _, o := range outs {
		d := Display{Output: o.Name, WidthMM: o.PhysicalSize.Width, HeightMM: o.PhysicalSize.Height}
		for _, m := range o.Modes {
			if m.Current || (d.Width == 0 && m.Preferred) {
				d.Width, d.Height = m.Width, m.Height
			}
		}
		switch strings.TrimPrefix(o.Transform, "flipped-") {
		case "90":
			d.Rotation = Rotate90
		case "180":
			d.Rotation = Rotate180
		case "270":
			d.Rotation = Rotate270
		}
		if d.Rotation == Rotate90 || d.Rotation == Rotate270 {
			d.Width, d.Height = d.Height, d.Width
			d.WidthMM, d.HeightMM = d.HeightMM, d.WidthMM
		}
		switch {
		case !o.Enabled:
			off = append(off, d)
		case strings.HasPrefix(o.Name, "DSI"):
			on = append([]Display{d}, on...)
		default:
			on = append(on, d)
		}
	}
	return append(on, off...), nil
}

// keyFile is a file of key=value lines, as the kiosk keeps its state in.
func parseKeyFile(s string) map[string]string {
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if k, v, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(line, "#") {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}
