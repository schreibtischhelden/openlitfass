package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

// pagedServer serves /page/1../page/n; each page has two events on the
// given dates and points to the next page via "next".
func pagedServer(t *testing.T, dates [][]string) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		var p int
		fmt.Sscanf(r.URL.Path, "/page/%d", &p)
		if p < 1 || p > len(dates) {
			http.NotFound(w, r)
			return
		}
		next := ""
		if p < len(dates) {
			next = fmt.Sprintf("%s/page/%d", srv.URL, p+1)
		}
		fmt.Fprintf(w, `{"next":%q,"events":[`, next)
		for i, d := range dates[p-1] {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"id":"%d-%d","title":"E%d-%d","start":"%s 10:00:00"}`, p, i, p, i, d)
		}
		fmt.Fprint(w, "]}")
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func jsonCfg(url string) config.SourceConfig {
	return config.SourceConfig{
		Name:      "test",
		Type:      "json",
		URL:       url,
		ItemsPath: "events",
		NextPath:  "next",
		Mapping: &mapping.SourceMapping{
			SourceID: mapping.FieldRule{Path: "id"},
			Title:    mapping.FieldRule{Path: "title"},
			Start:    mapping.FieldRule{Path: "start", Format: "2006-01-02 15:04:05"},
		},
	}
}

func day(offset int) string {
	return time.Now().In(model.Local).AddDate(0, 0, offset).Format("2006-01-02")
}

func TestJSONFollowsPages(t *testing.T) {
	srv, hits := pagedServer(t, [][]string{{day(1), day(2)}, {day(3), day(4)}, {day(5), day(6)}})
	evs, err := NewJSONSource(jsonCfg(srv.URL + "/page/1")).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 6 || *hits != 3 {
		t.Fatalf("got %d events in %d requests, want 6 in 3", len(evs), *hits)
	}
}

func TestJSONStopsBeyondWindow(t *testing.T) {
	srv, hits := pagedServer(t, [][]string{{day(1), day(2)}, {day(40), day(41)}, {day(80), day(81)}})
	cfg := jsonCfg(srv.URL + "/page/1")
	cfg.DaysAhead = 30
	if _, err := NewJSONSource(cfg).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *hits != 2 {
		t.Fatalf("expected to stop after the first page fully beyond the window (2 requests), got %d", *hits)
	}
}

func TestJSONRespectsMaxPages(t *testing.T) {
	srv, hits := pagedServer(t, [][]string{{day(1)}, {day(2)}, {day(3)}, {day(4)}})
	cfg := jsonCfg(srv.URL + "/page/1")
	cfg.MaxPages = 2
	evs, err := NewJSONSource(cfg).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if *hits != 2 || len(evs) != 2 {
		t.Fatalf("got %d events in %d requests, want 2 in 2", len(evs), *hits)
	}
}

func TestJSONKeepsEarlierPagesOnLaterFailure(t *testing.T) {
	srv, _ := pagedServer(t, [][]string{{day(1), day(2)}})
	cfg := jsonCfg(srv.URL + "/page/1")
	// Point "next" at a 404 by serving only one page but forcing a bogus next URL.
	cfg.NextPath = "events.0.id" // resolves to "1-0", an invalid URL
	evs, err := NewJSONSource(cfg).Fetch(context.Background())
	if err != nil {
		t.Fatalf("later page failure should not fail the source: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("want the 2 events of page 1, got %d", len(evs))
	}
}
