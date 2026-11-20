package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

type JSONSource struct {
	cfg config.SourceConfig
}

func NewJSONSource(cfg config.SourceConfig) *JSONSource {
	return &JSONSource{cfg: cfg}
}

func (s *JSONSource) Name() string { return s.cfg.Name }

func (s *JSONSource) Fetch(ctx context.Context) ([]model.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.cfg.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", s.cfg.URL, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var root interface{}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	items := root
	if s.cfg.ItemsPath != "" {
		v, ok := mapping.Get(root, s.cfg.ItemsPath)
		if !ok {
			return nil, fmt.Errorf("itemsPath %q not found in response", s.cfg.ItemsPath)
		}
		items = v
	}

	list, ok := items.([]interface{})
	if !ok {
		return nil, fmt.Errorf("itemsPath %q does not point to an array", s.cfg.ItemsPath)
	}

	// Mapping is a pointer (see config.SourceConfig doc comment) so "wcs"
	// sources don't carry an empty mapping block; "rss"/"json" sources
	// always set it, but fall back to the zero value defensively rather
	// than panicking on a config mistake.
	m := mapping.SourceMapping{}
	if s.cfg.Mapping != nil {
		m = *s.cfg.Mapping
	}

	now := time.Now().UTC()
	events := make([]model.Event, 0, len(list))
	for _, item := range list {
		events = append(events, mapping.BuildEvent(item, m, s.cfg.Name, now))
	}
	return events, nil
}
