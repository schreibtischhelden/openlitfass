package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

// WCSSource fetches events from a WordPress site running the "Weekly Class
// Schedule" plugin (by CurlyThemes) via its admin-ajax.php endpoint — the
// same request its own frontend Vue widget makes, found by inspecting the
// browser's network tab (there's no documented public API for this plugin).
//
// This is hand-written Go rather than a generic RSS/JSON mapping because the
// endpoint needs: PHP-array-style repeated query params for the taxonomy
// filter, a computed rolling date-range window, and a genuine data quirk in
// how it reports times (see parseWCSLocal) that a generic mapper shouldn't
// have to special-case.
type WCSSource struct {
	cfg config.SourceConfig
}

func NewWCSSource(cfg config.SourceConfig) *WCSSource {
	return &WCSSource{cfg: cfg}
}

func (s *WCSSource) Name() string { return s.cfg.Name }

type wcsTerm struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type wcsEvent struct {
	Title     string `json:"title"`
	ID        int    `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Excerpt   string `json:"excerpt"`
	Permalink string `json:"permalink"`
	Visible   bool   `json:"visible"`
	Terms     struct {
		WcsType []wcsTerm `json:"wcs_type"`
		WcsRoom []wcsTerm `json:"wcs_room"`
	} `json:"terms"`
}

func (s *WCSSource) Fetch(ctx context.Context) ([]model.Event, error) {
	daysAhead := s.cfg.DaysAhead
	if daysAhead <= 0 {
		daysAhead = 30
	}

	todayStr, _ := model.LocalParts(time.Now())
	today, err := time.ParseInLocation("2006-01-02", todayStr, model.Local)
	if err != nil {
		return nil, fmt.Errorf("determine today: %w", err)
	}
	until := today.AddDate(0, 0, daysAhead)

	q := url.Values{}
	q.Set("action", "wcs_get_events_json")
	q.Set("start", today.Format("2006-01-02"))
	q.Set("end", until.Format("2006-01-02"))
	for _, id := range s.cfg.WCSTypeIDs {
		q.Add("content[wcs_type][]", strconv.Itoa(id))
	}

	reqURL := strings.TrimSuffix(s.cfg.URL, "/") + "/wp-admin/admin-ajax.php?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", reqURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", reqURL, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var raw []wcsEvent
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	now := time.Now().UTC()
	events := make([]model.Event, 0, len(raw))
	for _, e := range raw {
		// visible == false: a canceled occurrence of a recurring class
		// (mirrors the source's own meta._wcs_canceled list). Kept and
		// flagged, see model.Event.Cancelled.
		cancelled := !e.Visible

		start, err := parseWCSLocal(e.Start)
		if err != nil {
			continue
		}
		date, startTime := model.LocalParts(start)
		endTime := ""
		if end, err := parseWCSLocal(e.End); err == nil {
			_, endTime = model.LocalParts(end)
		}

		category := ""
		if len(e.Terms.WcsType) > 0 {
			category = e.Terms.WcsType[0].Name
			if mapped, ok := s.cfg.CategoryMap[e.Terms.WcsType[0].Slug]; ok {
				category = mapped
			}
		}

		room := ""
		if len(e.Terms.WcsRoom) > 0 {
			room = e.Terms.WcsRoom[0].Name
		}
		location := wcsLocation(s.cfg.LocationDefault, room)

		events = append(events, model.Event{
			Title:       e.Title,
			Description: stripHTML(e.Excerpt),
			Category:    category,
			Date:        date,
			StartTime:   startTime,
			EndTime:     endTime,
			Location:    location,
			Price:       model.Price{Free: true, Amount: nil},
			Organizer:   s.cfg.Organizer,
			URL:         e.Permalink,
			// The plugin's API exposes no per-event signal for either of
			// these, so both are always false here - a known limitation,
			// not a claim that no wcs class ever requires booking. Revisit
			// if a real per-event field turns up in the API response.
			TicketRequired:       false,
			RegistrationRequired: false,
			// Per-source, not per-event (see config.SourceConfig.FamilyFriendly
			// doc comment) - true only if this source's config says so.
			FamilyFriendly: s.cfg.FamilyFriendly,
			Cancelled:      cancelled,
			Source: model.SourceRef{
				Name:     s.cfg.Name,
				SourceID: fmt.Sprintf("%d-%d", e.ID, e.Timestamp),
			},
			LastSeen: now,
		})
	}
	return events, nil
}

// wcsLocation builds the event location from locationDefault ("Venue,
// Street 1, 12345 City") and the optional wcs_room term. The venue (the part
// before the first comma) becomes the name, so lists, place matching and the
// map refer to the house rather than to one of its rooms; the room goes into
// the full address right after the venue:
// "Venue, Bewegungsraum, Street 1, 12345 City".
func wcsLocation(locationDefault, room string) model.Location {
	venue, rest, hasRest := strings.Cut(locationDefault, ",")
	venue = strings.TrimSpace(venue)
	parts := []string{venue}
	if room != "" {
		parts = append(parts, room)
	}
	if hasRest {
		if r := strings.TrimSpace(rest); r != "" {
			parts = append(parts, r)
		}
	}
	if len(parts) == 1 {
		return model.Location{Name: venue}
	}
	return model.Location{Name: venue, Address: strings.Join(parts, ", ")}
}

// parseWCSLocal parses the plugin's "start"/"end" strings. Despite the
// literal "+00:00"/"Z" suffix they carry, the digits are already the
// venue's local wall-clock time, not a real UTC instant — confirmed for the
// first wcs source this was built against (a Berlin venue whose published
// "16:30–18:00" matched the raw digits directly; a genuine UTC->local
// conversion would have shifted them by +2h in summer). So only the first
// 19 characters ("2006-01-02T15:04:05") are parsed, as instance-local time
// (model.Local), ignoring the (incorrect) offset suffix entirely. This
// assumes the venue is in the instance's timezone.
//
// This is a verified one-off for THIS plugin, not a general rule - do not
// copy this trick into a new source without individually re-verifying it
// against that source's real published times. Comparable aggregator
// projects (e.g. judell/community-calendar, see their docs/timezone.md)
// report feeds within a single city that genuinely use different timezones
// because the platform (Meetup, Eventbrite, ...) stamps its own default
// instead of the venue's - so "the digits are already local" is never a
// safe default assumption, only something to confirm per source. See
// https://docs.openlitfass.org/?page=source-types#weekly-class-schedule-wcs for the long version.
func parseWCSLocal(s string) (time.Time, error) {
	if len(s) < 19 {
		return time.Time{}, fmt.Errorf("timestamp too short: %q", s)
	}
	return time.ParseInLocation("2006-01-02T15:04:05", s[:19], model.Local)
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML turns the plugin's rich-text excerpt (WordPress block comments,
// tags, entities) into plain text for Event.Description, which the frontend
// renders via .textContent (see web/js/app.js) rather than as HTML.
func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
