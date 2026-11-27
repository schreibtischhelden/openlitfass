package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
)

// widgetServer imitates a widget like infomax: the list page only carries
// event ids, /items renders the entries for the requested ids.
type widgetServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	times    []time.Time
}

func newWidgetServer(t *testing.T) *widgetServer {
	t.Helper()
	ws := &widgetServer{}
	day := time.Now().AddDate(0, 0, 2).Format("02.01.2006")
	mux := http.NewServeMux()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		ws.record(r)
		fmt.Fprint(w, `<html><body>
<div class="day" data-idents="ev_1,ev_2"></div>
<div class="day" data-idents="ev_2, ev_3,"></div>
<a class="more" href="/detail/1">1</a><a class="more" href="detail/2">2</a><a class="more" href="/detail/3">3</a>
</body></html>`)
	})
	mux.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		ws.record(r)
		if d, err := time.ParseDuration(r.URL.Query().Get("slowFor")); err == nil && r.URL.Query().Get("object[]") == r.URL.Query().Get("slowID") {
			time.Sleep(d)
		}
		for _, id := range r.URL.Query()["object[]"] {
			fmt.Fprintf(w, `<article data-ident="%s"><h2>Event %s</h2><p class="info">%s / 19:00 Uhr / Saal</p></article>`, id, id, day)
		}
	})
	mux.HandleFunc("/detail/", func(w http.ResponseWriter, r *http.Request) {
		ws.record(r)
		id := strings.TrimPrefix(r.URL.Path, "/detail/")
		fmt.Fprintf(w, `<article data-ident="d%s"><h2>Detail %s</h2><p class="info">%s / 10:00 Uhr / Hof</p></article>`, id, id, day)
	})
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		ws.record(r)
		fmt.Fprint(w, `{"data": [{"id": 21063490}, {"id": 21063491}, {"id": 21063490}]}`)
	})
	ws.Server = httptest.NewServer(mux)
	t.Cleanup(ws.Close)
	return ws
}

func (ws *widgetServer) record(r *http.Request) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.requests = append(ws.requests, r.URL.RequestURI())
	ws.times = append(ws.times, time.Now())
}

const widgetItemFields = `
  "itemSelector": "article[data-ident]",
  "extract": {
    "id": { "attr": "data-ident" },
    "title": "h2",
    "date": { "selector": "p.info", "regex": "^(\\d{2}\\.\\d{2}\\.\\d{4})" },
    "start": { "selector": "p.info", "regex": "/ (\\d{1,2}:\\d{2})" }
  },
  "mapping": {
    "sourceId": { "path": "id" },
    "title": { "path": "title" },
    "start": { "template": "{date} {start}", "format": "02.01.2006 15:04" }
  }`

func stepConfig(t *testing.T, src string) config.SourceConfig {
	t.Helper()
	var cfg config.SourceConfig
	if err := json.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Name = "steps-test"
	return cfg
}

func titles(t *testing.T, cfg config.SourceConfig) []string {
	t.Helper()
	if !usesSteps(&cfg) {
		t.Fatal("config does not use steps")
	}
	evs, err := newStepSource(cfg, newTypedSource).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(evs))
	for i := range evs {
		out = append(out, evs[i].Title)
	}
	return out
}

func TestStepsCollectIDsAndBatch(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	cfg := stepConfig(t, `{
  "type": "html",
  "daysAhead": 10,
  "steps": [{
    "url": "`+ws.URL+`/list?from={today}&to={until}",
    "collect": { "ids": { "selector": "[data-idents]", "attr": "data-idents", "split": "," } }
  }],
  "url": "`+ws.URL+`/items?layout=list",
  "forEach": { "var": "ids", "param": "object[]", "batch": 2 },`+widgetItemFields+`
}`)

	// Act
	got := titles(t, cfg)

	// Assert
	if want := "Event ev_1,Event ev_2,Event ev_3"; strings.Join(got, ",") != want {
		t.Errorf("titles = %v, want %s", got, want)
	}
	today := time.Now().Format("2006-01-02")
	until := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	want := []string{
		"/list?from=" + today + "&to=" + until,
		"/items?layout=list&object%5B%5D=ev_1&object%5B%5D=ev_2",
		"/items?layout=list&object%5B%5D=ev_3",
	}
	if strings.Join(ws.requests, " ") != strings.Join(want, " ") {
		t.Errorf("requests = %v, want %v", ws.requests, want)
	}
}

func TestStepsForEachItemURLAndMaxRequests(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	cfg := stepConfig(t, `{
  "type": "html",
  "steps": [{
    "url": "`+ws.URL+`/list",
    "collect": { "links": { "selector": "a.more", "attr": "href" } }
  }],
  "url": "{item}",
  "forEach": { "var": "links" },
  "maxRequests": 2,`+widgetItemFields+`
}`)

	// Act
	got := titles(t, cfg)

	// Assert
	if want := "Detail 1,Detail 2"; strings.Join(got, ",") != want {
		t.Errorf("titles = %v, want %s (relative links resolved, third skipped)", got, want)
	}
}

func TestStepsJSONCollectKeepsLargeIDs(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	cfg := stepConfig(t, `{
  "type": "html",
  "steps": [{
    "type": "json",
    "url": "`+ws.URL+`/index.json",
    "collect": { "ids": { "path": "data.*.id", "regex": "(\\d+)" } }
  }],
  "url": "`+ws.URL+`/items?x=1&id={ids}",
  "forEach": { "var": "ids", "param": "object[]", "batch": 10 },`+widgetItemFields+`
}`)

	// Act
	got := titles(t, cfg)

	// Assert
	if want := "Event 21063490,Event 21063491"; strings.Join(got, ",") != want {
		t.Errorf("titles = %v, want %s", got, want)
	}
	if last := ws.requests[len(ws.requests)-1]; !strings.Contains(last, "id=21063490&object%5B%5D=21063490&object%5B%5D=21063491") {
		t.Errorf("request = %s, want {ids} expanded to the first value plus both ids as params", last)
	}
}

func TestStepsErrors(t *testing.T) {
	ws := newWidgetServer(t)
	cases := map[string]struct {
		src  string
		want string
	}{
		"collect finds nothing": {
			`{"type": "html", "steps": [{"url": "` + ws.URL + `/list", "collect": {"ids": {"selector": ".missing", "attr": "data-x"}}}],
			  "url": "` + ws.URL + `/items", "forEach": {"var": "ids", "param": "o"},` + widgetItemFields + `}`,
			`step 1: collect "ids" found nothing`,
		},
		"unknown placeholder": {
			`{"type": "html", "url": "` + ws.URL + `/items?from={todya}",` + widgetItemFields + `}`,
			"unknown placeholder {todya}",
		},
		"forEach without variable": {
			`{"type": "html", "url": "` + ws.URL + `/items", "forEach": {"var": "ids", "param": "o"},` + widgetItemFields + `}`,
			`no variable "ids"`,
		},
		"forEach without param or item": {
			`{"type": "html", "steps": [{"url": "` + ws.URL + `/list", "collect": {"ids": {"selector": "[data-idents]", "attr": "data-idents"}}}],
			  "url": "` + ws.URL + `/items", "forEach": {"var": "ids"},` + widgetItemFields + `}`,
			"needs {item}",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// Arrange
			cfg := stepConfig(t, tc.src)

			// Act
			_, err := newStepSource(cfg, newTypedSource).Fetch(context.Background())

			// Assert
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSourceTimeout(t *testing.T) {
	for seconds, want := range map[int]time.Duration{
		0:    defaultSourceTimeout,
		-5:   defaultSourceTimeout,
		90:   90 * time.Second,
		3600: maxSourceTimeout,
	} {
		if got := sourceTimeout(seconds); got != want {
			t.Errorf("sourceTimeout(%d) = %s, want %s", seconds, got, want)
		}
	}
}

func TestUsesSteps(t *testing.T) {
	plain := config.SourceConfig{Type: "html", URL: "https://example.org/list?x=1"}
	if usesSteps(&plain) {
		t.Error("plain source must not use steps")
	}
	dated := config.SourceConfig{Type: "ical", URL: "https://example.org/feed.ics?from={today}"}
	if !usesSteps(&dated) {
		t.Error("{today} in the url needs the step source")
	}
}

const pacedWidgetConfig = `{
  "type": "html",
  "steps": [{
    "url": "%s/list",
    "collect": { "ids": { "selector": "[data-idents]", "attr": "data-idents", "split": "," } }
  }],
  "url": "%s/items?%s",
  "forEach": { "var": "ids", "param": "object[]", "batch": 1 },` + widgetItemFields + `
}`

func TestStepsRequestDelaySpacesRequests(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	s := newStepSource(stepConfig(t, fmt.Sprintf(pacedWidgetConfig, ws.URL, ws.URL, "x=1")), newTypedSource)
	s.delay = 80 * time.Millisecond

	// Act
	evs, err := s.Fetch(context.Background())

	// Assert
	if err != nil || len(evs) != 3 {
		t.Fatalf("got %d events, err %v; want 3 events", len(evs), err)
	}
	if len(ws.times) != 4 {
		t.Fatalf("requests = %v, want the list and three item requests", ws.requests)
	}
	for i := 1; i < len(ws.times); i++ {
		if gap := ws.times[i].Sub(ws.times[i-1]); gap < s.delay-10*time.Millisecond {
			t.Errorf("request %d started %s after the previous one, want at least %s", i+1, gap, s.delay)
		}
	}
}

func TestStepsRequestTimeoutAppliesPerRequest(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	s := newStepSource(stepConfig(t, fmt.Sprintf(pacedWidgetConfig, ws.URL, ws.URL, "slowFor=500ms&slowID=ev_2")), newTypedSource)
	s.delay = time.Millisecond
	s.requestTimeout = 150 * time.Millisecond

	// Act
	got, err := s.Fetch(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("err = %v, want the other batches kept", err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Title)
	}
	if want := "Event ev_1,Event ev_3"; strings.Join(names, ",") != want {
		t.Errorf("titles = %v, want %s (slow batch timed out on its own)", names, want)
	}
}

func TestStepsRequestDelayStopsWhenContextEnds(t *testing.T) {
	// Arrange
	ws := newWidgetServer(t)
	s := newStepSource(stepConfig(t, fmt.Sprintf(pacedWidgetConfig, ws.URL, ws.URL, "x=1")), newTypedSource)
	s.delay = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()

	// Act
	_, err := s.Fetch(ctx)

	// Assert
	if err == nil {
		t.Error("err = nil, want the time limit reported")
	}
	if took := time.Since(started); took > time.Second {
		t.Errorf("Fetch took %s, want it to stop waiting when the context ends", took)
	}
}

func TestNewStepSourcePacing(t *testing.T) {
	paced := newStepSource(config.SourceConfig{RequestDelaySeconds: 5, TimeoutSeconds: 30}, newTypedSource)
	if paced.delay != 5*time.Second || paced.requestTimeout != 30*time.Second {
		t.Errorf("delay %s, request timeout %s; want 5s and 30s", paced.delay, paced.requestTimeout)
	}
	plain := newStepSource(config.SourceConfig{TimeoutSeconds: 30}, newTypedSource)
	if plain.delay != 0 || plain.requestTimeout != 0 {
		t.Errorf("without requestDelaySeconds: delay %s, request timeout %s; want none", plain.delay, plain.requestTimeout)
	}
}

func TestAllRunsPacedSourcesLast(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	files := map[string]string{
		"a-paced.json": `{"type": "html", "url": "https://example.org/items", "requestDelaySeconds": 5,
		  "steps": [{"url": "https://example.org/list", "collect": {"ids": {"selector": "[data-id]", "attr": "data-id"}}}],
		  "forEach": {"var": "ids", "param": "id"}}`,
		"b-plain.json": `{"type": "rss", "url": "https://example.org/feed"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Act
	all := All(dir, filepath.Join(dir, "inbox"), filepath.Join(dir, "holidays.json"))

	// Assert
	var names []string
	for _, s := range all {
		names = append(names, s.Name())
	}
	if want := "b-plain,inbox,a-paced"; strings.Join(names, ",") != want {
		t.Errorf("order = %v, want %s", names, want)
	}
}
