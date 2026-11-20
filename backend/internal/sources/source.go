// Package sources contains one fetcher per event source (iCal feed, venue
// website, API, ...). Each fetcher normalizes its data into model.Event.
package sources

import (
	"context"
	"log"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

// Source is implemented by every event source.
type Source interface {
	// Name is a short, stable identifier used in Event.Source.Name and logs.
	Name() string
	// Fetch retrieves and normalizes events from this source. It should
	// return an error rather than partial/invalid data; the caller logs the
	// error and continues with the remaining sources.
	Fetch(ctx context.Context) ([]model.Event, error)
}

// All loads every *.json file in sourcesDir (<instance>/sources/) and
// builds one Source per entry — adding an RSS or
// JSON source is adding one file to that directory, no Go code needed. See
// <instance>/sources/*.example.json and docs/backend.md.
//
// A missing/empty config directory yields zero sources, which cmd/fetch
// treats as "nothing to do" rather than wiping data/events.json (see
// writer.Write).
//
// A source with "disabled": true in its config file is skipped entirely -
// no fetch attempt, no status write - so a broken or paused source can be
// turned off by editing one field instead of deleting or renaming the file.
//
// Every other source is wrapped, regardless of type, so an individual
// source misbehaving can never take the whole run down or silently return
// nonsense data, and so a broken source is visible without reading CI logs:
//   - withCommonFilters: daysAhead/categories, always drops past events
//     (filter.go)
//   - withTimeout: a bounded per-source deadline so one slow endpoint can't
//     starve the others of the run's shared time budget (safety.go)
//   - withPanicRecovery: a panic inside one source becomes a logged error
//     for that source instead of crashing the process and losing every
//     other source's already-fetched results (safety.go)
//   - withStatusTracking: writes lastAttempt/lastSuccess/lastError back into
//     the source's own config file after every attempt (status.go)
//
// For sources that don't fit the generic RSS/JSON model (iCal, HTML
// scraping, paginated/authenticated APIs, ...), implement Source directly
// (see example.go for a template) and append the instance below.
//
// inboxDir (<instance>/inbox/) holds hand-authored single-event files, see
// docs/inbox.md.
func All(sourcesDir, inboxDir string) []Source {
	dir := sourcesDir

	configs, err := config.Load(dir)
	if err != nil {
		log.Printf("loading %s failed: %v (continuing with zero configured sources)", dir, err)
		configs = nil
	}

	var all []Source
	for _, c := range configs {
		if c.Disabled {
			log.Printf("source %q: disabled, skipping", c.Name)
			continue
		}

		var src Source
		switch c.Type {
		case "rss":
			src = NewRSSSource(c)
		case "json":
			src = NewJSONSource(c)
		case "wcs":
			src = NewWCSSource(c)
		default:
			log.Printf("source %q: unknown type %q, skipping", c.Name, c.Type)
			continue
		}
		wrapped := harden(withCommonFilters(src, c))
		if c.Path != "" {
			wrapped = withStatusTracking(wrapped, c.Path)
		}
		all = append(all, wrapped)
	}

	// Add hand-written sources here, e.g.:
	// all = append(all, harden(withCommonFilters(NewSomeICalSource(...), someConfig)))

	// The inbox: hand-authored single-event JSON files (see
	// docs/inbox.md), the manual counterpart to the fetched
	// sources above. No withStatusTracking - it's a local directory scan,
	// not a network call, so it doesn't fail the way a remote source can;
	// per-file errors are already logged by InboxSource.Fetch itself. Still
	// gets withCommonFilters (with a bare config.SourceConfig, so no
	// daysAhead/categories restriction - only the unconditional "drop past
	// events" rule actually applies) and harden, like every other source.
	all = append(all, harden(withCommonFilters(NewInboxSource(inboxDir), config.SourceConfig{Name: "inbox"})))

	return all
}

// harden applies the two resilience wrappers every source gets, regardless
// of type - see the All() doc comment above.
func harden(src Source) Source {
	return withPanicRecovery(withTimeout(src, defaultSourceTimeout))
}
