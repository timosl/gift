package kiosk

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// store is what the kiosk keeps across starts: the PIN set in the settings
// and the failed attempts to enter it, which a restart must not reset, or
// pulling the plug would be a way around the lockout.
type store struct {
	path string

	mu   sync.Mutex
	data storeData
}

type storeData struct {
	PIN      *pinHash  `json:"pin,omitempty"`
	Failed   int       `json:"failed,omitempty"`
	Until    time.Time `json:"until,omitzero"`
	Protects Area      `json:"protects,omitempty"`
	// Appearance is "light" or "dark".
	Appearance string `json:"appearance,omitempty"`
}

// pinHash is a PIN as PBKDF2 with SHA-256 keeps it.
type pinHash struct {
	Salt   []byte `json:"salt"`
	Hash   []byte `json:"hash"`
	Rounds int    `json:"rounds"`
	Digits int    `json:"digits"`
}

// pinRounds is the cost of a PIN hash: a few milliseconds on a Raspberry
// Pi, which is nothing for one attempt and a lot for ten thousand.
const pinRounds = 200_000

func openStore(path string) *store {
	s := &store{path: path}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &s.data)
	}
	return s
}

// save writes the store through a temporary file.
func (s *store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// checkPIN reports whether pin is a PIN: 4 or 6 digits, not all the same.
func checkPIN(pin string) error {
	if len(pin) != 4 && len(pin) != 6 {
		return errors.New("A passcode has 4 or 6 digits.")
	}
	if strings.Trim(pin, "0123456789") != "" {
		return errors.New("A passcode has only digits.")
	}
	if strings.Count(pin, pin[:1]) == len(pin) {
		return errors.New("A passcode of one digit repeated is too easy to guess.")
	}
	return nil
}

func hashPIN(pin string, salt []byte, rounds int) []byte {
	h, _ := pbkdf2.Key(sha256.New, pin, salt, rounds, 32)
	return h
}

// setPIN sets the PIN of the settings, or removes it for an empty pin.
func (s *store) setPIN(pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pin == "" {
		s.data.PIN = nil
		return s.save()
	}
	if err := checkPIN(pin); err != nil {
		return err
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	s.data.PIN = &pinHash{Salt: salt, Hash: hashPIN(pin, salt, pinRounds), Rounds: pinRounds, Digits: len(pin)}
	s.data.Failed, s.data.Until = 0, time.Time{}
	return s.save()
}

// lockout is how long the PIN cannot be entered after a number of failed
// attempts, after the table iOS has for its passcode.
func lockout(failed int) time.Duration {
	switch {
	case failed < 4:
		return 0
	case failed == 4:
		return time.Minute
	case failed == 5:
		return 5 * time.Minute
	case failed == 6:
		return 15 * time.Minute
	case failed == 7:
		return time.Hour
	case failed == 8:
		return 3 * time.Hour
	}
	return 8 * time.Hour
}

// verdict is what entering a PIN came to.
type verdict struct {
	ok bool
	// wait is how long until the next attempt, after too many wrong ones.
	wait   time.Duration
	failed int
}

// tryPIN checks pin against the PIN of the program, if it has one, or the
// one set in the settings, and counts a failure.
func (s *store) tryPIN(pin, fixed string, now time.Time) verdict {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Before(s.data.Until) {
		return verdict{wait: s.data.Until.Sub(now), failed: s.data.Failed}
	}
	ok := false
	switch {
	case fixed != "":
		ok = subtle.ConstantTimeCompare([]byte(pin), []byte(fixed)) == 1
	case s.data.PIN != nil:
		p := s.data.PIN
		ok = subtle.ConstantTimeCompare(hashPIN(pin, p.Salt, p.Rounds), p.Hash) == 1
	default:
		ok = true
	}
	if ok {
		s.data.Failed, s.data.Until = 0, time.Time{}
		s.save()
		return verdict{ok: true}
	}
	s.data.Failed++
	wait := lockout(s.data.Failed)
	s.data.Until = now.Add(wait)
	s.save()
	return verdict{wait: wait, failed: s.data.Failed}
}

// digits is the length of the PIN in force, or 0 without one.
func (s *store) digits(fixed string) int {
	if fixed != "" {
		return len(fixed)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.PIN != nil {
		return s.data.PIN.Digits
	}
	return 0
}

// protects is what the PIN set in the settings protects.
func (s *store) protects() Area {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Protects == 0 {
		return AreaSettings
	}
	return s.data.Protects
}

func (s *store) setProtects(a Area) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Protects = a
	return s.save()
}

// waiting is how long until the next attempt.
func (s *store) waiting(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Before(s.data.Until) {
		return s.data.Until.Sub(now)
	}
	return 0
}

// appearance is the appearance the settings chose, "light" unless "dark".
func (s *store) appearance() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Appearance == "dark" {
		return "dark"
	}
	return "light"
}

func (s *store) setAppearance(a string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Appearance = a
	return s.save()
}
