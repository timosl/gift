package maps_test

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/worldiety/gift/geo"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui/maps"
)

// tileServer serves a PNG for every tile and records the requests.
type tileServer struct {
	mu     sync.Mutex
	paths  []string
	agents []string
}

func (s *tileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.paths = append(s.paths, r.URL.Path)
	s.agents = append(s.agents, r.Header.Get("User-Agent"))
	s.mu.Unlock()
	if strings.HasPrefix(r.URL.Path, "/0/") {
		http.NotFound(w, r)
		return
	}
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 256, 256)))
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("ETag", `"t1"`)
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(b.Bytes())
}

func (s *tileServer) requests() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...), append([]string(nil), s.agents...)
}

func TestATileServiceIsAProviderLikeAnyOther(t *testing.T) {
	ts := &tileServer{}
	srv := httptest.NewServer(ts)
	defer srv.Close()
	p, err := maps.XYZ(srv.URL+"/{z}/{x}/{y}.png", maps.XYZOptions{
		Attribution: "© Test", Header: http.Header{"User-Agent": {"MapTest/1.0 (+https://example.org)"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := newScene(t, maps.Config{Tiles: p, Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}},
		geom.Sz(512, 384), nil)
	s.load(t)
	paths, agents := ts.requests()
	st := s.m.Stats()
	if len(paths) != st.Tiles {
		t.Errorf("%d requests for %d visible tiles: the service forbids prefetching and was asked for more", len(paths), st.Tiles)
	}
	for _, a := range agents {
		if a != "MapTest/1.0 (+https://example.org)" {
			t.Fatalf("a request went out as %q", a)
		}
	}
	s.AssertExists(gifttest.ByText("© Test"))
}

func TestATileServiceThatPermitsItIsPrefetched(t *testing.T) {
	ts := &tileServer{}
	srv := httptest.NewServer(ts)
	defer srv.Close()
	p, err := maps.XYZ(srv.URL+"/{z}/{x}/{y}.png", maps.XYZOptions{Attribution: "© Test", Prefetch: true})
	if err != nil {
		t.Fatal(err)
	}
	s := newScene(t, maps.Config{Tiles: p, Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}},
		geom.Sz(512, 384), nil)
	s.load(t)
	paths, agents := ts.requests()
	if len(paths) <= s.m.Stats().Tiles {
		t.Errorf("%d requests for %d visible tiles: nothing around the view was prefetched", len(paths), s.m.Stats().Tiles)
	}
	for _, a := range agents {
		if !strings.HasPrefix(a, "gift/") {
			t.Fatalf("a request went out as %q, not gift's default", a)
		}
	}
}

func TestATileServiceNeedsItsAttribution(t *testing.T) {
	if _, err := maps.XYZ("https://example.org/{z}/{x}/{y}.png", maps.XYZOptions{}); err == nil {
		t.Error("a service without attribution was accepted")
	}
	if _, err := maps.XYZ("https://example.org/tiles.png", maps.XYZOptions{Attribution: "x"}); err == nil {
		t.Error("a template without {z}/{x}/{y} was accepted")
	}
}

// TestASecondVisitLoadsNothingAgain: with the pipeline's disk cache the
// tiles of a service are kept by its caching headers, and a map opened again
// on the same place asks the service for nothing, which the usage policy of
// the OpenStreetMap Foundation requires.
func TestASecondVisitLoadsNothingAgain(t *testing.T) {
	ts := &tileServer{}
	srv := httptest.NewServer(ts)
	defer srv.Close()
	cache := t.TempDir()
	visit := func() int {
		p, err := maps.XYZ(srv.URL+"/{z}/{x}/{y}.png", maps.XYZOptions{Attribution: "© Test"})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := ts.requests()
		s := newSceneWithCache(t, maps.Config{Tiles: p, Camera: maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 10}},
			geom.Sz(512, 384), cache)
		s.load(t)
		after, _ := ts.requests()
		return len(after) - len(before)
	}
	if n := visit(); n == 0 {
		t.Fatal("the first visit asked for no tiles")
	}
	if n := visit(); n != 0 {
		t.Fatalf("the second visit asked the service for %d tiles again", n)
	}
}
