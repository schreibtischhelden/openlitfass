package sources

import (
	"testing"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/model"
)

func TestCommonFiltersCategoryMap(t *testing.T) {
	cfg := config.SourceConfig{
		CategoryMap: map[string]string{"Kino": "Kultur", "kurs": "Kultur"},
		Categories:  []string{"Kultur"},
		DaysAhead:   30,
	}
	evs := []model.Event{
		{Title: "film", Category: "Kino", Date: day(1)},
		{Title: "kurs", Category: "Kurs", Date: day(2)},     // key matched case-insensitively
		{Title: "sport", Category: "Sport", Date: day(3)},   // not in allowlist after mapping
		{Title: "old", Category: "Kino", Date: day(-1)},     // past
		{Title: "far", Category: "Kino", Date: day(45)},     // beyond window
		{Title: "direct", Category: "Kultur", Date: day(4)}, // already our vocabulary
	}
	got := applyCommonFilters(evs, cfg)
	want := []string{"film", "kurs", "direct"}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i, e := range got {
		if e.Title != want[i] || e.Category != "Kultur" {
			t.Errorf("event %d = %s/%s, want %s/Kultur", i, e.Title, e.Category, want[i])
		}
	}
}

func TestMarkCancelledByTitle(t *testing.T) {
	tests := []struct {
		in        string
		cancelled bool
		title     string
	}{
		{"ABGESAGT: Lesung im Hof", true, "Lesung im Hof"},
		{"Abgesagt! Konzert", true, "Konzert"},
		{"+++ Entfällt +++ Repair-Café", true, "Repair-Café"},
		{"[abgesagt] Flohmarkt", true, "Flohmarkt"},
		{"Konzert (abgesagt)", true, "Konzert"},
		{"Konzert – ABGESAGT", true, "Konzert"},
		{"Kinderdisco - fällt aus!", true, "Kinderdisco"},
		{"ABGESAGT Sommerfest", true, "Sommerfest"},
		{"Cancelled: Jazz Night", true, "Jazz Night"},
		// no marker / ambiguous - unchanged
		{"Abgesagt – ein Theaterstück über Absagen", false, "Abgesagt – ein Theaterstück über Absagen"},
		{"Die abgesagte Party", false, "Die abgesagte Party"},
		{"Lesung", false, "Lesung"},
		{"ABGESAGT", false, "ABGESAGT"},
	}
	for _, tt := range tests {
		e := model.Event{Title: tt.in}
		markCancelledByTitle(&e)
		if e.Cancelled != tt.cancelled || e.Title != tt.title {
			t.Errorf("%q -> cancelled=%v title=%q, want %v %q", tt.in, e.Cancelled, e.Title, tt.cancelled, tt.title)
		}
	}
}
