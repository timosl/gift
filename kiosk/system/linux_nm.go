//go:build linux

package system

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// NetworkManager on D-Bus; see https://networkmanager.dev/docs/api/latest/.
const (
	nmService    = "org.freedesktop.NetworkManager"
	nmPath       = "/org/freedesktop/NetworkManager"
	nmSettings   = "/org/freedesktop/NetworkManager/Settings"
	nmIface      = "org.freedesktop.NetworkManager"
	nmDevice     = nmIface + ".Device"
	nmWireless   = nmIface + ".Device.Wireless"
	nmAP         = nmIface + ".AccessPoint"
	nmActive     = nmIface + ".Connection.Active"
	nmIP4        = nmIface + ".IP4Config"
	nmConnection = nmIface + ".Settings.Connection"
)

// The device types and states of NetworkManager that matter here.
const (
	nmTypeEthernet = 1
	nmTypeWifi     = 2

	nmStateUnmanaged    = 10
	nmStateUnavailable  = 20
	nmStateDisconnected = 30
	nmStateActivated    = 100
	nmStateFailed       = 120
)

// settings is the settings of a NetworkManager connection, by setting and
// key, as GetSettings and Update take them.
type settings = map[string]map[string]dbus.Variant

func (s *linuxSystem) nm() (dbus.BusObject, *dbus.Conn, error) {
	c, err := s.conn()
	if err != nil {
		return nil, nil, err
	}
	return c.Object(nmService, nmPath), c, nil
}

// prop reads one property into v.
func prop(o dbus.BusObject, name string, v any) error {
	p, err := o.GetProperty(name)
	if err != nil {
		return err
	}
	return p.Store(v)
}

// device is what Interfaces needs of one NetworkManager device.
type device struct {
	path  dbus.ObjectPath
	iface Interface
}

func (s *linuxSystem) devices(ctx context.Context) ([]device, error) {
	nm, c, err := s.nm()
	if err != nil {
		return nil, err
	}
	var paths []dbus.ObjectPath
	if err := nm.CallWithContext(ctx, nmIface+".GetDevices", 0).Store(&paths); err != nil {
		return nil, fmt.Errorf("system: NetworkManager: %w", err)
	}
	var wifiOn bool
	prop(nm, nmIface+".WirelessEnabled", &wifiOn)
	var out []device
	for _, p := range paths {
		o := c.Object(nmService, p)
		var typ, state uint32
		if prop(o, nmDevice+".DeviceType", &typ) != nil || (typ != nmTypeEthernet && typ != nmTypeWifi) {
			continue
		}
		prop(o, nmDevice+".State", &state)
		if state == nmStateUnmanaged {
			continue
		}
		d := device{path: p}
		in := &d.iface
		prop(o, nmDevice+".Interface", &in.Device)
		prop(o, nmDevice+".HwAddress", &in.MAC)
		in.MAC = strings.ToLower(in.MAC)
		in.Kind, in.Enabled = Ethernet, true
		if typ == nmTypeWifi {
			in.Kind, in.Enabled = Wifi, wifiOn
		}
		switch {
		case state >= nmStateActivated && state < nmStateFailed:
			in.State = Connected
		case state > nmStateDisconnected && state < nmStateActivated:
			in.State = Connecting
		case state == nmStateDisconnected || state == nmStateFailed:
			in.State = Disconnected
		default:
			in.State = Unavailable
		}
		var active dbus.ObjectPath
		if prop(o, nmDevice+".ActiveConnection", &active) == nil && active != "/" {
			a := c.Object(nmService, active)
			prop(a, nmActive+".Id", &in.Connection)
			var conn dbus.ObjectPath
			if prop(a, nmActive+".Connection", &conn) == nil {
				var st settings
				if c.Object(nmService, conn).CallWithContext(ctx, nmConnection+".GetSettings", 0).Store(&st) == nil {
					in.Config = configOf(st)
				}
			}
		}
		if typ == nmTypeWifi {
			var ap dbus.ObjectPath
			if prop(o, nmWireless+".ActiveAccessPoint", &ap) == nil && ap != "/" {
				a := c.Object(nmService, ap)
				var ssid []byte
				var strength uint8
				prop(a, nmAP+".Ssid", &ssid)
				prop(a, nmAP+".Strength", &strength)
				in.SSID, in.Signal = string(ssid), int(strength)
			}
		}
		var ip4 dbus.ObjectPath
		if in.State == Connected && prop(o, nmDevice+".Ip4Config", &ip4) == nil && ip4 != "/" {
			in.Addr = addressOf(c.Object(nmService, ip4))
			in.Addr.Method = in.Config.Method
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].iface.Kind != out[j].iface.Kind {
			return out[i].iface.Kind < out[j].iface.Kind
		}
		return out[i].iface.Device < out[j].iface.Device
	})
	return out, nil
}

// addressOf reads the IPv4 configuration a device has now.
func addressOf(o dbus.BusObject) IPConfig {
	var c IPConfig
	var addrs []map[string]dbus.Variant
	if prop(o, nmIP4+".AddressData", &addrs) == nil && len(addrs) > 0 {
		var a string
		var p uint32
		addrs[0]["address"].Store(&a)
		addrs[0]["prefix"].Store(&p)
		if ip, err := netip.ParseAddr(a); err == nil {
			c.Address = netip.PrefixFrom(ip, int(p))
		}
	}
	var gw string
	if prop(o, nmIP4+".Gateway", &gw) == nil {
		c.Router, _ = netip.ParseAddr(gw)
	}
	var ns []map[string]dbus.Variant
	if prop(o, nmIP4+".NameserverData", &ns) == nil {
		for _, n := range ns {
			var a string
			if n["address"].Store(&a) == nil {
				if ip, err := netip.ParseAddr(a); err == nil {
					c.DNS = append(c.DNS, ip)
				}
			}
		}
	}
	return c
}

// configOf reads the IPv4 configuration of the settings of a connection.
func configOf(st settings) IPConfig {
	var c IPConfig
	ip4 := st["ipv4"]
	var method string
	if v, ok := ip4["method"]; ok {
		v.Store(&method)
	}
	if method == "manual" {
		c.Method = Manual
	}
	if v, ok := ip4["address-data"]; ok {
		var addrs []map[string]dbus.Variant
		if v.Store(&addrs) == nil && len(addrs) > 0 {
			var a string
			var p uint32
			addrs[0]["address"].Store(&a)
			addrs[0]["prefix"].Store(&p)
			if ip, err := netip.ParseAddr(a); err == nil {
				c.Address = netip.PrefixFrom(ip, int(p))
			}
		}
	}
	if v, ok := ip4["gateway"]; ok {
		var g string
		if v.Store(&g) == nil {
			c.Router, _ = netip.ParseAddr(g)
		}
	}
	if v, ok := ip4["dns"]; ok {
		var ns []uint32
		if v.Store(&ns) == nil {
			for _, n := range ns {
				var b [4]byte
				binary.LittleEndian.PutUint32(b[:], n)
				c.DNS = append(c.DNS, netip.AddrFrom4(b))
			}
		}
	}
	return c
}

// applyConfig writes c into the IPv4 setting of a connection. The keys that
// NetworkManager deprecated, addresses and dns-data, are removed: with
// addresses set it would ignore address-data, and dns-data would be read
// before dns.
func applyConfig(st settings, c IPConfig) {
	ip4 := st["ipv4"]
	if ip4 == nil {
		ip4 = map[string]dbus.Variant{}
		st["ipv4"] = ip4
	}
	for _, k := range []string{"addresses", "address-data", "gateway", "dns", "dns-data", "ignore-auto-dns"} {
		delete(ip4, k)
	}
	if c.Method == Manual {
		ip4["method"] = dbus.MakeVariant("manual")
		ip4["address-data"] = dbus.MakeVariant([]map[string]dbus.Variant{{
			"address": dbus.MakeVariant(c.Address.Addr().String()),
			"prefix":  dbus.MakeVariant(uint32(c.Address.Bits())),
		}})
		if c.Router.IsValid() {
			ip4["gateway"] = dbus.MakeVariant(c.Router.String())
		}
	} else {
		ip4["method"] = dbus.MakeVariant("auto")
	}
	if len(c.DNS) > 0 {
		ns := make([]uint32, 0, len(c.DNS))
		for _, d := range c.DNS {
			b := d.As4()
			ns = append(ns, binary.LittleEndian.Uint32(b[:]))
		}
		ip4["dns"] = dbus.MakeVariant(ns)
		ip4["ignore-auto-dns"] = dbus.MakeVariant(true)
	}
	// IPv6 is left as it is: a kiosk on a building network needs IPv4, and
	// a link-local IPv6 address hurts nobody.
}

func (s *linuxSystem) Interfaces(ctx context.Context) ([]Interface, error) {
	ds, err := s.devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Interface, len(ds))
	for i, d := range ds {
		out[i] = d.iface
	}
	return out, nil
}

func (s *linuxSystem) device(ctx context.Context, name string) (device, error) {
	ds, err := s.devices(ctx)
	if err != nil {
		return device{}, err
	}
	for _, d := range ds {
		if d.iface.Device == name {
			return d, nil
		}
	}
	return device{}, fmt.Errorf("system: no network interface %q", name)
}

func (s *linuxSystem) SetWifi(ctx context.Context, on bool) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	nm, _, err := s.nm()
	if err != nil {
		return err
	}
	return nm.SetProperty(nmIface+".WirelessEnabled", dbus.MakeVariant(on))
}

func (s *linuxSystem) Scan(ctx context.Context, name string) ([]AccessPoint, error) {
	d, err := s.device(ctx, name)
	if err != nil {
		return nil, err
	}
	if d.iface.Kind != Wifi {
		return nil, fmt.Errorf("system: %s is not a wireless interface", name)
	}
	_, c, _ := s.nm()
	o := c.Object(nmService, d.path)
	var before int64
	prop(o, nmWireless+".LastScan", &before)
	// NetworkManager refuses a scan right after the last one; the access
	// points it has are then recent enough, so the refusal is not an error.
	if s.plat.Root && o.CallWithContext(ctx, nmWireless+".RequestScan", 0, map[string]dbus.Variant{}).Err == nil {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var last int64
			if prop(o, nmWireless+".LastScan", &last) == nil && last != before {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
	var aps []dbus.ObjectPath
	if err := o.CallWithContext(ctx, nmWireless+".GetAllAccessPoints", 0).Store(&aps); err != nil {
		return nil, fmt.Errorf("system: NetworkManager: %w", err)
	}
	known := s.knownSSIDs(ctx)
	best := map[string]AccessPoint{}
	for _, p := range aps {
		a := c.Object(nmService, p)
		var ssid []byte
		var strength uint8
		var flags, wpa, rsn uint32
		prop(a, nmAP+".Ssid", &ssid)
		if len(ssid) == 0 {
			continue
		}
		prop(a, nmAP+".Strength", &strength)
		prop(a, nmAP+".Flags", &flags)
		prop(a, nmAP+".WpaFlags", &wpa)
		prop(a, nmAP+".RsnFlags", &rsn)
		ap := AccessPoint{SSID: string(ssid), Strength: int(strength), Secured: flags&1 != 0 || wpa != 0 || rsn != 0}
		ap.Known = known[ap.SSID] != nil
		ap.Active = ap.SSID == d.iface.SSID && d.iface.State == Connected
		if old, ok := best[ap.SSID]; !ok || ap.Strength > old.Strength {
			best[ap.SSID] = ap
		}
	}
	out := make([]AccessPoint, 0, len(best))
	for _, ap := range best {
		out = append(out, ap)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		if out[i].Strength != out[j].Strength {
			return out[i].Strength > out[j].Strength
		}
		return out[i].SSID < out[j].SSID
	})
	return out, nil
}

// knownSSIDs returns the saved wireless connections by the SSID they join.
func (s *linuxSystem) knownSSIDs(ctx context.Context) map[string][]dbus.ObjectPath {
	_, c, err := s.nm()
	if err != nil {
		return nil
	}
	var conns []dbus.ObjectPath
	if c.Object(nmService, nmSettings).CallWithContext(ctx, nmIface+".Settings.ListConnections", 0).Store(&conns) != nil {
		return nil
	}
	out := map[string][]dbus.ObjectPath{}
	for _, p := range conns {
		var st settings
		if c.Object(nmService, p).CallWithContext(ctx, nmConnection+".GetSettings", 0).Store(&st) != nil {
			continue
		}
		var typ string
		st["connection"]["type"].Store(&typ)
		if typ != "802-11-wireless" {
			continue
		}
		var ssid []byte
		if v, ok := st["802-11-wireless"]["ssid"]; ok && v.Store(&ssid) == nil {
			out[string(ssid)] = append(out[string(ssid)], p)
		}
	}
	return out
}

func (s *linuxSystem) Join(ctx context.Context, name string, j Join) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	if err := j.Config.Check(); err != nil {
		return err
	}
	d, err := s.device(ctx, name)
	if err != nil {
		return err
	}
	nm, c, _ := s.nm()
	// A network the system remembers is joined with what it remembers,
	// unless a new password is given: then the old profile is replaced.
	known := s.knownSSIDs(ctx)[j.SSID]
	var active dbus.ObjectPath
	if len(known) > 0 && j.Password == "" {
		if err := nm.CallWithContext(ctx, nmIface+".ActivateConnection", 0, known[0], d.path, dbus.ObjectPath("/")).Store(&active); err != nil {
			return fmt.Errorf("system: joining %q: %w", j.SSID, err)
		}
		return s.waitActive(ctx, active, d.path, nil)
	}
	for _, p := range known {
		c.Object(nmService, p).CallWithContext(ctx, nmConnection+".Delete", 0)
	}
	st := settings{
		"connection": {
			"id":   dbus.MakeVariant(j.SSID),
			"type": dbus.MakeVariant("802-11-wireless"),
		},
		"802-11-wireless": {
			"ssid": dbus.MakeVariant([]byte(j.SSID)),
			"mode": dbus.MakeVariant("infrastructure"),
		},
	}
	if j.Hidden {
		st["802-11-wireless"]["hidden"] = dbus.MakeVariant(true)
	}
	if j.Password != "" {
		st["802-11-wireless-security"] = map[string]dbus.Variant{
			"key-mgmt": dbus.MakeVariant("wpa-psk"),
			"psk":      dbus.MakeVariant(j.Password),
		}
	}
	applyConfig(st, j.Config)
	var conn dbus.ObjectPath
	if err := nm.CallWithContext(ctx, nmIface+".AddAndActivateConnection", 0, st, d.path, dbus.ObjectPath("/")).Store(&conn, &active); err != nil {
		return fmt.Errorf("system: joining %q: %w", j.SSID, err)
	}
	// A profile that never connected is not remembered: a mistyped password
	// must not stay behind as a network the device keeps trying.
	return s.waitActive(ctx, active, d.path, func() {
		c.Object(nmService, conn).Call(nmConnection+".Delete", 0)
	})
}

// waitActive waits until an activation succeeds or fails, and tells a
// rejected password from other failures by the reason of the device.
func (s *linuxSystem) waitActive(ctx context.Context, active, dev dbus.ObjectPath, failed func()) error {
	_, c, _ := s.nm()
	a, o := c.Object(nmService, active), c.Object(nmService, dev)
	deadline := time.Now().Add(45 * time.Second)
wait:
	for time.Now().Before(deadline) {
		var state uint32
		if err := prop(a, nmActive+".State", &state); err != nil {
			break // the activation object is gone: it failed
		}
		switch state {
		case 2: // activated
			return nil
		case 3, 4: // deactivating, deactivated
			break wait
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	var reason struct {
		State, Reason uint32
	}
	prop(o, nmDevice+".StateReason", &reason)
	if failed != nil {
		failed()
	}
	switch reason.Reason {
	case 7, 8, 9: // no secrets, supplicant disconnected, supplicant config failed
		return ErrWrongPassword
	}
	return errors.New("system: the network could not be joined")
}

func (s *linuxSystem) Forget(ctx context.Context, ssid string) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	_, c, err := s.nm()
	if err != nil {
		return err
	}
	for _, p := range s.knownSSIDs(ctx)[ssid] {
		if err := c.Object(nmService, p).CallWithContext(ctx, nmConnection+".Delete", 0).Err; err != nil {
			return fmt.Errorf("system: forgetting %q: %w", ssid, err)
		}
	}
	return nil
}

// configureTimeout is how long a configuration lasts unless kept.
const configureTimeout = 60 * time.Second

func (s *linuxSystem) Configure(ctx context.Context, name string, cfg IPConfig) (Change, error) {
	if !s.plat.Root {
		return nil, ErrReadOnly
	}
	if err := cfg.Check(); err != nil {
		return nil, err
	}
	d, err := s.device(ctx, name)
	if err != nil {
		return nil, err
	}
	nm, c, _ := s.nm()
	conn, err := s.connectionOf(ctx, d)
	if err != nil {
		return nil, err
	}
	// The checkpoint undoes everything below after the timeout, unless it
	// is destroyed; flag 1 destroys a checkpoint an earlier change left.
	var cp dbus.ObjectPath
	if err := nm.CallWithContext(ctx, nmIface+".CheckpointCreate", 0, []dbus.ObjectPath{d.path}, uint32(configureTimeout/time.Second), uint32(1)).Store(&cp); err != nil {
		return nil, fmt.Errorf("system: NetworkManager checkpoint: %w", err)
	}
	ch := &nmChange{s: s, cp: cp}
	co := c.Object(nmService, conn)
	var st settings
	if err := co.CallWithContext(ctx, nmConnection+".GetSettings", 0).Store(&st); err != nil {
		ch.Revert(ctx)
		return nil, fmt.Errorf("system: NetworkManager: %w", err)
	}
	// GetSettings leaves the secrets out, and Update without them would
	// forget the password of a wireless network.
	if _, ok := st["802-11-wireless-security"]; ok {
		var sec settings
		if co.CallWithContext(ctx, nmConnection+".GetSecrets", 0, "802-11-wireless-security").Store(&sec) == nil {
			for k, v := range sec["802-11-wireless-security"] {
				st["802-11-wireless-security"][k] = v
			}
		}
	}
	applyConfig(st, cfg)
	if err := co.CallWithContext(ctx, nmConnection+".Update", 0, st).Err; err != nil {
		ch.Revert(ctx)
		return nil, fmt.Errorf("system: NetworkManager: %w", err)
	}
	var active dbus.ObjectPath
	if err := nm.CallWithContext(ctx, nmIface+".ActivateConnection", 0, conn, d.path, dbus.ObjectPath("/")).Store(&active); err != nil {
		ch.Revert(ctx)
		return nil, fmt.Errorf("system: NetworkManager: %w", err)
	}
	return ch, nil
}

// connectionOf returns the connection a device is connected with, or the
// saved one bound to its interface, or a new wired connection for it.
func (s *linuxSystem) connectionOf(ctx context.Context, d device) (dbus.ObjectPath, error) {
	_, c, _ := s.nm()
	o := c.Object(nmService, d.path)
	var active dbus.ObjectPath
	if prop(o, nmDevice+".ActiveConnection", &active) == nil && active != "/" {
		var conn dbus.ObjectPath
		if prop(c.Object(nmService, active), nmActive+".Connection", &conn) == nil {
			return conn, nil
		}
	}
	var avail []dbus.ObjectPath
	if prop(o, nmDevice+".AvailableConnections", &avail) == nil && len(avail) > 0 {
		return avail[0], nil
	}
	if d.iface.Kind != Ethernet {
		return "", fmt.Errorf("system: %s has no connection to configure; join a network first", d.iface.Device)
	}
	st := settings{
		"connection": {
			"id":             dbus.MakeVariant("Wired " + d.iface.Device),
			"type":           dbus.MakeVariant("802-3-ethernet"),
			"interface-name": dbus.MakeVariant(d.iface.Device),
		},
		"802-3-ethernet": {},
		"ipv4":           {"method": dbus.MakeVariant("auto")},
	}
	var conn dbus.ObjectPath
	if err := c.Object(nmService, nmSettings).CallWithContext(ctx, nmIface+".Settings.AddConnection", 0, st).Store(&conn); err != nil {
		return "", fmt.Errorf("system: NetworkManager: %w", err)
	}
	return conn, nil
}

type nmChange struct {
	s  *linuxSystem
	cp dbus.ObjectPath
}

func (c *nmChange) Timeout() time.Duration { return configureTimeout }

func (c *nmChange) Keep(ctx context.Context) error {
	nm, _, err := c.s.nm()
	if err != nil {
		return err
	}
	return nm.CallWithContext(ctx, nmIface+".CheckpointDestroy", 0, c.cp).Err
}

func (c *nmChange) Revert(ctx context.Context) error {
	nm, _, err := c.s.nm()
	if err != nil {
		return err
	}
	var result map[string]uint32
	return nm.CallWithContext(ctx, nmIface+".CheckpointRollback", 0, c.cp).Store(&result)
}
