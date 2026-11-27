package sources

import (
	"context"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

// withCommonFilters wraps any Source so that, regardless of its type
// ("rss", "json", "wcs", or a hand-written one), the same three rules apply
// uniformly after Fetch: already-past events are always dropped, events
// beyond cfg.DaysAhead (if set) are dropped, and events whose Category
// isn't in cfg.Categories (if set) are dropped. This lets every source
// declare these as plain config instead of every fetcher reimplementing
// them (or forgetting to).
func withCommonFilters(inner Source, cfg config.SourceConfig) Source {
	return &filteringSource{inner: inner, cfg: cfg}
}

type filteringSource struct {
	inner Source
	cfg   config.SourceConfig
}

func (f *filteringSource) Name() string { return f.inner.Name() }

func (f *filteringSource) Fetch(ctx context.Context) ([]model.Event, error) {
	events, err := f.inner.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return applyCommonFilters(events, f.cfg), nil
}

func applyCommonFilters(events []model.Event, cfg config.SourceConfig) []model.Event {
	today, _ := model.LocalParts(time.Now())

	var maxDate string
	if cfg.DaysAhead > 0 {
		if t, err := time.ParseInLocation("2006-01-02", today, model.Local); err == nil {
			maxDate = t.AddDate(0, 0, cfg.DaysAhead).Format("2006-01-02")
		}
	}

	var categoryMap map[string]string
	if len(cfg.CategoryMap) > 0 {
		categoryMap = make(map[string]string, len(cfg.CategoryMap))
		for k, v := range cfg.CategoryMap {
			categoryMap[strings.ToLower(strings.TrimSpace(k))] = v
		}
	}

	var allowedCategories map[string]bool
	if len(cfg.Categories) > 0 {
		allowedCategories = make(map[string]bool, len(cfg.Categories))
		for _, c := range cfg.Categories {
			allowedCategories[c] = true
		}
	}

	include := compileMatchRules(cfg.Name, "include", cfg.Include)
	exclude := compileMatchRules(cfg.Name, "exclude", cfg.Exclude)

	kept := make([]model.Event, 0, len(events))
	for _, e := range events {
		if e.Date != "" && e.Date < today {
			continue // already past - always dropped, not configurable
		}
		if maxDate != "" && e.Date > maxDate {
			continue // beyond the configured daysAhead window
		}
		markCancelledByTitle(&e)
		e.Image = keptImage(e.Image, cfg.Images)
		if e.Place == "" {
			e.Place = cfg.Place
		}
		if mapped, ok := categoryMap[strings.ToLower(strings.TrimSpace(e.Category))]; ok {
			e.Category = mapped
		}
		if allowedCategories != nil && !allowedCategories[e.Category] {
			continue
		}
		if len(include) > 0 && !anyRuleMatches(include, e) {
			continue
		}
		if anyRuleMatches(exclude, e) {
			continue
		}
		kept = append(kept, e)
	}
	return kept
}

// Title markers for events that don't take place on this date, as
// organisers commonly write them into the title instead of a status field:
// "ABGESAGT: Lesung", "+++ Entfällt +++ Konzert", "Konzert (abgesagt)",
// "Konzert – ABGESAGT", "Neu-Zeit [VERSCHOBEN!]". A postponed event counts
// as cancelled for this slot - the new date is usually listed separately.
// A word alone at the start is not enough ("Abgesagt – ein Theaterstück"
// could be a title), it needs a separator or brackets.
//
// The title itself is never changed: we take over what the organiser
// wrote and only derive the cancelled flag. Other title hints
// ("ausverkauft", "Zusatztermin", "Terminverschiebung") are deliberately
// not interpreted.
var (
	cancelWords        = `(?:abgesagt|entfällt|entfaellt|fällt aus|faellt aus|ausgefallen|verschoben|cancelled|canceled|postponed)`
	cancelPrefixRe     = regexp.MustCompile(`(?i)^\s*(?:\+{2,3}\s*` + cancelWords + `!?\s*\+{2,3}|[\[(]\s*` + cancelWords + `!?\s*[\])]|` + cancelWords + `\s*[:!])`)
	cancelSuffixRe     = regexp.MustCompile(`(?i)(?:[\[(]\s*` + cancelWords + `!?\s*[\])]|[-–—:]\s*` + cancelWords + `!?|\+{2,3}\s*` + cancelWords + `!?\s*\+{2,3})\s*$`)
	cancelUpperStartRe = regexp.MustCompile(`^\s*(?:ABGESAGT|ENTFÄLLT|FÄLLT AUS|VERSCHOBEN|CANCELLED|CANCELED|POSTPONED)\s+[-–—]?\s*\S`)
)

// markCancelledByTitle sets e.Cancelled if the title carries one of the
// markers above. The title stays exactly as the source wrote it.
func markCancelledByTitle(e *model.Event) {
	for _, re := range []*regexp.Regexp{cancelPrefixRe, cancelSuffixRe, cancelUpperStartRe} {
		if re.MatchString(e.Title) {
			e.Cancelled = true
			return
		}
	}
}

// keptImage returns raw if the source allows images and raw is an absolute
// https URL with a host, "" otherwise. Plain http would be blocked or
// rewritten as mixed content on an https site, and anything else (data:,
// javascript:, relative paths) has no business in og:image or an <img>.
func keptImage(raw string, allowed bool) string {
	if !allowed || raw == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

type matchRule struct {
	field string
	re    *regexp.Regexp
}

// compileMatchRules compiles include/exclude rules; invalid ones are logged
// and skipped (an include list that ends up empty then keeps everything,
// rather than silently dropping the whole source).
func compileMatchRules(source, kind string, rules []config.MatchRule) []matchRule {
	var out []matchRule
	for _, r := range rules {
		switch r.Field {
		case "title", "location", "organizer", "category", "description":
		default:
			log.Printf("source %q: %s: unknown field %q (title, location, organizer, category, description)", source, kind, r.Field)
			continue
		}
		re, err := regexp.Compile(r.Regex)
		if err != nil {
			log.Printf("source %q: %s: invalid regex %q: %v", source, kind, r.Regex, err)
			continue
		}
		out = append(out, matchRule{field: r.Field, re: re})
	}
	return out
}

func anyRuleMatches(rules []matchRule, e model.Event) bool {
	for _, r := range rules {
		var v string
		switch r.field {
		case "title":
			v = e.Title
		case "location":
			v = e.Location.Name + " " + e.Location.Address
		case "organizer":
			v = e.Organizer
		case "category":
			v = e.Category
		case "description":
			v = e.Description
		}
		if r.re.MatchString(v) {
			return true
		}
	}
	return false
}
