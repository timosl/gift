package kiosk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// AppUpdater finds and fetches new versions of the program. The kiosk
// installs what it fetches in the place of the program and restarts.
type AppUpdater interface {
	// Check returns the newest version there is.
	Check(ctx context.Context) (AppRelease, error)
	// Download fetches a version into a file of the given directory,
	// reporting how far it is from 0 to 1, and returns the path of the file.
	Download(ctx context.Context, r AppRelease, dir string, progress func(float64)) (string, error)
}

// AppRelease is a version of the program.
type AppRelease struct {
	Version string
	// Notes say what is new.
	Notes string
	// URL and SHA256 are where the program is and the checksum it has, for
	// the platform of the running program.
	URL, SHA256 string
}

// Manifest is an AppUpdater that reads a JSON file from a web server:
//
//	{
//	  "version": "1.4.0",
//	  "notes": "Prints faster.",
//	  "files": {
//	    "linux/arm64": {"url": "https://example.com/werkbank-1.4.0-arm64", "sha256": "…"},
//	    "linux/amd64": {"url": "https://example.com/werkbank-1.4.0-amd64", "sha256": "…"}
//	  }
//	}
type Manifest struct {
	// URL is the address of the manifest.
	URL string
	// Client makes the requests; nil is http.DefaultClient.
	Client *http.Client
}

type manifest struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
	Files   map[string]struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func (m Manifest) client() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return http.DefaultClient
}

// Check implements [AppUpdater].
func (m Manifest) Check(ctx context.Context) (AppRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return AppRelease{}, err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return AppRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return AppRelease{}, fmt.Errorf("kiosk: the manifest: %s", resp.Status)
	}
	var mf manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&mf); err != nil {
		return AppRelease{}, fmt.Errorf("kiosk: the manifest: %w", err)
	}
	f, ok := mf.Files[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return AppRelease{}, fmt.Errorf("kiosk: the manifest has no program for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return AppRelease{Version: mf.Version, Notes: mf.Notes, URL: f.URL, SHA256: f.SHA256}, nil
}

// Download implements [AppUpdater]: it checks the checksum and refuses a
// file that does not match.
func (m Manifest) Download(ctx context.Context, r AppRelease, dir string, progress func(float64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kiosk: downloading %s: %s", r.Version, resp.Status)
	}
	f, err := os.CreateTemp(dir, ".update-*")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil && resp.ContentLength > 0 {
				progress(float64(done) / float64(resp.ContentLength))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
	}
	f.Close()
	if r.SHA256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), r.SHA256) {
		os.Remove(f.Name())
		return "", fmt.Errorf("kiosk: the download of %s does not match its checksum", r.Version)
	}
	return f.Name(), nil
}

// newer reports whether version a is newer than b, comparing the dotted
// numbers of both, so that 1.10 is newer than 1.9; a leading v is ignored.
func newer(a, b string) bool {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(strings.TrimLeft(pa[i], "v"))
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}
