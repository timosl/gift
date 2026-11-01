package system

import (
	"errors"
	"fmt"
	"math/bits"
	"net/netip"
	"strings"
)

// CheckHostname reports whether name may be the name of a device: one label
// of letters, digits and hyphens, not starting or ending with a hyphen, at
// most 63 characters, as RFC 1123 has it.
func CheckHostname(name string) error {
	if name == "" || len(name) > 63 {
		return fmt.Errorf("system: a host name has 1 to 63 characters, not %d", len(name))
	}
	if name[0] == '-' || name[len(name)-1] == '-' {
		return errors.New("system: a host name does not start or end with a hyphen")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return fmt.Errorf("system: a host name has only letters, digits and hyphens, not %q", r)
		}
	}
	return nil
}

// CheckName reports whether name may name a kiosk: it becomes the name of
// the program in /usr/local/bin and of its systemd unit, so it is lower case
// letters, digits and hyphens, starting with a letter.
func CheckName(name string) error {
	if name == "" || len(name) > 32 || !(name[0] >= 'a' && name[0] <= 'z') {
		return fmt.Errorf("system: the name of a kiosk starts with a lower case letter and has at most 32 characters, not %q", name)
	}
	if strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return fmt.Errorf("system: the name of a kiosk has only lower case letters, digits and hyphens, not %q", name)
	}
	return nil
}

// Check reports whether c is a complete configuration: a manual one needs
// an IPv4 address with its prefix, and its router, if any, must be in the
// same subnet.
func (c IPConfig) Check() error {
	if c.Method == Automatic {
		for _, d := range c.DNS {
			if !d.Is4() {
				return fmt.Errorf("system: %v is not an IPv4 address", d)
			}
		}
		return nil
	}
	if !c.Address.IsValid() || !c.Address.Addr().Is4() {
		return errors.New("system: a manual configuration needs an IPv4 address")
	}
	if b := c.Address.Bits(); b < 1 || b > 32 {
		return fmt.Errorf("system: %d is not an IPv4 prefix length", b)
	}
	if c.Router.IsValid() {
		if !c.Router.Is4() {
			return fmt.Errorf("system: the router %v is not an IPv4 address", c.Router)
		}
		if !c.Address.Masked().Contains(c.Router) {
			return fmt.Errorf("system: the router %v is not in the subnet %v", c.Router, c.Address.Masked())
		}
	}
	for _, d := range c.DNS {
		if !d.Is4() {
			return fmt.Errorf("system: %v is not an IPv4 address", d)
		}
	}
	return nil
}

// MaskBits returns the prefix length of a subnet mask such as
// "255.255.255.0", as the settings of a building automation network give
// it, and whether the mask is one: ones followed by zeros.
func MaskBits(mask string) (int, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(mask))
	if err != nil || !a.Is4() {
		return 0, false
	}
	b := a.As4()
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	n := bits.LeadingZeros32(^v)
	if v<<n != 0 {
		return 0, false
	}
	return n, true
}

// Mask returns the subnet mask of a prefix length, such as "255.255.255.0"
// for 24.
func Mask(bits int) string {
	bits = max(0, min(bits, 32))
	var v uint32
	if bits > 0 {
		v = ^uint32(0) << (32 - bits)
	}
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}).String()
}
