package writer

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"openlitfass/backend/internal/model"
)

// LoadPrevious reads the events of the last successful run from path
// (data/events.json). A missing file is not an error (first run).
func LoadPrevious(path string) ([]model.Event, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out Output
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out.Events, nil
}

// CarryOver returns the not-yet-past events of the failed sources from the
// previous run. data/events.json is recomputed from scratch every run, so
// without this a single failed fetch (a slow or briefly unreachable site)
// would make all events of that source disappear until the next run
// succeeds. The source's status still records the error.
//
// Only events whose shown version came from the failed source are carried
// (prev.SourceName); merged events still reported by a working source come
// back through that source anyway. Carried events keep their public id.
func CarryOver(prev []model.Event, failed map[string]bool, today string) []model.Event {
	var out []model.Event
	for _, e := range prev {
		if !failed[e.SourceName] || e.Date < today {
			continue
		}
		e.Source = model.SourceRef{Name: e.SourceName, SourceID: "carried|" + e.ID}
		e.Sources = nil // recomputed by MergeAcrossSources
		out = append(out, e)
	}
	return out
}
