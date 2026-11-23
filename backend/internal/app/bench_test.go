package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
)

// BenchmarkBuildSite builds a whole instance with 500 upcoming events,
// static pages and sitemap included, as `serve` does after every fetch.
// Compare branches with scripts/check.sh bench.
func BenchmarkBuildSite(b *testing.B) {
	webDir, err := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	if err != nil {
		b.Fatal(err)
	}
	root := b.TempDir()
	instDir := filepath.Join(root, "inst")
	if err := os.MkdirAll(filepath.Join(instDir, "data"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instDir, "instance.json"),
		[]byte(`{"name":"Benchkiez","baseUrl":"https://example.org/"}`), 0o644); err != nil {
		b.Fatal(err)
	}

	type event struct {
		ID          string         `json:"id"`
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Category    string         `json:"category"`
		Date        string         `json:"date"`
		StartTime   string         `json:"startTime"`
		Location    map[string]any `json:"location"`
		Price       map[string]any `json:"price"`
	}
	start := time.Now().In(model.Local)
	events := make([]event, 0, 500)
	for i := range 500 {
		events = append(events, event{
			ID:          fmt.Sprintf("event-%d", i),
			Title:       fmt.Sprintf("Veranstaltung %d", i),
			Description: "Ein Abend mit Musik, Lesung und Gesprächen. Mehr: https://example.org/info",
			Category:    "Kultur",
			Date:        start.AddDate(0, 0, 1+i%90).Format("2006-01-02"),
			StartTime:   "19:00",
			Location:    map[string]any{"name": "Hof", "address": "Weg 1, 13086 Berlin"},
			Price:       map[string]any{"free": i%3 == 0, "amount": nil},
		})
	}
	data, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instDir, "data", "events.json"), data, 0o644); err != nil {
		b.Fatal(err)
	}
	inst, err := instance.Load(instDir)
	if err != nil {
		b.Fatal(err)
	}
	out := filepath.Join(root, "out")
	opts := SiteOptions{StaticPages: true, Sitemap: true}

	// BuildSite logs every run; keep the benchmark output readable.
	log.SetOutput(io.Discard)
	b.Cleanup(func() { log.SetOutput(os.Stderr) })

	b.ReportAllocs()
	for b.Loop() {
		if err := BuildSite(inst, webDir, out, opts); err != nil {
			b.Fatal(err)
		}
	}
}
