package sources

import (
	"testing"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

func TestCommonFiltersIncludeExclude(t *testing.T) {
	cfg := config.SourceConfig{
		Name:    "t",
		Include: []config.MatchRule{{Field: "location", Regex: `(?i)weißensee|1308[68]`}},
		Exclude: []config.MatchRule{{Field: "title", Regex: `(?i)bewerbung`}},
	}
	evs := []model.Event{
		{Title: "Ausstellung", Date: day(1), Location: model.Location{Name: "Kunsthalle", Address: "Bühringstraße 20, 13086 Berlin"}},
		{Title: "Film", Date: day(1), Location: model.Location{Name: "Kinemathek", Address: "Mauerstraße 79, 10117 Berlin"}},
		{Title: "Ohne Ort", Date: day(1)},
		{Title: "Bewerbungsfrist", Date: day(1), Location: model.Location{Name: "Campus Weißensee"}},
	}
	got := applyCommonFilters(evs, cfg)
	if len(got) != 1 || got[0].Title != "Ausstellung" {
		t.Errorf("kept %+v, want only Ausstellung", got)
	}

	// Invalid rules are skipped; an include list without valid rules keeps all.
	bad := config.SourceConfig{Name: "t", Include: []config.MatchRule{{Field: "ort", Regex: "x"}, {Field: "title", Regex: "("}}}
	if got := applyCommonFilters(evs, bad); len(got) != len(evs) {
		t.Errorf("invalid include rules: kept %d of %d", len(got), len(evs))
	}
}
