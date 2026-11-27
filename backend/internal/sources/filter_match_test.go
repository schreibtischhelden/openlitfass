package sources

import (
	"strings"
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

func TestCommonFiltersCategoryList(t *testing.T) {
	// Arrange
	cfg := config.SourceConfig{
		Name:              "t",
		CategorySeparator: ", ",
		CategoryMap:       map[string]string{"Führungen & Touren": "Führungen", "Konzert": "Musik"},
		CategoryFilter: &config.CategoryFilter{
			Deny:    []string{"ausstellung"},
			Allow:   []string{"Führungen & Touren"},
			Exclude: []string{"Ausstellung"},
		},
		CategoryAttributes: map[string]config.CategoryAttributes{"kinder": {FamilyFriendly: true}},
	}
	evs := []model.Event{
		{Title: "exhibition", Category: "Ausstellung", Date: day(1)},
		{Title: "exhibition tour", Category: "Ausstellung, Führungen & Touren", Date: day(1)},
		{Title: "party", Category: "Tanz,Party & Disco, Konzert", Date: day(1)},
		{Title: "kids", Category: "Kinder, Theater, Kinder", Date: day(1)},
		{Title: "none", Date: day(1)},
	}

	// Act
	got := applyCommonFilters(evs, cfg)

	// Assert
	want := []struct {
		title, main, list string
		family            bool
	}{
		{"exhibition tour", "Führungen", "Führungen", false},
		{"party", "Musik", "Tanz,Party & Disco|Musik", false},
		{"kids", "Kinder", "Kinder|Theater", true},
		{"none", "", "", false},
	}
	if len(got) != len(want) {
		t.Fatalf("kept %+v, want %d events", got, len(want))
	}
	for i, w := range want {
		e := got[i]
		if e.Title != w.title || e.Category != w.main || strings.Join(e.Categories, "|") != w.list || e.FamilyFriendly != w.family {
			t.Errorf("event %d = %q %q %v family=%v, want %q %q %q family=%v",
				i, e.Title, e.Category, e.Categories, e.FamilyFriendly, w.title, w.main, w.list, w.family)
		}
	}
}

func TestCommonFiltersCategoryFilterIncludeAndEmpty(t *testing.T) {
	// Arrange
	cfg := config.SourceConfig{
		Name:              "t",
		CategorySeparator: ", ",
		CategoryFilter:    &config.CategoryFilter{Include: []string{"Lesung"}},
	}
	evs := []model.Event{
		{Title: "reading", Category: "Bühne, Lesung", Date: day(1)},
		{Title: "stage", Category: "Bühne", Date: day(1)},
	}

	// Act
	kept := applyCommonFilters(evs, cfg)
	cfg.CategoryFilter.Deny = []string{""}
	denied := applyCommonFilters(evs, cfg)

	// Assert
	if len(kept) != 2 || kept[0].Category != "Lesung" || kept[1].Category != "" || kept[1].Categories != nil {
		t.Errorf("include: kept %+v, want reading/Lesung and stage without category", kept)
	}
	if len(denied) != 2 {
		t.Errorf(`deny "" drops events the source gave no category, not ones include emptied: kept %+v`, denied)
	}
	none := applyCommonFilters([]model.Event{{Title: "none", Date: day(1)}}, cfg)
	if len(none) != 0 {
		t.Errorf(`deny "": kept %+v, want the event without categories dropped`, none)
	}
}

func TestCommonFiltersWithoutSeparatorKeepsOneCategory(t *testing.T) {
	// Arrange
	cfg := config.SourceConfig{Name: "t", CategoryMap: map[string]string{"kino": "Kultur"}}
	evs := []model.Event{
		{Title: "film", Category: "Kino", Date: day(1)},
		{Title: "list", Category: "A, B", Date: day(1)},
	}

	// Act
	got := applyCommonFilters(evs, cfg)

	// Assert
	if got[0].Category != "Kultur" || strings.Join(got[0].Categories, "|") != "Kultur" {
		t.Errorf("film = %q %v, want Kultur [Kultur]", got[0].Category, got[0].Categories)
	}
	if got[1].Category != "A, B" || len(got[1].Categories) != 1 {
		t.Errorf("list = %q %v, want the value kept whole", got[1].Category, got[1].Categories)
	}
}

func TestCommonFiltersTownFromLocationName(t *testing.T) {
	// Arrange
	cfg := config.SourceConfig{Name: "t", Include: []config.MatchRule{{Field: "location", Regex: `(?i),\s*Stralsund\s*$`}}}
	evs := []model.Event{
		{Title: "in town", Date: day(1), Location: model.Location{Name: "Theater Vorpommern, Stralsund"}},
		{Title: "venue name only", Date: day(1), Location: model.Location{Name: "Stralsund-Halle, Greifswald"}},
	}

	// Act
	got := applyCommonFilters(evs, cfg)

	// Assert
	if len(got) != 1 || got[0].Title != "in town" {
		t.Errorf("kept %+v, want only the event whose town is Stralsund", got)
	}
}
