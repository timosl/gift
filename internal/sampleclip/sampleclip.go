// Package sampleclip is three seconds of Big Buck Bunny with two audio
// tracks, for the demos, the examples and the tests that need a video; see
// README.md for its licence and how it was cut.
//
// It is embedded, so that a program that shows it finds it wherever it runs:
// the kitchen sink on a Raspberry Pi as much as the gallery in the
// repository. FFmpeg opens files by path, so [Path] writes it out once.
package sampleclip

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed bbb.mp4
var clip []byte

// Path writes the clip into dir, unless it is there already, and returns its
// path.
func Path(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "bbb.mp4")
	if have, err := os.ReadFile(p); err == nil && bytes.Equal(have, clip) {
		return p, nil
	}
	if err := os.WriteFile(p, clip, 0o644); err != nil {
		return "", err
	}
	return p, nil
}
