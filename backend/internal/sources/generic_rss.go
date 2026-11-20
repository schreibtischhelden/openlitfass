package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

type RSSSource struct {
	cfg config.SourceConfig
}

func NewRSSSource(cfg config.SourceConfig) *RSSSource {
	return &RSSSource{cfg: cfg}
}

func (s *RSSSource) Name() string { return s.cfg.Name }

type rssFeed struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Fields []rssField `xml:",any"`
}

type rssField struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func (s *RSSSource) Fetch(ctx context.Context) ([]model.Event, error) {
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

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse rss: %w", err)
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
	events := make([]model.Event, 0, len(feed.Channel.Items))
	for _, raw := range feed.Channel.Items {
		item := make(map[string]interface{}, len(raw.Fields))
		for _, f := range raw.Fields {
			key := strings.ToLower(f.XMLName.Local)
			if existing, ok := item[key]; ok {
				switch v := existing.(type) {
				case []interface{}:
					item[key] = append(v, f.Value)
				default:
					item[key] = []interface{}{v, f.Value}
				}
				continue
			}
			item[key] = f.Value
		}
		events = append(events, mapping.BuildEvent(item, m, s.cfg.Name, now))
	}
	return events, nil
}
