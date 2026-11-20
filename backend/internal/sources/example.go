package sources

import (
	"context"
	"time"

	"openlitfass/backend/internal/model"
)

// ExampleSource is a template that demonstrates the Source interface. It is
// NOT registered in All() (see source.go) so it has no effect on real runs.
// Copy this file when wiring up a real source: implement Name() and Fetch(),
// normalize into model.Event, set Source.Name/Source.SourceID (used to
// derive a stable Event.ID and to dedupe), and use model.LocalParts to turn
// a time.Time into the Date/StartTime/EndTime strings the frontend expects.
type ExampleSource struct{}

func NewExampleSource() *ExampleSource {
	return &ExampleSource{}
}

func (s *ExampleSource) Name() string {
	return "example-placeholder"
}

func (s *ExampleSource) Fetch(ctx context.Context) ([]model.Event, error) {
	now := time.Now().UTC()
	start := now.Add(48 * time.Hour).Truncate(time.Hour)
	end := start.Add(2 * time.Hour)

	date, startTime := model.LocalParts(start)
	_, endTime := model.LocalParts(end)

	events := []model.Event{
		{
			Title:       "Beispiel-Event (Platzhalter-Quelle)",
			Description: "Demonstriert das Source-Interface. Durch eine echte Quelle ersetzen.",
			Category:    "Unkategorisiert",
			Date:        date,
			StartTime:   startTime,
			EndTime:     endTime,
			Location: model.Location{
				Name: "Antonplatz",
			},
			Price:     model.Price{Free: true, Amount: nil},
			Organizer: "",
			Source: model.SourceRef{
				Name:     s.Name(),
				SourceID: "example-1",
			},
			LastSeen: now,
		},
	}
	return events, nil
}
