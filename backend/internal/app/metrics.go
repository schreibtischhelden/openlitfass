package app

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/writer"
)

// Version is the openLitfass version, set at build time:
//
//	go build -ldflags "-X openlitfass/backend/internal/app.Version=…"
var Version = "dev"

// Prometheus metrics in the text exposition format (version 0.0.4),
// written by hand - the format is simple and this keeps the core free of
// the Prometheus client library.

// metricsState is the part of the server state only /metrics needs.
// Guarded by Server.mu.
type metricsState struct {
	fetchRuns map[string]uint64 // result -> count: ok, partial, failed, error, nothing
	builds    map[string]uint64 // result -> count: ok, failed
	requests  map[string]uint64 // status class -> count: 2xx, 3xx, 4xx, 5xx
	lastFetch *FetchResult
	events    eventStats
}

type eventStats struct {
	loaded     bool
	total      int
	cancelled  int
	merged     int // shown events reported by more than one source
	bySource   map[string]int
	byCategory map[string]int
}

func (m *metricsState) init() {
	if m.fetchRuns == nil {
		m.fetchRuns = map[string]uint64{}
		m.builds = map[string]uint64{}
		m.requests = map[string]uint64{}
	}
}

// fetchResultLabel classifies a run for openlitfass_fetch_runs_total.
func fetchResultLabel(res FetchResult, err error) string {
	switch {
	case err != nil:
		return "error"
	case !res.Written && len(res.Failed) > 0:
		return "failed"
	case !res.Written:
		return "nothing"
	case len(res.Failed) > 0:
		return "partial"
	default:
		return "ok"
	}
}

// loadEventStats reads the instance's current data/events.json, so event
// gauges are right even when serve doesn't fetch itself (OL_FETCH_INTERVAL=0).
func (s *Server) loadEventStats() eventStats {
	evs, err := writer.LoadPrevious(s.Instance.EventsOutput())
	st := eventStats{loaded: err == nil, bySource: map[string]int{}, byCategory: map[string]int{}}
	for _, e := range evs {
		st.total++
		if e.Cancelled {
			st.cancelled++
		}
		if len(e.Sources) > 1 {
			st.merged++
		}
		st.bySource[e.SourceName]++
		st.byCategory[e.Category]++
	}
	return st
}

type metricWriter struct {
	w    io.Writer
	seen map[string]bool
}

func (m *metricWriter) head(name, typ, help string) {
	if m.seen[name] {
		return
	}
	m.seen[name] = true
	fmt.Fprintf(m.w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func (m *metricWriter) val(name string, labels []string, v float64) {
	fmt.Fprintf(m.w, "%s%s %s\n", name, labelString(labels), formatFloat(v))
}

func labelString(kv []string) string {
	if len(kv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, kv[i], escapeLabel(kv[i+1])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapeLabel(s string) string { return labelEscaper.Replace(s) }

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func ts(t *time.Time) float64 {
	if t == nil {
		return 0
	}
	return float64(t.UnixMilli()) / 1000
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// WriteMetrics writes all metrics in the Prometheus text format.
func (s *Server) WriteMetrics(w io.Writer) {
	s.mu.Lock()
	st := s.status
	ms := s.metrics
	ms.init()
	// Copy maps under the lock.
	runs, builds, reqs := map[string]uint64{}, map[string]uint64{}, map[string]uint64{}
	for k, v := range ms.fetchRuns {
		runs[k] = v
	}
	for k, v := range ms.builds {
		builds[k] = v
	}
	for k, v := range ms.requests {
		reqs[k] = v
	}
	last := ms.lastFetch
	ev := ms.events
	s.mu.Unlock()

	inst := s.Instance.Config.Name
	m := &metricWriter{w: w, seen: map[string]bool{}}

	m.head("openlitfass_info", "gauge", "Version and instance of this openLitfass server (always 1).")
	m.val("openlitfass_info", []string{"version", Version, "instance", inst}, 1)

	m.head("openlitfass_up", "gauge", "1 if the site is being served.")
	up := 0.0
	if s.current.Load() != nil {
		up = 1
	}
	m.val("openlitfass_up", nil, up)

	m.head("openlitfass_fetch_interval_seconds", "gauge", "Configured fetch interval (0 = serve only).")
	m.val("openlitfass_fetch_interval_seconds", nil, s.Interval.Seconds())

	m.head("openlitfass_last_fetch_timestamp_seconds", "gauge", "Unix time of the last fetch run (0 = none yet).")
	m.val("openlitfass_last_fetch_timestamp_seconds", nil, ts(st.LastFetch))
	m.head("openlitfass_next_fetch_timestamp_seconds", "gauge", "Unix time of the next scheduled fetch run (0 = none).")
	m.val("openlitfass_next_fetch_timestamp_seconds", nil, ts(st.NextFetch))
	m.head("openlitfass_last_build_timestamp_seconds", "gauge", "Unix time of the last successful site build.")
	m.val("openlitfass_last_build_timestamp_seconds", nil, ts(st.LastBuild))

	m.head("openlitfass_fetch_runs_total", "counter", "Fetch runs since start, by result (ok, partial = some sources failed, failed, nothing = no sources, error).")
	for _, r := range []string{"ok", "partial", "failed", "nothing", "error"} {
		m.val("openlitfass_fetch_runs_total", []string{"result", r}, float64(runs[r]))
	}
	m.head("openlitfass_builds_total", "counter", "Site builds since start, by result.")
	for _, r := range []string{"ok", "failed"} {
		m.val("openlitfass_builds_total", []string{"result", r}, float64(builds[r]))
	}
	m.head("openlitfass_http_requests_total", "counter", "HTTP requests served, by status class.")
	for _, c := range []string{"2xx", "3xx", "4xx", "5xx"} {
		m.val("openlitfass_http_requests_total", []string{"code", c}, float64(reqs[c]))
	}

	if last != nil {
		m.head("openlitfass_fetch_duration_seconds", "gauge", "Duration of the last fetch run.")
		m.val("openlitfass_fetch_duration_seconds", nil, last.Duration.Seconds())
		for _, r := range last.PerSource {
			m.head("openlitfass_source_fetch_duration_seconds", "gauge", "Duration of the source's last fetch.")
			m.val("openlitfass_source_fetch_duration_seconds", []string{"source", r.Name}, r.Duration.Seconds())
		}
		for _, r := range last.PerSource {
			m.head("openlitfass_source_fetched_events", "gauge", "Events returned by the source in the last fetch run (before merging).")
			m.val("openlitfass_source_fetched_events", []string{"source", r.Name}, float64(r.Events))
		}
		for _, r := range last.PerSource {
			m.head("openlitfass_source_carried_events", "gauge", "Events kept from the previous run because the source failed.")
			m.val("openlitfass_source_carried_events", []string{"source", r.Name}, float64(r.Carried))
		}
	}

	// Source health from the status fields in the source files - also
	// right when another process (CI) does the fetching.
	if configs, err := config.Load(s.Instance.SourcesDir()); err == nil {
		for _, c := range configs {
			if c.Disabled {
				continue
			}
			upv := 0.0
			if c.Status != nil && c.Status.LastError == "" && c.Status.LastSuccess != "" {
				upv = 1
			}
			m.head("openlitfass_source_up", "gauge", "1 if the source's last fetch succeeded (from its status field).")
			m.val("openlitfass_source_up", []string{"source", c.Name}, upv)
		}
		for _, c := range configs {
			if c.Disabled || c.Status == nil {
				continue
			}
			if t, err := time.Parse(time.RFC3339, c.Status.LastSuccess); err == nil {
				m.head("openlitfass_source_last_success_timestamp_seconds", "gauge", "Unix time of the source's last successful fetch.")
				m.val("openlitfass_source_last_success_timestamp_seconds", []string{"source", c.Name}, float64(t.Unix()))
			}
		}
	}

	if ev.loaded {
		m.head("openlitfass_events", "gauge", "Events currently published (data/events.json).")
		m.val("openlitfass_events", nil, float64(ev.total))
		m.head("openlitfass_events_cancelled", "gauge", "Published events flagged as cancelled.")
		m.val("openlitfass_events_cancelled", nil, float64(ev.cancelled))
		m.head("openlitfass_events_merged", "gauge", "Published events reported by more than one source.")
		m.val("openlitfass_events_merged", nil, float64(ev.merged))
		m.head("openlitfass_events_by_source", "gauge", "Published events by the source of the shown version.")
		for _, k := range sortedKeys(ev.bySource) {
			m.val("openlitfass_events_by_source", []string{"source", k}, float64(ev.bySource[k]))
		}
		m.head("openlitfass_events_by_category", "gauge", "Published events by category.")
		for _, k := range sortedKeys(ev.byCategory) {
			m.val("openlitfass_events_by_category", []string{"category", k}, float64(ev.byCategory[k]))
		}
	}
}

// metricsHandler serves /metrics.
func (s *Server) metricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		s.WriteMetrics(w)
	}
}

// statusRecorder captures the response code for the request counter.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// countRequests wraps h and counts responses by status class.
func (s *Server) countRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		h.ServeHTTP(rec, r)
		class := fmt.Sprintf("%dxx", rec.code/100)
		s.mu.Lock()
		s.metrics.init()
		s.metrics.requests[class]++
		s.mu.Unlock()
	})
}
