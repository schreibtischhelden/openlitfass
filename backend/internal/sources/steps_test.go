package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
