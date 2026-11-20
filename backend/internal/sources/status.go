package sources

import (
	"context"
	"log"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

// withStatusTracking wraps inner so that every Fetch attempt - success or
// failure - is written back into the source's own config file at path via
// config.RecordAttempt. This is what makes a silently-broken source (a
// website redesign breaking our mapping, a moved feed URL, ...) visible
// directly in the source file itself (lastAttempt/lastSuccess/lastError)
// instead of only in a CI log that scrolls away, and lets that same file be
// hand-edited (see "disabled") to turn the source off without deleting it.
//
// It's the outermost wrapper (see All()) so it records the final outcome
// after common filters, timeout and panic recovery have all had a chance to
// run - a timeout or a recovered panic is exactly the kind of failure this
// is meant to surface.
func withStatusTracking(inner Source, path string) Source {
	return &statusTrackingSource{inner: inner, path: path}
}

type statusTrackingSource struct {
	inner Source
	path  string
}

func (s *statusTrackingSource) Name() string { return s.inner.Name() }

func (s *statusTrackingSource) Fetch(ctx context.Context) ([]model.Event, error) {
	events, err := s.inner.Fetch(ctx)
	if recordErr := config.RecordAttempt(s.path, err); recordErr != nil {
		// Failing to write status is never fatal for the run itself - the
		// fetched events (or the original fetch error) still take
		// precedence. Just log it so a broken/read-only source file is
		// still noticed.
		log.Printf("source %q: recording status in %s failed: %v", s.Name(), s.path, recordErr)
	}
	return events, err
}
