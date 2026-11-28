package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openlitfass/backend/internal/instance"
)

// testInstance creates a minimal instance + web dir in a temp dir.
func testInstance(t *testing.T) (instance.Instance, string) {
	t.Helper()
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	web := filepath.Join(root, "web")
	for _, d := range []string{filepath.Join(instDir, "data"), filepath.Join(web, "js")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(instDir, "instance.json"):       `{"name":"test.events"}`,
		filepath.Join(instDir, "data", "events.json"): `{"events":[]}`,
		filepath.Join(web, "index.html"):              `<html><title>{{.Title}}</title>{{.Logo}}</html>`,
		filepath.Join(web, "logo.svg"):                `<svg></svg>`,
		filepath.Join(web, "sw.js"):                   `const CACHE = "x-__BUILD_ID__";`,
		filepath.Join(web, "js", "app.js"):            `// app`,
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	return inst, web
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func TestServerServesAndSwapsBuilds(t *testing.T) {
	inst, web := testInstance(t)
	s := &Server{Instance: inst, WebDir: web, PublicDir: inst.Path("public")}
	h := s.Handler()

	if r := get(t, h, "/"); r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("before the first build: %d, want 503", r.StatusCode)
	}

	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	first := *s.current.Load()
	r := get(t, h, "/")
	body, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !strings.Contains(string(body), "<title>test.events</title>") {
		t.Fatalf("index: %d %s", r.StatusCode, body)
	}
	if cc := r.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control = %q", cc)
	}
	if cc := get(t, h, "/js/app.js").Header.Get("Cache-Control"); !strings.Contains(cc, "max-age") {
		t.Errorf("asset Cache-Control = %q", cc)
	}
	if cc := get(t, h, "/data/events.json").Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("data Cache-Control = %q", cc)
	}
	if code := get(t, h, "/data/").StatusCode; code != http.StatusNotFound {
		t.Errorf("directory listing should be hidden, got %d", code)
	}
	if code := get(t, h, "/"+BuildMarker).StatusCode; code != http.StatusNotFound {
		t.Errorf("build marker should be hidden, got %d", code)
	}

	// A failed build keeps the previous version online.
	s.WebDir = filepath.Join(web, "missing")
	if err := s.Rebuild(); err == nil {
		t.Fatal("build with a missing web dir should fail")
	}
	if *s.current.Load() != first || get(t, h, "/").StatusCode != 200 {
		t.Error("failed build must not replace the live one")
	}

	// A good build switches over.
	s.WebDir = web
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if *s.current.Load() == first {
		t.Error("rebuild should switch to a new directory")
	}
}

func TestHealthz(t *testing.T) {
	inst, web := testInstance(t)
	s := &Server{Instance: inst, WebDir: web, PublicDir: inst.Path("public")}
	h := s.Handler()
	if code := get(t, h, "/healthz").StatusCode; code != http.StatusServiceUnavailable {
		t.Errorf("healthz before build: %d", code)
	}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	r := get(t, h, "/healthz")
	var st Status
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil || r.StatusCode != 200 || !st.Serving || st.Instance != "test.events" {
		t.Errorf("healthz: %d %+v %v", r.StatusCode, st, err)
	}
}

func TestMetrics(t *testing.T) {
	inst, web := testInstance(t)
	// Two events, one cancelled, one merged from two sources.
	if err := os.WriteFile(inst.EventsOutput(), []byte(`{"events":[
	  {"id":"a","title":"A","date":"2030-01-01","category":"Kultur","source":"brot","sources":["brot","agg"],"price":{"free":true}},
	  {"id":"b","title":"B \"x\"","date":"2030-01-02","category":"Sport","source":"fz","cancelled":true,"price":{"free":true}}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(inst.SourcesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(inst.SourcesDir(), "brot.json"), []byte(`{"type":"json","status":{"lastSuccess":"2030-01-01T10:00:00Z"}}`), 0o644)
	os.WriteFile(filepath.Join(inst.SourcesDir(), "fz.json"), []byte(`{"type":"wcs","status":{"lastSuccess":"2029-12-31T10:00:00Z","lastError":"timeout"}}`), 0o644)

	s := &Server{Instance: inst, WebDir: web, PublicDir: inst.Path("public")}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	get(t, h, "/")
	get(t, h, "/nope")

	r := get(t, h, "/metrics")
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Errorf("Content-Type = %q", ct)
	}
	body, _ := io.ReadAll(r.Body)
	out := string(body)
	for _, want := range []string{
		`openlitfass_info{version="dev",instance="test.events"} 1`,
		"openlitfass_up 1",
		"openlitfass_events 2",
		"openlitfass_events_cancelled 1",
		"openlitfass_events_merged 1",
		`openlitfass_events_by_source{source="brot"} 1`,
		`openlitfass_events_by_category{category="Sport"} 1`,
		`openlitfass_source_up{source="brot"} 1`,
		`openlitfass_source_up{source="fz"} 0`,
		`openlitfass_source_last_success_timestamp_seconds{source="brot"} 1893492000`,
		`openlitfass_builds_total{result="ok"} 1`,
		`openlitfass_http_requests_total{code="2xx"} 1`,
		`openlitfass_http_requests_total{code="4xx"} 1`,
		"# TYPE openlitfass_fetch_runs_total counter",
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Every non-comment line is "name{labels} value".
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if f := strings.Fields(line); len(f) < 2 {
			t.Errorf("malformed line %q", line)
		}
	}
	// Healthz now knows the event count even without fetching.
	if st := s.Status(); st.Events != 2 {
		t.Errorf("healthz events = %d, want 2", st.Events)
	}
}

func TestMetricsSeparateAddrHidesThemFromSite(t *testing.T) {
	inst, web := testInstance(t)
	s := &Server{Instance: inst, WebDir: web, PublicDir: inst.Path("public"), MetricsAddr: ":0"}
	if err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	if code := get(t, s.Handler(), "/metrics").StatusCode; code != http.StatusNotFound {
		t.Errorf("with MetricsAddr, /metrics must not be on the site, got %d", code)
	}
}

func TestEscapeLabel(t *testing.T) {
	if got := labelString([]string{"t", "a\"b\\c\nd"}); got != `{t="a\"b\\c\nd"}` {
		t.Errorf("got %s", got)
	}
}
