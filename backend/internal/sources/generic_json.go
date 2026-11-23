package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

// defaultMaxPages caps JSON pagination when a source sets nextPath but no
// maxPages.
const defaultMaxPages = 10

// userAgent identifies openLitfass to the sites it fetches from, so
// operators can see who's calling (and block or contact if needed).
const userAgent = "openLitfass-fetch/1.0 (event calendar aggregator)"

type JSONSource struct {
	cfg config.SourceConfig
}

func NewJSONSource(cfg config.SourceConfig) *JSONSource {
	return &JSONSource{cfg: cfg}
}

func (s *JSONSource) Name() string { return s.cfg.Name }

// Fetch reads cfg.URL and, if cfg.NextPath is set, follows the next-page
// URLs (see config.SourceConfig.NextPath for when it stops).
func (s *JSONSource) Fetch(ctx context.Context) ([]model.Event, error) {
	// Mapping is a pointer (see config.SourceConfig doc comment) so "wcs"
	// sources don't carry an empty mapping block; "rss"/"json" sources
	// always set it, but fall back to the zero value defensively rather
	// than panicking on a config mistake.
	m := mapping.SourceMapping{}
	if s.cfg.Mapping != nil {
		m = *s.cfg.Mapping
	}

	maxPages := 1
	if s.cfg.NextPath != "" {
		maxPages = s.cfg.MaxPages
		if maxPages <= 0 {
			maxPages = defaultMaxPages
		}
	}
	maxDate := windowEnd(s.cfg.DaysAhead)

	now := time.Now().UTC()
	var events []model.Event
	url := s.cfg.URL
	seen := map[string]bool{}
	for page := 1; page <= maxPages && url != "" && !seen[url]; page++ {
		seen[url] = true
		root, list, err := s.fetchPage(ctx, url)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			// A later page failing shouldn't throw away what we have
			// (returning an error would - see withCommonFilters/harden).
			log.Printf("source %q: page %d failed, keeping %d events from earlier pages: %v", s.cfg.Name, page, len(events), err)
			break
		}

		beyond := len(list) > 0
		for _, item := range list {
			ev := mapping.BuildEvent(item, &m, s.cfg.Name, now)
			events = append(events, ev)
			if maxDate == "" || ev.Date == "" || ev.Date <= maxDate {
				beyond = false
			}
		}
		if beyond {
			break // whole page outside the daysAhead window: stop paginating
		}

		url = ""
		if s.cfg.NextPath != "" {
			url = mapping.String(root, s.cfg.NextPath)
		}
	}
	return events, nil
}

func (s *JSONSource) fetchPage(ctx context.Context, url string) (interface{}, []interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response: %w", err)
	}

	var root interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, nil, fmt.Errorf("parse json: %w", err)
	}

	items := root
	if s.cfg.ItemsPath != "" {
		v, ok := mapping.Get(root, s.cfg.ItemsPath)
		if !ok {
			return nil, nil, fmt.Errorf("itemsPath %q not found in response", s.cfg.ItemsPath)
		}
		items = v
	}

	list, ok := items.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("itemsPath %q does not point to an array", s.cfg.ItemsPath)
	}
	return root, list, nil
}

// windowEnd returns the last date (YYYY-MM-DD, instance-local) inside a
// daysAhead window starting today, or "" for no window.
func windowEnd(daysAhead int) string {
	if daysAhead <= 0 {
		return ""
	}
	return time.Now().In(model.Local).AddDate(0, 0, daysAhead).Format("2006-01-02")
}
