package sources

import (
	"context"
	"fmt"
	"time"

	"openlitfass/backend/internal/model"
)

// defaultSourceTimeout bounds a single source's Fetch, independent of how
// many other sources run in the same process. Without this, a single slow
// or hanging endpoint could consume the entire run's shared context budget
// (see cmd/fetch/main.go) and starve every source queued after it.
const defaultSourceTimeout = 20 * time.Second

// maxSourceTimeout caps SourceConfig.TimeoutSeconds, leaving the other
// sources room within the run's FetchTimeout (internal/app).
const maxSourceTimeout = 180 * time.Second

// sourceTimeout is the time limit for one source: TimeoutSeconds if set
// (capped at maxSourceTimeout), else defaultSourceTimeout.
func sourceTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultSourceTimeout
	}
	return min(time.Duration(seconds)*time.Second, maxSourceTimeout)
}

// withTimeout gives inner its own bounded sub-context, derived from
// whatever context Fetch is called with.
func withTimeout(inner Source, d time.Duration) Source {
	return &timeoutSource{inner: inner, timeout: d}
}

type timeoutSource struct {
	inner   Source
	timeout time.Duration
}

func (t *timeoutSource) Name() string { return t.inner.Name() }

func (t *timeoutSource) Fetch(ctx context.Context) ([]model.Event, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	return t.inner.Fetch(ctx)
}

// withPanicRecovery turns a panic anywhere inside inner.Fetch (a malformed
// response tripping up a parser, an unexpected nil, ...) into an ordinary
// error instead of crashing cmd/fetch outright. Without this, one buggy or
// surprised-by-bad-data source would take the whole run down with it,
// discarding every other source's already-successful results along the way
// (see the per-source error handling in cmd/fetch/main.go, which is
// otherwise resilient to individual sources failing).
func withPanicRecovery(inner Source) Source {
	return &recoveringSource{inner: inner}
}

type recoveringSource struct {
	inner Source
}

func (r *recoveringSource) Name() string { return r.inner.Name() }

func (r *recoveringSource) Fetch(ctx context.Context) (events []model.Event, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return r.inner.Fetch(ctx)
}
