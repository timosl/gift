//go:build !(darwin || freebsd || linux)

package ffmpeg

import "fmt"

// bind always fails where purego cannot dlopen; see asset/turbojpeg for why
// Windows is not attempted.
func bind() error { return fmt.Errorf("%w: unsupported platform", ErrUnavailable) }
