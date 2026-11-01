package system

import (
	"context"
	"math"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestOSReleaseNamesTheSystem(t *testing.T) {
	pi := parseOSRelease(`PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
ID=debian
`)
	if pi["PRETTY_NAME"] != "Debian GNU/Linux 13 (trixie)" || pi["ID"] != "debian" {
		t.Fatalf("%v", pi)
	}
	if k := kindOf(pi, true); k != RaspberryPiOS {
		t.Errorf("Debian with the Raspberry Pi archive is %v", k)
	}
	if k := kindOf(pi, false); k != Debian {
		t.Errorf("Debian alone is %v", k)
	}
	if k := kindOf(parseOSRelease("ID=ubuntu\nVERSION_ID='26.04'\n"), true); k != Ubuntu {
		t.Errorf("Ubuntu is %v", k)
	}
}

func TestMeminfoAndCPUInfoAreRead(t *testing.T) {
	total, avail := parseMeminfo("MemTotal:       16604096 kB\nMemFree:  100 kB\nMemAvailable:   15110224 kB\n")
	if total != 16604096*1024 || avail != 15110224*1024 {
		t.Fatalf("%d %d", total, avail)
	}
	if c := cpuName("processor\t: 0\nBogoMIPS\t: 108.00\nCPU part\t: 0xd0b\n"); c != "Cortex-A76" {
		t.Errorf("ARM part 0xd0b is %q", c)
	}
	if c := cpuName("model name\t: AMD Ryzen 7 7840U\n"); c != "AMD Ryzen 7 7840U" {
		t.Errorf("x86 is %q", c)
	}
}

func TestMountinfoKeepsTheRealFileSystemsOnce(t *testing.T) {
	ms := parseMountinfo(`24 1 259:2 / / rw,noatime - ext4 /dev/nvme0n1p2 rw
28 24 259:1 / /boot/firmware rw - vfat /dev/nvme0n1p1 rw
30 24 0:25 / /tmp rw - tmpfs tmpfs rw
31 24 7:0 / /snap/core/1 ro - squashfs /dev/loop0 ro
32 24 259:2 /var/lib /mnt/bind rw - ext4 /dev/nvme0n1p2 rw
33 24 8:1 / /media/USB\040Stick rw - exfat /dev/sda1 rw
`)
	var got []string
	for _, m := range ms {
		got = append(got, m.point)
	}
	if want := []string{"/", "/boot/firmware", "/media/USB Stick"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mounts %q, want %q", got, want)
	}
}

func TestASimulatedUpgradeListsItsPackages(t *testing.T) {
	ps := parseAptSimulation(`Reading package lists...
Inst libc6 [2.41-12] (2.41-12+deb13u1 Debian-Security:13/stable-security [arm64])
Inst linux-image-rpi-2712 (1:6.18.50-1+rpt1 Raspberry Pi Foundation:stable [arm64])
Conf libc6 (2.41-12+deb13u1 Debian-Security:13/stable-security [arm64])
`)
	want := []Package{{Name: "libc6", Current: "2.41-12", Candidate: "2.41-12+deb13u1"},
		{Name: "linux-image-rpi-2712", Candidate: "1:6.18.50-1+rpt1"}}
	if !reflect.DeepEqual(ps, want) {
		t.Fatalf("%+v", ps)
	}
}

func TestAptStatusBecomesOneProgress(t *testing.T) {
	dl, ok := parseAptStatus("dlstatus:1:50.0000:Retrieving file 1 of 2")
	if !ok || math.Abs(dl.fraction()-0.1) > 1e-9 {
		t.Fatalf("download half done is %v %v", dl, dl.fraction())
	}
	pm, ok := parseAptStatus("pmstatus:libc6:50.0000:Unpacking libc6 (arm64)")
	if !ok || math.Abs(pm.fraction()-0.6) > 1e-9 || pm.desc != "Unpacking libc6 (arm64)" {
		t.Fatalf("install half done is %+v %v", pm, pm.fraction())
	}
	if e, ok := parseAptStatus("pmerror:/var/cache/x.deb:10:trying to overwrite"); !ok || !e.err {
		t.Fatalf("%+v", e)
	}
	if _, ok := parseAptStatus("media-change: whatever"); ok {
		t.Fatal("a line apt does not send in a kiosk is parsed")
	}
}

func TestWlrRandrPutsThePanelFirstAndTurnsItsSize(t *testing.T) {
	ds, err := parseOutputs(`[
 {"name":"HDMI-A-1","enabled":true,"physical_size":{"width":600,"height":340},
  "modes":[{"width":1920,"height":1080,"refresh":60,"preferred":true,"current":true}],"transform":"normal"},
 {"name":"DSI-1","enabled":true,"physical_size":{"width":87,"height":155},
  "modes":[{"width":720,"height":1280,"refresh":60,"preferred":true,"current":true}],"transform":"90"}
]`)
	if err != nil {
		t.Fatal(err)
	}
	d := ds[0]
	if d.Output != "DSI-1" || d.Width != 1280 || d.Height != 720 || d.Rotation != Rotate90 || d.WidthMM != 155 {
		t.Fatalf("%+v", d)
	}
	if in := d.Diagonal(); in < 6.9 || in > 7.1 {
		t.Errorf("the panel is %.2f inches", in)
	}
}

func TestTheKioskUnitTakesThePlaceOfTheDisplayManager(t *testing.T) {
	u := renderUnit("werkbank", []string{"-x", "it's"})
	for _, want := range []string{
		"Alias=display-manager.service",
		"Environment=GIFT_KIOSK=werkbank",
		"PAMName=login",
		"TTYPath=/dev/tty1",
		"ExecStartPre=-/usr/bin/chvt 1",
		`ExecStart=/usr/bin/labwc --config-dir /etc/werkbank/labwc --session "/usr/local/bin/werkbank -x 'it'\\''s'"`,
	} {
		if !strings.Contains(u, want) {
			t.Errorf("the unit lacks %q:\n%s", want, u)
		}
	}
	if exe, ok := parseUnit(u); !ok || exe != "/usr/local/bin/werkbank" {
		t.Fatalf("the unit is read as %q, %v", exe, ok)
	}
	if _, ok := parseUnit("[Unit]\nDescription=lightdm\n"); ok {
		t.Fatal("a unit of somebody else is taken for the kiosk")
	}
}

func TestTheCompositorOfAKioskSendsTouchAsAMouseToThePanel(t *testing.T) {
	rc := renderRCXML("DSI-1")
	if !strings.Contains(rc, `<touch mapToOutput="DSI-1" mouseEmulation="yes"/>`) || !strings.Contains(rc, "<keybind") {
		t.Fatal(rc)
	}
	if a := renderAutostart("DSI-1", Rotate270); !strings.Contains(a, "wlr-randr --output DSI-1 --transform 270") {
		t.Fatal(a)
	}
	if e := renderEnvironment(map[string]string{"XKBLAYOUT": "de", "XKBMODEL": "pc105"}); !strings.Contains(e, "XKB_DEFAULT_LAYOUT=de") {
		t.Fatal(e)
	}
}

func TestNamesAndAddressesAreChecked(t *testing.T) {
	for _, ok := range []string{"werkbank-1", "a", "Box7"} {
		if err := CheckHostname(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-x", "x-", "a b", "über", strings.Repeat("a", 64)} {
		if CheckHostname(bad) == nil {
			t.Errorf("%q passes", bad)
		}
	}
	if CheckName("Werkbank") == nil || CheckName("1box") == nil || CheckName("werkbank") != nil {
		t.Error("kiosk names")
	}
	c := IPConfig{Method: Manual, Address: netip.MustParsePrefix("10.6.6.247/24"), Router: netip.MustParseAddr("10.6.7.1")}
	if c.Check() == nil {
		t.Error("a router outside the subnet passes")
	}
	c.Router = netip.MustParseAddr("10.6.6.1")
	if err := c.Check(); err != nil {
		t.Error(err)
	}
	if (IPConfig{Method: Manual}).Check() == nil {
		t.Error("a manual configuration without an address passes")
	}
}

func TestSubnetMasksAndPrefixesConvert(t *testing.T) {
	for bits := range 33 {
		m := Mask(bits)
		if got, ok := MaskBits(m); !ok || got != bits {
			t.Errorf("/%d is %s, read back as %d %v", bits, m, got, ok)
		}
	}
	for _, bad := range []string{"255.0.255.0", "255.255.255.1", "x", "::1"} {
		if _, ok := MaskBits(bad); ok {
			t.Errorf("%q is taken for a mask", bad)
		}
	}
}

func TestTheSimulationChangesWhatItReports(t *testing.T) {
	s := Simulated(0)
	ctx := context.Background()
	if err := s.SetHostname(ctx, "werkbank-2"); err != nil {
		t.Fatal(err)
	}
	in, _ := s.Info(ctx)
	if in.Hostname != "werkbank-2" {
		t.Errorf("hostname %q", in.Hostname)
	}
	ch, err := s.Configure(ctx, "eth0", IPConfig{Method: Manual, Address: netip.MustParsePrefix("10.20.2.5/24")})
	if err != nil {
		t.Fatal(err)
	}
	ch.Revert(ctx)
	ifs, _ := s.Interfaces(ctx)
	if ifs[0].Config.Address != netip.MustParsePrefix("10.20.1.17/24") {
		t.Errorf("the revert left %v", ifs[0].Config.Address)
	}
	if err := s.Join(ctx, "wlan0", Join{SSID: "Halle 2", Password: "kurz"}); err != ErrWrongPassword {
		t.Errorf("a short password is %v", err)
	}
}
