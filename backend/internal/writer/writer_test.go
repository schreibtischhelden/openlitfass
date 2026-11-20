package writer

import (
	"path/filepath"
	"testing"
	"time"

	"openlitfass/backend/internal/model"
)

func TestWriteDerivesIDsForSourcesWithout(t *testing.T) {
	now := time.Now()
	day := now.Format("2006-01-02")
	evs := []model.Event{
		{Title: "A", Date: day, StartTime: "10:00", Source: model.SourceRef{Name: "html"}, LastSeen: now},
		{Title: "B", Date: day, StartTime: "12:00", Source: model.SourceRef{Name: "html"}, LastSeen: now},
		{Title: "A", Date: day, StartTime: "10:00", Source: model.SourceRef{Name: "html"}, LastSeen: now}, // real duplicate
	}
	out, err := Write(filepath.Join(t.TempDir(), "events.json"), evs, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 events (A, B; duplicate A merged), got %d", len(out))
	}
	if out[0].ID == out[1].ID {
		t.Errorf("distinct events got the same id %q", out[0].ID)
	}

	// Stable across runs.
	again, _ := Write(filepath.Join(t.TempDir(), "events.json"), []model.Event{
		{Title: "A", Date: day, StartTime: "10:00", Source: model.SourceRef{Name: "html"}, LastSeen: now},
	}, "")
	if again[0].ID != out[0].ID {
		t.Errorf("id not stable: %q vs %q", again[0].ID, out[0].ID)
	}
}
