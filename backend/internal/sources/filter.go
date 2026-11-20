package sources

import (
	"context"
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

	kept := make([]model.Event, 0, len(events))
	for _, e := range events {
		if e.Date != "" && e.Date < today {
			continue // already past - always dropped, not configurable
		}
		if maxDate != "" && e.Date > maxDate {
			continue // beyond the configured daysAhead window
		}
		markCancelledByTitle(&e)
		if mapped, ok := categoryMap[strings.ToLower(strings.TrimSpace(e.Category))]; ok {
			e.Category = mapped
		}
		if allowedCategories != nil && !allowedCategories[e.Category] {
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
