package weather

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/worldiety/gift/geo"
)

// SkyConfig configures a [Sky].
type SkyConfig struct {
	// Region is the area to keep the weather of. A map sets it from what it
	// shows; see [Sky.SetRegion].
	Region geo.Rect
	// Variables are the quantities to fetch; nil fetches all the provider
	// has.
	Variables []Variable
	// Horizon is how far ahead to fetch; zero selects three hours.
	Horizon time.Duration
	// Refresh is how often to look for a newer run; zero selects the
	// provider's cycle.
	Refresh time.Duration
	// Now is the clock; nil is time.Now.
	Now func() time.Time
}

// Sky keeps the latest run of a provider over a region, fetched on a
// goroutine of its own and published atomically, so that a map reads it in
// every frame without waiting for anything.
type Sky struct {
	p   Provider
	cfg SkyConfig

	run     atomic.Pointer[Run]
	version atomic.Uint64
	err     atomic.Pointer[error]

	mu      sync.Mutex
	region  geo.Rect
	wake    chan struct{}
	cancel  context.CancelFunc
	running bool
}

// NewSky returns a sky over p. Nothing is fetched before [Sky.Start] or
// [Sky.Fetch].
func NewSky(p Provider, cfg SkyConfig) *Sky {
	if cfg.Horizon <= 0 {
		cfg.Horizon = 3 * time.Hour
	}
	if cfg.Refresh <= 0 {
		cfg.Refresh = p.Info().Cycle
		if cfg.Refresh <= 0 {
			cfg.Refresh = time.Hour
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Sky{p: p, cfg: cfg, region: cfg.Region, wake: make(chan struct{}, 1)}
}

// Info returns what the provider says about itself.
func (s *Sky) Info() ProviderInfo { return s.p.Info() }

// Run returns the latest run, or nil before the first has arrived.
func (s *Sky) Run() *Run { return s.run.Load() }

// Version counts the runs published, for a map to notice a new one.
func (s *Sky) Version() uint64 { return s.version.Load() }

// Err returns the error of the last fetch, or nil.
func (s *Sky) Err() error {
	if e := s.err.Load(); e != nil {
		return *e
	}
	return nil
}

// Now returns the time of the sky's clock.
func (s *Sky) Now() time.Time { return s.cfg.Now() }

// Region returns the area the sky keeps.
func (s *Sky) Region() geo.Rect {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.region
}

// SetRegion changes the area, and fetches again when it is not inside the
// one fetched: a map panned far away gets the weather there. A provider that
// caches its files reads the rest from its cache.
func (s *Sky) SetRegion(r geo.Rect) {
	s.mu.Lock()
	old := s.region
	s.region = r
	s.mu.Unlock()
	inside := r.Min.Lat >= old.Min.Lat && r.Max.Lat <= old.Max.Lat && r.Min.Lon >= old.Min.Lon && r.Max.Lon <= old.Max.Lon
	if !inside {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// Fetch fetches once, now, and publishes the run.
func (s *Sky) Fetch(ctx context.Context) error {
	region := s.Region()
	now := s.cfg.Now()
	run, err := s.p.Fetch(ctx, Query{Region: region, From: now, To: now.Add(s.cfg.Horizon), Variables: s.cfg.Variables})
	if err != nil {
		s.err.Store(&err)
		return err
	}
	s.err.Store(nil)
	s.run.Store(run)
	s.version.Add(1)
	return nil
}

// Start fetches in the background: now, then every refresh, and when the
// region moves away. Stop ends it.
func (s *Sky) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.running = cancel, true
	s.mu.Unlock()
	go func() {
		t := time.NewTimer(0)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-s.wake:
			}
			if err := s.Fetch(ctx); err != nil && ctx.Err() == nil {
				// Again in a minute rather than at the next run.
				t.Reset(time.Minute)
				continue
			}
			t.Reset(s.cfg.Refresh)
		}
	}()
}

// Stop ends the background fetching.
func (s *Sky) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}
