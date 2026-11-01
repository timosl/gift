package kiosk

import (
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestThePINLocksOutAsIOSDoesAndSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kiosk.json")
	s := openStore(path)
	if err := s.setPIN("1111"); err == nil {
		t.Fatal("a PIN of one digit repeated is accepted")
	}
	if err := s.setPIN("2580"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		if v := s.tryPIN("0000", "", now); v.ok || v.wait != 0 {
			t.Fatalf("attempt %d: %+v", i, v)
		}
	}
	if v := s.tryPIN("0000", "", now); v.wait != time.Minute {
		t.Fatalf("the fourth wrong attempt waits %v", v.wait)
	}
	// Restarting the program does not reset the count.
	s = openStore(path)
	if v := s.tryPIN("2580", "", now.Add(30*time.Second)); v.ok {
		t.Fatal("the right PIN is taken while locked out")
	}
	if v := s.tryPIN("2580", "", now.Add(61*time.Second)); !v.ok {
		t.Fatalf("the right PIN after the lockout: %+v", v)
	}
	if s.digits("") != 4 || s.digits("123456") != 6 {
		t.Error("digits")
	}
}

func TestAConfigurationIsReadFromItsFields(t *testing.T) {
	c, err := parseConfig(true, "10.6.6.247", "255.255.255.0", "10.6.6.1", "10.6.6.1, 1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Address != netip.MustParsePrefix("10.6.6.247/24") || len(c.DNS) != 2 {
		t.Fatalf("%+v", c)
	}
	if c, err := parseConfig(true, "10.6.6.247", "/22", "", ""); err != nil || c.Address.Bits() != 22 {
		t.Fatalf("a prefix length as the mask: %+v %v", c, err)
	}
	for _, bad := range [][4]string{{"10.6.6", "255.255.255.0", "", ""}, {"10.6.6.2", "255.0.255.0", "", ""},
		{"10.6.6.2", "24", "10.7.0.1", ""}, {"10.6.6.2", "24", "", "dns"}} {
		if _, err := parseConfig(true, bad[0], bad[1], bad[2], bad[3]); err == nil {
			t.Errorf("%q is accepted", bad)
		}
	}
	if c, err := parseConfig(false, "", "", "", ""); err != nil || c.Address.IsValid() {
		t.Fatalf("automatic: %+v %v", c, err)
	}
}

func TestVersionsCompareByTheirNumbers(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"1.10.0", "1.9.3", true}, {"v2", "1.99", true}, {"1.0.0", "1.0.0", false}, {"1.0", "1.0.1", false}} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
