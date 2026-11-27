package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

func TestKeptImage(t *testing.T) {
	tests := []struct {
		raw     string
		allowed bool
		want    string
	}{
		{"https://example.org/a.jpg", true, "https://example.org/a.jpg"},
		{"  https://example.org/a b.jpg ", true, "https://example.org/a%20b.jpg"},
		{"https://example.org/a.jpg", false, ""},
		{"http://example.org/a.jpg", true, ""},
		{"/img/a.jpg", true, ""},
		{"data:image/png;base64,AAAA", true, ""},
		{"javascript:alert(1)", true, ""},
		{"https://user:pw@example.org/a.jpg", true, ""},
		{"https:///a.jpg", true, ""},
		{"", true, ""},
	}
	for _, tt := range tests {
		if got := keptImage(tt.raw, tt.allowed); got != tt.want {
			t.Errorf("keptImage(%q, %v) = %q, want %q", tt.raw, tt.allowed, got, tt.want)
		}
	}
}

func TestCommonFiltersDropImagesUnlessAllowed(t *testing.T) {
	evs := []model.Event{{Title: "a", Date: day(1), Image: "https://example.org/a.jpg"}}

	if got := applyCommonFilters(evs, config.SourceConfig{}); got[0].Image != "" {
		t.Errorf("images off by default, got %q", got[0].Image)
	}
	if got := applyCommonFilters(evs, config.SourceConfig{Images: true}); got[0].Image != "https://example.org/a.jpg" {
		t.Errorf("images on: got %q", got[0].Image)
	}
}

func TestICalImage(t *testing.T) {
	body := cal(
		"BEGIN:VEVENT\r\nUID:image\r\nSUMMARY:Image\r\nDTSTART:20261201T170000Z\r\nIMAGE;VALUE=URI;DISPLAY=BADGE:https://example.org/i.png\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:attach\r\nSUMMARY:Attach\r\nDTSTART:20261201T170000Z\r\nATTACH;FMTTYPE=application/pdf:https://example.org/f.pdf\r\nATTACH;FMTTYPE=image/jpeg:https://example.org/a.jpg\r\nEND:VEVENT\r\n",
		"BEGIN:VEVENT\r\nUID:inline\r\nSUMMARY:Inline\r\nDTSTART:20261201T170000Z\r\nATTACH;FMTTYPE=image/png;ENCODING=BASE64;VALUE=BINARY:AAAA\r\nEND:VEVENT\r\n",
	)
	from, to := icalWindow("2026-11-20", 60)
	evs, err := parseICal(body, config.SourceConfig{Name: "t"}, from, to, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range evs {
		got[e.Source.SourceID] = e.Image
	}
	want := map[string]string{
		"image":  "https://example.org/i.png",
		"attach": "https://example.org/a.jpg",
		"inline": "",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: image = %q, want %q", k, got[k], w)
		}
	}
}

func TestRSSAttributesForImages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0" xmlns:media="http://search.yahoo.com/mrss/"><channel>
<item><title>Mit Bild</title><pubDate>Tue, 01 Dec 2026 19:00:00 +0100</pubDate>
<enclosure url="https://example.org/e.jpg" type="image/jpeg" length="1"/>
<media:content url="https://example.org/m.jpg" medium="image"/></item>
</channel></rss>`))
	}))
	defer srv.Close()

	m := &mapping.SourceMapping{
		Title: mapping.FieldRule{Path: "title"},
		Start: mapping.FieldRule{Path: "pubdate", Format: "RFC1123Z"},
		Image: mapping.FieldRule{Path: "enclosure@url"},
	}
	evs, err := NewRSSSource(config.SourceConfig{Name: "rss", URL: srv.URL, Mapping: m}).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Image != "https://example.org/e.jpg" {
		t.Fatalf("enclosure@url: got %+v", evs)
	}

	m.Image = mapping.FieldRule{Path: "content@url"}
	evs, _ = NewRSSSource(config.SourceConfig{Name: "rss", URL: srv.URL, Mapping: m}).Fetch(context.Background())
	if len(evs) != 1 || evs[0].Image != "https://example.org/m.jpg" {
		t.Fatalf("content@url (media:content): got %+v", evs)
	}
	if evs[0].Title != "Mit Bild" {
		t.Errorf("element text still mapped, got title %q", evs[0].Title)
	}
}
