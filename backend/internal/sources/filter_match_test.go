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

func TestCommonFiltersCategoryRules(t *testing.T) {
	cfg := config.SourceConfig{
		Name:        "t",
		CategoryMap: map[string]string{"Extra": "Unkategorisiert", "Bühne": "Kultur"},
		CategoryRules: []config.CategoryRule{
			{Field: "title", Regex: `(?i)workshop|bastel`, Category: "Mitmachen"},
			{Field: "title", Regex: `(?i)(advents|floh)markt\b`, Category: "Markt"},
			{Field: "title", Regex: `(?i)\b(stadt)?führung(en)?\b|rundgang`, Category: "Führungen"},
			{Field: "category", Regex: `^Kultur$`, Category: "Bühne & Kultur"},
		},
		Categories: []string{"Mitmachen", "Markt", "Führungen", "Unkategorisiert", "Bühne & Kultur"},
	}
	evs := []model.Event{
		{Title: "Aquarell-Workshop", Category: "Extra", Date: day(1)},
		{Title: "Flohmarkt am Hafen", Category: "Extra", Date: day(1)},
		{Title: "Stadtführung", Category: "Extra", Date: day(1)},
		{Title: "Aufführung: Faust", Category: "Extra", Date: day(1)},
		{Title: "Konzert", Category: "Bühne", Date: day(1)},
		{Title: "Bastelnachmittag", Category: "Sport", Date: day(1)},
	}

	got := applyCommonFilters(evs, cfg)

	want := []string{"Mitmachen", "Markt", "Führungen", "Unkategorisiert", "Bühne & Kultur", "Mitmachen"}
	if len(got) != len(want) {
		t.Fatalf("kept %d events (%+v), want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].Category != w {
			t.Errorf("%q: category %q, want %q", got[i].Title, got[i].Category, w)
		}
	}
}

func TestCommonFiltersCategoryRulesFirstMatchAndFilters(t *testing.T) {
	cfg := config.SourceConfig{
		Name: "t",
		CategoryRules: []config.CategoryRule{
			{Field: "category", Regex: `^Umland$`, Category: "Umland"},
			{Field: "title", Regex: `(?i)markt`, Category: "Markt"},
		},
		Exclude: []config.MatchRule{{Field: "category", Regex: `^Markt$`}},
	}
	evs := []model.Event{
		{Title: "Wochenmarkt", Category: "Umland", Date: day(1)},
		{Title: "Weihnachtsmarkt", Category: "Extra", Date: day(1)},
		{Title: "Lesung", Category: "Extra", Date: day(1)},
	}

	got := applyCommonFilters(evs, cfg)

	if len(got) != 2 || got[0].Category != "Umland" || got[1].Category != "Extra" {
		t.Errorf("kept %+v, want Wochenmarkt/Umland and Lesung/Extra", got)
	}
}

func TestCommonFiltersInvalidCategoryRulesSkipped(t *testing.T) {
	cfg := config.SourceConfig{
		Name: "t",
		CategoryRules: []config.CategoryRule{
			{Field: "ort", Regex: "x", Category: "A"},
			{Field: "title", Regex: "(", Category: "B"},
			{Field: "title", Regex: "x", Category: " "},
			{Field: "title", Regex: "x", Category: "C"},
		},
	}
	evs := []model.Event{{Title: "x", Category: "Extra", Date: day(1)}}

	got := applyCommonFilters(evs, cfg)

	if len(got) != 1 || got[0].Category != "C" {
		t.Errorf("kept %+v, want category C from the only valid rule", got)
	}
}
