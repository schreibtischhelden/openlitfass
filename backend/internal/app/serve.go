package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"openlitfass/backend/internal/instance"
)

// Server runs one instance self-contained: it fetches on an interval,
// rebuilds the site and serves it over HTTP - "one instance per container",
// no CI or separate web server needed.
//
// Every build goes into a fresh directory under PublicDir and is switched
// live only when complete, so visitors never see a half-built site and a
// failed build leaves the previous version online.
type Server struct {
	Instance  instance.Instance
	WebDir    string        // core frontend (image: /opt/openlitfass/web)
	PublicDir string        // where builds go (default <instance>/.openlitfass-serve)
	Interval  time.Duration // fetch interval; 0 = never fetch, only serve
	// MetricsAddr, if set (e.g. ":9090"), serves /metrics on a separate
	// listener instead of next to the site - so metrics don't have to be
	// public. Empty: /metrics on the main address.
	MetricsAddr string

	current atomic.Pointer[string] // live build directory

	mu      sync.Mutex
	status  Status
	metrics metricsState
}

// Status is what /healthz reports.
type Status struct {
	Instance      string     `json:"instance"`
	Serving       bool       `json:"serving"`
	LastFetch     *time.Time `json:"lastFetch,omitempty"`
	LastFetchOK   bool       `json:"lastFetchOk"`
	LastError     string     `json:"lastError,omitempty"`
	Events        int        `json:"events"`
	FailedSources []string   `json:"failedSources,omitempty"`
	LastBuild     *time.Time `json:"lastBuild,omitempty"`
	NextFetch     *time.Time `json:"nextFetch,omitempty"`
}

func (s *Server) setStatus(f func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.status)
}

// Status returns a copy of the current status.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.Instance = s.Instance.Config.Name
	st.Serving = s.current.Load() != nil
	return st
}

// Rebuild builds the site into a new directory and switches it live.
func (s *Server) Rebuild() error {
	if err := os.MkdirAll(s.PublicDir, 0o755); err != nil {
		return err
	}
	dir := filepath.Join(s.PublicDir, "build-"+time.Now().UTC().Format("20060102T150405.000000000"))
	if err := BuildSite(s.Instance, s.WebDir, dir); err != nil {
		os.RemoveAll(dir)
		s.mu.Lock()
		s.metrics.init()
		s.metrics.builds["failed"]++
		s.mu.Unlock()
		return err
	}
	old := s.current.Swap(&dir)
	ev := s.loadEventStats()
	s.mu.Lock()
	now := time.Now()
	s.status.LastBuild = &now
	if ev.loaded {
		s.status.Events = ev.total
	}
	s.metrics.init()
	s.metrics.builds["ok"]++
	s.metrics.events = ev
	s.mu.Unlock()
	if old != nil && *old != dir {
		// Give in-flight requests a moment before removing the old build.
		go func(p string) {
			time.Sleep(30 * time.Second)
			removeBuild(p)
		}(*old)
	}
	s.cleanupStale(dir)
	return nil
}

// removeBuild deletes a build directory, but only one BuildSite created.
func removeBuild(dir string) {
	if _, err := os.Stat(filepath.Join(dir, BuildMarker)); err == nil {
		os.RemoveAll(dir)
	}
}

// cleanupStale removes leftover builds from earlier processes (crash,
// restart), keeping the live one.
func (s *Server) cleanupStale(live string) {
	entries, err := os.ReadDir(s.PublicDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(s.PublicDir, e.Name())
		if e.IsDir() && strings.HasPrefix(e.Name(), "build-") && p != live {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > time.Minute {
				removeBuild(p)
			}
		}
	}
}

// FetchAndRebuild runs one fetch and, if anything was written, a rebuild.
func (s *Server) FetchAndRebuild(ctx context.Context) {
	res, err := Fetch(ctx, s.Instance)
	s.setStatus(func(st *Status) {
		now := time.Now()
		st.LastFetch = &now
		st.LastFetchOK = err == nil && res.Written
		st.FailedSources = res.Failed
		st.LastError = ""
		if err != nil {
			st.LastError = err.Error()
		}
		if res.Written {
			st.Events = res.Events
		}
	})
	s.mu.Lock()
	s.metrics.init()
	s.metrics.fetchRuns[fetchResultLabel(res, err)]++
	r := res
	s.metrics.lastFetch = &r
	s.mu.Unlock()
	if err != nil {
		log.Printf("fetch: %v", err)
		return
	}
	if !res.Written && s.current.Load() != nil {
		return // nothing new; keep serving the current build
	}
	if err := s.Rebuild(); err != nil {
		log.Printf("rebuild: %v (keeping the previous version online)", err)
		s.setStatus(func(st *Status) { st.LastError = "build: " + err.Error() })
	}
}

// Run serves on addr until ctx is cancelled. It builds from existing data
// first (so the site is up within seconds), then fetches right away and
// every Interval.
func (s *Server) Run(ctx context.Context, addr string) error {
	if s.PublicDir == "" {
		// Not "public": that is build-site's default output, which wipes
		// its directory on every run.
		s.PublicDir = s.Instance.Path(".openlitfass-serve")
	}
	if err := s.Rebuild(); err != nil {
		log.Printf("initial build from existing data failed: %v", err)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 2)
	go func() {
		log.Printf("serving %q on %s", s.Instance.Config.Name, addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()
	var metricsSrv *http.Server
	if s.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", s.metricsHandler())
		metricsSrv = &http.Server{Addr: s.MetricsAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			log.Printf("metrics on %s/metrics", s.MetricsAddr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- err
			}
		}()
	}

	if s.Interval > 0 {
		go func() {
			for {
				s.FetchAndRebuild(ctx)
				next := time.Now().Add(s.Interval)
				s.setStatus(func(st *Status) { st.NextFetch = &next })
				select {
				case <-ctx.Done():
					return
				case <-time.After(s.Interval):
				}
			}
		}()
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Printf("shutting down")
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdown)
	}
	return srv.Shutdown(shutdown)
}

// Handler serves the live build plus /healthz.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		st := s.Status()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if !st.Serving {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(st)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		dir := s.current.Load()
		if dir == nil {
			http.Error(w, "Die Seite wird gerade gebaut. Bitte gleich noch einmal versuchen.", http.StatusServiceUnavailable)
			return
		}
		setCacheHeaders(w, r.URL.Path)
		http.FileServer(noListing{http.Dir(*dir)}).ServeHTTP(w, r)
	})
	if s.MetricsAddr == "" {
		mux.Handle("/metrics", s.metricsHandler())
	}
	return s.countRequests(mux)
}

// setCacheHeaders: data, the page itself, the manifest and the service
// worker must always be revalidated (they change with every build); other
// assets may be cached briefly. The service worker handles offline use.
func setCacheHeaders(w http.ResponseWriter, p string) {
	switch {
	case p == "/" || strings.HasSuffix(p, ".html") || strings.HasPrefix(p, "/data/") ||
		p == "/sw.js" || p == "/manifest.webmanifest":
		w.Header().Set("Cache-Control", "no-cache")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// noListing hides directory listings (e.g. /data/) and the build marker.
type noListing struct{ fs http.FileSystem }

func (n noListing) Open(name string) (http.File, error) {
	if strings.HasSuffix(name, "/"+BuildMarker) {
		return nil, fs.ErrNotExist
	}
	f, err := n.fs.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		idx, err := n.fs.Open(strings.TrimSuffix(name, "/") + "/index.html")
		if err != nil {
			f.Close()
			return nil, fs.ErrNotExist
		}
		idx.Close()
	}
	return f, nil
}
