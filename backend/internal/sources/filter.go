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

	cats := newSourceCategories(cfg)

	var allowedCategories map[string]bool
	if len(cfg.Categories) > 0 {
		log.Printf(`source %q: "categories" is deprecated, use "categoryFilter"`, cfg.Name)
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
		if !cats.apply(&e) {
			continue
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

// sourceCategories is the compiled category handling of one source: split
// by categorySeparator, categoryFilter.deny/allow, categoryAttributes,
// categoryFilter.include/exclude, categoryMap. All names are compared by
// categoryKey.
type sourceCategories struct {
	separator   string
	categoryMap map[string]string
	deny, allow map[string]bool
	include     map[string]bool
	exclude     map[string]bool
	attributes  map[string]config.CategoryAttributes
}

func categoryKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func categoryKeySet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[categoryKey(n)] = true
	}
	return set
}

func newSourceCategories(cfg config.SourceConfig) sourceCategories {
	sc := sourceCategories{separator: cfg.CategorySeparator}
	if len(cfg.CategoryMap) > 0 {
		sc.categoryMap = make(map[string]string, len(cfg.CategoryMap))
		for k, v := range cfg.CategoryMap {
			sc.categoryMap[categoryKey(k)] = v
		}
	}
	if f := cfg.CategoryFilter; f != nil {
		sc.deny = categoryKeySet(f.Deny)
		sc.allow = categoryKeySet(f.Allow)
		sc.include = categoryKeySet(f.Include)
		sc.exclude = categoryKeySet(f.Exclude)
	}
	if len(cfg.CategoryAttributes) > 0 {
		sc.attributes = make(map[string]config.CategoryAttributes, len(cfg.CategoryAttributes))
		for k, v := range cfg.CategoryAttributes {
			sc.attributes[categoryKey(k)] = v
		}
	}
	return sc
}

// split returns the source's own categories of e: e.Categories if the
// source set them, else e.Category split by the separator. Trimmed, empty
// ones dropped, duplicates removed.
func (sc sourceCategories) split(e model.Event) []string {
	raw := e.Categories
	if len(raw) == 0 && e.Category != "" {
		raw = []string{e.Category}
		if sc.separator != "" {
			raw = strings.Split(e.Category, sc.separator)
		}
	}
	return uniqueCategories(raw)
}

func uniqueCategories(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[categoryKey(n)] {
			continue
		}
		seen[categoryKey(n)] = true
		out = append(out, n)
	}
	return out
}

// anyIn reports whether one of names is in set; no names counts as "".
func anyIn(set map[string]bool, names []string) bool {
	if len(names) == 0 {
		return set[""]
	}
	for _, n := range names {
		if set[categoryKey(n)] {
			return true
		}
	}
	return false
}

// apply sets e.Category, e.Categories and the category attributes of e. It
// returns false if categoryFilter.deny drops the event.
func (sc sourceCategories) apply(e *model.Event) bool {
	names := sc.split(*e)
	if anyIn(sc.deny, names) && !anyIn(sc.allow, names) {
		return false
	}
	for _, n := range names {
		if sc.attributes[categoryKey(n)].FamilyFriendly {
			e.FamilyFriendly = true
		}
	}

	kept := make([]string, 0, len(names))
	main := ""
	for _, n := range names {
		k := categoryKey(n)
		if (sc.include != nil && !sc.include[k]) || sc.exclude[k] {
			continue
		}
		if mapped, ok := sc.categoryMap[k]; ok {
			n = mapped
			if main == "" {
				main = mapped
			}
		}
		kept = append(kept, n)
	}
	kept = uniqueCategories(kept)
	if main == "" && len(kept) > 0 {
		main = kept[0]
	}
	e.Category = main
	e.Categories = nil
	if len(kept) > 0 {
		e.Categories = kept
	}
	return true
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
		case "title", "location", "organizer", "description":
		case "category":
			log.Printf(`source %q: %s: field "category" is deprecated, use "categoryFilter"`, source, kind)
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
