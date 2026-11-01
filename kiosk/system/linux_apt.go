//go:build linux

package system

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/godbus/dbus/v5"
)

// aptEnv is the environment of every apt-get run: no questions, and the
// configuration files the administrator changed kept.
var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a", "APT_LISTCHANGES_FRONTEND=none"}

// upgradeVerb is what installs the updates. Raspberry Pi OS changes the
// dependencies of its packages more often than Debian and asks for
// full-upgrade; elsewhere upgrade with new packages is the conservative
// choice, which never removes one.
func (s *linuxSystem) upgradeVerb() []string {
	if s.plat.Kind == RaspberryPiOS {
		return []string{"full-upgrade"}
	}
	return []string{"upgrade", "--with-new-pkgs"}
}

func (s *linuxSystem) CheckUpdates(ctx context.Context) ([]Package, error) {
	if !s.plat.Root {
		return nil, ErrReadOnly
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		return nil, ErrUnsupported
	}
	if _, err := run(ctx, aptEnv, "apt-get", "-q", "-o", "DPkg::Lock::Timeout=120", "update"); err != nil {
		return nil, err
	}
	args := append([]string{"-s", "-q", "-o", "Debug::NoLocking=1"}, s.upgradeVerb()...)
	out, err := run(ctx, aptEnv, "apt-get", args...)
	if err != nil {
		return nil, err
	}
	return parseAptSimulation(out), nil
}

func (s *linuxSystem) InstallUpdates(ctx context.Context, progress func(Progress)) error {
	if !s.plat.Root {
		return ErrReadOnly
	}
	release := s.inhibit("Installing updates")
	defer release()
	args := append([]string{"-y", "-q", "-o", "DPkg::Lock::Timeout=600", "-o", "APT::Status-Fd=3",
		"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}, s.upgradeVerb()...)
	return runWithStatus(ctx, progress, "apt-get", args...)
}

// runWithStatus runs apt-get with its status lines on file descriptor 3 and
// hands them to progress.
func runWithStatus(ctx context.Context, progress func(Progress), name string, args ...string) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(append(os.Environ(), "LC_ALL=C", "LANG=C"), aptEnv...)
	cmd.ExtraFiles = []*os.File{w}
	var errb strings.Builder
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return err
	}
	w.Close()
	var failed []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		st, ok := parseAptStatus(sc.Text())
		if !ok {
			continue
		}
		if st.err {
			failed = append(failed, st.desc)
			continue
		}
		if progress != nil {
			progress(Progress{Fraction: st.fraction(), Status: st.desc})
		}
	}
	r.Close()
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(failed) > 0 {
			msg = strings.Join(failed, "; ")
		}
		if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
			msg = msg[i+1:]
		}
		return fmt.Errorf("system: %s: %s", name, msg)
	}
	if progress != nil {
		progress(Progress{Fraction: 1})
	}
	return nil
}

// inhibit keeps the system from rebooting or powering off until release is
// called, root included: since systemd 257 a block inhibitor holds against
// everybody, and a reboot in the middle of dpkg breaks a system.
func (s *linuxSystem) inhibit(why string) (release func()) {
	c, err := s.conn()
	if err != nil {
		return func() {}
	}
	var fd dbus.UnixFD
	if c.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0, "shutdown:sleep", "gift kiosk", why, "block").Store(&fd) != nil {
		return func() {}
	}
	f := os.NewFile(uintptr(fd), "inhibitor")
	return func() { f.Close() }
}

var newRelease = regexp.MustCompile(`New release '([^']+)' available`)

func (s *linuxSystem) Release(ctx context.Context) (Release, error) {
	switch s.plat.Kind {
	case RaspberryPiOS:
		return Release{Note: "Raspberry Pi OS is upgraded to a new release by installing a new image; Raspberry Pi does not support upgrading in place."}, nil
	case Ubuntu:
	default:
		return Release{Note: "Upgrading the release in place is supported on Ubuntu only."}, nil
	}
	if _, err := exec.LookPath("do-release-upgrade"); err != nil {
		return Release{Note: "do-release-upgrade is not installed (package ubuntu-release-upgrader-core)."}, nil
	}
	// -c reports by its exit status, and names the release on its output.
	out, err := run(ctx, nil, "do-release-upgrade", "-c")
	if m := newRelease.FindStringSubmatch(out); m != nil {
		return Release{Available: true, Name: m[1], Supported: s.plat.Root}, nil
	}
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) && !strings.Contains(err.Error(), "exit status") {
		return Release{}, err
	}
	return Release{}, nil
}

func (s *linuxSystem) UpgradeRelease(ctx context.Context, progress func(Progress)) error {
	if s.plat.Kind != Ubuntu {
		return ErrUnsupported
	}
	if !s.plat.Root {
		return ErrReadOnly
	}
	release := s.inhibit("Upgrading the release")
	defer release()
	if progress != nil {
		progress(Progress{Fraction: -1, Status: "Upgrading the release"})
	}
	_, err := run(ctx, aptEnv, "do-release-upgrade", "-f", "DistUpgradeViewNonInteractive")
	return err
}
