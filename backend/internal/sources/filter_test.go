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
		title     string
		cancelled bool
	}{
		{"ABGESAGT: Lesung im Hof", true},
		{"Abgesagt! Konzert", true},
		{"+++ Entfällt +++ Repair-Café", true},
		{"[abgesagt] Flohmarkt", true},
		{"Konzert (abgesagt)", true},
		{"Konzert – ABGESAGT", true},
		{"Kinderdisco - fällt aus!", true},
		{"ENTFÄLLT - Tanztee für alle", true},
		{"ABGESAGT Sommerfest", true},
		{"Cancelled: Jazz Night", true},
		{"Neu-Zeit: Dr. Neu trifft Gabriele Gysi [VERSCHOBEN!]", true}, // old slot of a postponed event
		{"Lesung (verschoben)", true},
		// not interpreted
		{"SONNTAGSSCHÖN [Achtung: Terminverschiebung!]", false},
		{"FIL - MIT NEUER SHOW - AUSVERKAUFT!", false},
		{"Rudelsingen [AUSVERKAUFT!]", false},
		{"Talkshow [ZUSATZTERMIN]", false},
		{"Abgesagt – ein Theaterstück über Absagen", false},
		{"Die abgesagte Party", false},
		{"Das verschobene Fest", false},
		{"Lesung", false},
	}
	for _, tt := range tests {
		e := model.Event{Title: tt.title}
		markCancelledByTitle(&e)
		if e.Cancelled != tt.cancelled {
			t.Errorf("%q -> cancelled=%v, want %v", tt.title, e.Cancelled, tt.cancelled)
		}
		if e.Title != tt.title {
			t.Errorf("title must stay unchanged: %q -> %q", tt.title, e.Title)
		}
	}
}
