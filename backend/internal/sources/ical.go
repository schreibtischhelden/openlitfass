package sources

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

// ICalSource reads an iCalendar (.ics, RFC 5545) feed - the export format
// of Google/Nextcloud/Outlook calendars and of most WordPress event plugins
// (Events Manager, The Events Calendar, MEC, ...).
//
// Supported: folded lines, text escapes, DTSTART/DTEND/DURATION as UTC,
// TZID or floating time (calendar X-WR-TIMEZONE, else the instance
// timezone), all-day events (VALUE=DATE), STATUS:CANCELLED (kept, flagged
// as model.Event.Cancelled - also for a cancelled RECURRENCE-ID occurrence),
// recurring events (RRULE: DAILY/WEEKLY/MONTHLY/YEARLY with INTERVAL,
// COUNT, UNTIL, BYDAY for WEEKLY; EXDATE; RECURRENCE-ID overrides),
// expanded only inside the fetch window.
//
// Field mapping is fixed (no "mapping" block): SUMMARY -> title,
// DESCRIPTION -> description, URL, first CATEGORIES value -> category
// (translate with categoryMap), ORGANIZER;CN -> organizer (else the
// config's "organizer"), X-APPLE-STRUCTURED-LOCATION title/address or
// LOCATION -> location (else "locationDefault"). iCal has no price field,
// so the price is unknown (nothing shown).
type ICalSource struct {
	cfg config.SourceConfig
}

func NewICalSource(cfg config.SourceConfig) *ICalSource {
	return &ICalSource{cfg: cfg}
}

func (s *ICalSource) Name() string { return s.cfg.Name }

// defaultICalWindowDays bounds RRULE expansion when the source sets no
// daysAhead; maxOccurrences bounds it in any case.
const (
	defaultICalWindowDays = 180
	maxOccurrences        = 500
)

func (s *ICalSource) Fetch(ctx context.Context) ([]model.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/calendar, */*;q=0.5")
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
	if !strings.Contains(string(body), "BEGIN:VCALENDAR") {
		return nil, fmt.Errorf("%s is not an iCalendar file", s.cfg.URL)
	}

	days := s.cfg.DaysAhead
	if days <= 0 {
		days = defaultICalWindowDays
	}
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, model.Local)
	to := from.AddDate(0, 0, days+1)

	return parseICal(string(body), s.cfg, from, to, now.UTC())
}

// ---- parsing ----

type icalProp struct {
	name   string
	params map[string]string
	value  string
}

type vevent struct {
	props map[string][]icalProp
}

func (v vevent) first(name string) (icalProp, bool) {
	p, ok := v.props[name]
	if !ok || len(p) == 0 {
		return icalProp{}, false
	}
	return p[0], true
}

func (v vevent) text(name string) string {
	p, ok := v.first(name)
	if !ok {
		return ""
	}
	return unescapeText(p.value)
}

// unfold joins RFC 5545 folded lines (CRLF followed by space or tab).
func unfold(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

func parseLine(line string) (icalProp, bool) {
	// The value starts after the first ':' that is not inside a quoted
	// parameter value.
	inQuote := false
	colon := -1
	for i, r := range line {
		if r == '"' {
			inQuote = !inQuote
		} else if r == ':' && !inQuote {
			colon = i
			break
		}
	}
	if colon < 0 {
		return icalProp{}, false
	}
	head, value := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	p := icalProp{name: strings.ToUpper(parts[0]), params: map[string]string{}, value: value}
	for _, kv := range parts[1:] {
		if k, v, ok := strings.Cut(kv, "="); ok {
			p.params[strings.ToUpper(k)] = strings.Trim(v, `"`)
		}
	}
	return p, true
}

func unescapeText(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
	return r.Replace(s)
}

func parseCalendar(s string) (events []vevent, calTZ string) {
	var cur *vevent
	depth := 0 // nesting inside VEVENT (VALARM etc.)
	for _, line := range unfold(s) {
		if line == "" {
			continue
		}
		p, ok := parseLine(line)
		if !ok {
			continue
		}
		switch {
		case p.name == "BEGIN" && strings.EqualFold(p.value, "VEVENT"):
			cur = &vevent{props: map[string][]icalProp{}}
			depth = 0
		case p.name == "END" && strings.EqualFold(p.value, "VEVENT"):
			if cur != nil {
				events = append(events, *cur)
			}
			cur = nil
		case cur != nil && p.name == "BEGIN":
			depth++
		case cur != nil && p.name == "END":
			depth--
		case cur != nil && depth == 0:
			cur.props[p.name] = append(cur.props[p.name], p)
		case cur == nil && p.name == "X-WR-TIMEZONE":
			calTZ = strings.TrimSpace(p.value)
		}
	}
	return events, calTZ
}

// parseICalTime returns the instant and whether it is a date-only value.
func parseICalTime(p icalProp, calTZ string) (time.Time, bool, error) {
	v := strings.TrimSpace(p.value)
	if strings.EqualFold(p.params["VALUE"], "DATE") || len(v) == 8 {
		t, err := time.ParseInLocation("20060102", v, model.Local)
		return t, true, err
	}
	if strings.HasSuffix(v, "Z") {
		t, err := time.Parse("20060102T150405Z", v)
		return t, false, err
	}
	loc := model.Local
	tz := p.params["TZID"]
	if tz == "" {
		tz = calTZ
	}
	if tz != "" {
		if l, err := time.LoadLocation(strings.TrimPrefix(tz, "/")); err == nil {
			loc = l
		}
		// Unknown zone names (e.g. Windows "W. Europe Standard Time"):
		// fall back to the instance timezone rather than failing.
	}
	t, err := time.ParseInLocation("20060102T150405", v, loc)
	return t, false, err
}

// parseDuration handles the common subset of RFC 5545 durations:
// P#W, P#DT#H#M#S (any part optional).
func parseDuration(s string) (time.Duration, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	if !strings.HasPrefix(s, "P") {
		return 0, false
	}
	s = s[1:]
	var d time.Duration
	inTime := false
	num := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			num += string(r)
		case r == 'T':
			inTime = true
		default:
			n, err := strconv.Atoi(num)
			if err != nil {
				return 0, false
			}
			num = ""
			switch {
			case r == 'W':
				d += time.Duration(n) * 7 * 24 * time.Hour
			case r == 'D':
				d += time.Duration(n) * 24 * time.Hour
			case r == 'H' && inTime:
				d += time.Duration(n) * time.Hour
			case r == 'M' && inTime:
				d += time.Duration(n) * time.Minute
			case r == 'S' && inTime:
				d += time.Duration(n) * time.Second
			default:
				return 0, false
			}
		}
	}
	if neg {
		d = -d
	}
	return d, true
}

func parseICal(body string, cfg config.SourceConfig, from, to time.Time, now time.Time) ([]model.Event, error) {
	vevents, calTZ := parseCalendar(body)

	// RECURRENCE-ID overrides: UID -> set of replaced occurrence starts.
	overridden := map[string]map[int64]bool{}
	for _, v := range vevents {
		if rid, ok := v.first("RECURRENCE-ID"); ok {
			if t, _, err := parseICalTime(rid, calTZ); err == nil {
				uid := v.text("UID")
				if overridden[uid] == nil {
					overridden[uid] = map[int64]bool{}
				}
				overridden[uid][t.Unix()] = true
			}
		}
	}

	// Plain events (no RRULE, no RECURRENCE-ID) sharing one UID: against
	// the spec, but some exporters (e.g. the Land Berlin event calendar)
	// write every date of a series as its own VEVENT with the series' UID.
	// Those get per-date ids below, or Dedupe would keep only one of them.
	plainUIDs := map[string]int{}
	for _, v := range vevents {
		_, isOverride := v.first("RECURRENCE-ID")
		_, hasRule := v.first("RRULE")
		if !isOverride && !hasRule {
			plainUIDs[v.text("UID")]++
		}
	}

	var events []model.Event
	for _, v := range vevents {
		dtstart, ok := v.first("DTSTART")
		if !ok {
			continue
		}
		start, allDay, err := parseICalTime(dtstart, calTZ)
		if err != nil {
			log.Printf("source %q: skipping event %q: DTSTART: %v", cfg.Name, v.text("SUMMARY"), err)
			continue
		}

		var length time.Duration
		if dtend, ok := v.first("DTEND"); ok {
			if end, _, err := parseICalTime(dtend, calTZ); err == nil {
				length = end.Sub(start)
			}
		} else if dur, ok := v.first("DURATION"); ok {
			if d, ok := parseDuration(dur.value); ok {
				length = d
			}
		}

		uid := v.text("UID")
		rid, isOverride := v.first("RECURRENCE-ID")
		rr, hasRule := v.first("RRULE")
		recurring := hasRule && !isOverride

		starts := []time.Time{start}
		if recurring {
			starts = expandRRule(start, rr.value, exdates(v, calTZ), to)
		}

		for _, occ := range starts {
			if occ.Add(length).Before(from) || !occ.Before(to) {
				continue
			}
			if recurring && overridden[uid][occ.Unix()] {
				continue // replaced by its own RECURRENCE-ID event
			}
			ev := buildICalEvent(v, cfg, occ, length, allDay, now)
			ev.Cancelled = strings.EqualFold(strings.TrimSpace(v.text("STATUS")), "CANCELLED")
			// Stable per-occurrence ids; an override keeps the id of the
			// occurrence it replaces.
			switch {
			case recurring:
				ev.Source.SourceID = uid + "|" + occ.UTC().Format("20060102T150405")
			case isOverride:
				if t, _, err := parseICalTime(rid, calTZ); err == nil {
					ev.Source.SourceID = uid + "|" + t.UTC().Format("20060102T150405")
				}
			case plainUIDs[uid] > 1:
				ev.Source.SourceID = uid + "|" + occ.UTC().Format("20060102T150405")
			}
			events = append(events, ev)
		}
	}
	return events, nil
}

func exdates(v vevent, calTZ string) map[int64]bool {
	out := map[int64]bool{}
	for _, p := range v.props["EXDATE"] {
		for _, val := range strings.Split(p.value, ",") {
			q := p
			q.value = val
			if t, _, err := parseICalTime(q, calTZ); err == nil {
				out[t.Unix()] = true
			}
		}
	}
	return out
}

func buildICalEvent(v vevent, cfg config.SourceConfig, start time.Time, length time.Duration, allDay bool, now time.Time) model.Event {
	ev := model.Event{
		Title:          mapping.CleanText(v.text("SUMMARY")),
		Description:    mapping.CleanMultiline(v.text("DESCRIPTION")),
		URL:            strings.TrimSpace(v.text("URL")),
		Image:          icalImage(v),
		Organizer:      cfg.Organizer,
		Price:          model.Price{Free: false, Amount: nil},
		FamilyFriendly: cfg.FamilyFriendly,
		Source:         model.SourceRef{Name: cfg.Name, SourceID: v.text("UID")},
		LastSeen:       now,
	}
	if cats := v.text("CATEGORIES"); cats != "" {
		ev.Category = mapping.CleanText(strings.SplitN(cats, ",", 2)[0])
	}
	if org, ok := v.first("ORGANIZER"); ok && org.params["CN"] != "" {
		ev.Organizer = mapping.CleanText(org.params["CN"])
	}

	// Location: Apple's structured location carries name and address
	// separately; plain LOCATION is one free-text string.
	if apple, ok := v.first("X-APPLE-STRUCTURED-LOCATION"); ok && apple.params["X-TITLE"] != "" {
		ev.Location = model.Location{
			Name:    mapping.CleanText(unescapeText(apple.params["X-TITLE"])),
			Address: mapping.CleanText(unescapeText(apple.params["X-ADDRESS"])),
		}
	} else if loc := mapping.CleanText(v.text("LOCATION")); loc != "" {
		ev.Location = model.Location{Name: loc}
	} else {
		ev.Location = model.Location{Name: cfg.LocationDefault}
	}

	if allDay {
		ev.Date = start.Format("2006-01-02")
		return ev
	}
	ev.Date, ev.StartTime = model.LocalParts(start)
	// Some exporters (e.g. Events Manager) set DTEND == DTSTART when there's
	// no real end time: treat a zero/negative length as "no end".
	if length > 0 {
		end := start.Add(length)
		endDate, endTime := model.LocalParts(end)
		if endDate == ev.Date {
			ev.EndTime = endTime
		}
	}
	return ev
}

// icalImage returns the URL of the event's image: the first IMAGE property
// (RFC 7986) given as a URI, else the first ATTACH whose FMTTYPE is an
// image. Inline (base64) images are skipped; the image is only referenced,
// never copied.
func icalImage(v vevent) string {
	for _, p := range v.props["IMAGE"] {
		if !strings.EqualFold(p.params["ENCODING"], "BASE64") && !strings.EqualFold(p.params["VALUE"], "BINARY") {
			return strings.TrimSpace(p.value)
		}
	}
	for _, p := range v.props["ATTACH"] {
		if strings.HasPrefix(strings.ToLower(p.params["FMTTYPE"]), "image/") && !strings.EqualFold(p.params["ENCODING"], "BASE64") {
			return strings.TrimSpace(p.value)
		}
	}
	return ""
}

// ---- RRULE ----

var weekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// expandRRule returns the occurrence starts of a recurring event before `to`
// (all of them from DTSTART on, so COUNT is honoured; the caller drops the
// past ones). Unsupported rule parts make it fall back to the
// first occurrence only, with a log line.
func expandRRule(start time.Time, rule string, ex map[int64]bool, to time.Time) []time.Time {
	parts := map[string]string{}
	for _, kv := range strings.Split(rule, ";") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			parts[strings.ToUpper(k)] = strings.ToUpper(v)
		}
	}
	freq := parts["FREQ"]
	interval := 1
	if n, err := strconv.Atoi(parts["INTERVAL"]); err == nil && n > 0 {
		interval = n
	}
	count := -1
	if n, err := strconv.Atoi(parts["COUNT"]); err == nil && n > 0 {
		count = n
	}
	until := to
	if u := parts["UNTIL"]; u != "" {
		if t, _, err := parseICalTime(icalProp{value: u, params: map[string]string{}}, ""); err == nil {
			if len(u) == 8 {
				t = t.AddDate(0, 0, 1) // date-only UNTIL is inclusive
			} else {
				t = t.Add(time.Second)
			}
			if t.Before(until) {
				until = t
			}
		}
	}
	for k := range parts {
		switch k {
		case "FREQ", "INTERVAL", "COUNT", "UNTIL", "WKST":
		case "BYDAY":
			if freq != "WEEKLY" {
				log.Printf("ical: RRULE %q not supported (BYDAY only with WEEKLY), using first occurrence only", rule)
				return []time.Time{start}
			}
		default:
			log.Printf("ical: RRULE part %s not supported in %q, using first occurrence only", k, rule)
			return []time.Time{start}
		}
	}

	var byDay []time.Weekday
	if freq == "WEEKLY" && parts["BYDAY"] != "" {
		for _, d := range strings.Split(parts["BYDAY"], ",") {
			wd, ok := weekdays[strings.TrimLeft(d, "+-0123456789")]
			if !ok {
				log.Printf("ical: RRULE BYDAY %q not supported, using first occurrence only", d)
				return []time.Time{start}
			}
			byDay = append(byDay, wd)
		}
		sort.Slice(byDay, func(i, j int) bool {
			return (byDay[i]+6)%7 < (byDay[j]+6)%7 // Monday first
		})
	}

	var out []time.Time
	emitted := 0
	add := func(t time.Time) bool {
		if !t.Before(until) || emitted >= maxOccurrences || (count >= 0 && emitted >= count) {
			return false
		}
		if t.Before(start) {
			return true
		}
		emitted++
		if !ex[t.Unix()] {
			out = append(out, t)
		}
		return true
	}

	switch freq {
	case "DAILY":
		for i := 0; ; i++ {
			if !add(start.AddDate(0, 0, i*interval)) {
				break
			}
		}
	case "WEEKLY":
		if len(byDay) == 0 {
			for i := 0; ; i++ {
				if !add(start.AddDate(0, 0, 7*i*interval)) {
					break
				}
			}
			break
		}
		// Start of the week (Monday) containing DTSTART.
		weekStart := start.AddDate(0, 0, -((int(start.Weekday()) + 6) % 7))
	weeks:
		for w := 0; ; w++ {
			base := weekStart.AddDate(0, 0, 7*w*interval)
			for _, wd := range byDay {
				t := base.AddDate(0, 0, (int(wd)+6)%7)
				if !add(t) {
					break weeks
				}
			}
		}
	case "MONTHLY":
		for i := 0; ; i++ {
			t := start.AddDate(0, i*interval, 0)
			if t.Day() != start.Day() {
				continue // e.g. the 31st in a 30-day month: skipped, like RFC 5545
			}
			if !add(t) {
				break
			}
			if i > 12*50 {
				break
			}
		}
	case "YEARLY":
		for i := 0; ; i++ {
			if !add(start.AddDate(i*interval, 0, 0)) {
				break
			}
			if i > 200 {
				break
			}
		}
	default:
		log.Printf("ical: RRULE FREQ %q not supported, using first occurrence only", freq)
		return []time.Time{start}
	}
	return out
}
