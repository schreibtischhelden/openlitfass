// Command fetch runs every source configured in an instance directory,
// merges the results and writes <instance>/data/events.json plus the
// derived <instance>/data/categories.json category/icon mapping alongside
// it. Intended to be run by the instance's fetch workflow, but works the
// same locally:
//
//	go run ./cmd/fetch ../path/to/instance     (or $INSTANCE_DIR, else ".")
//
// Side effects inside the instance directory, by design: every source file
// in sources/ gets its "status" field updated, and inbox files whose event
// date has passed are deleted (see internal/sources/inbox.go).
package main

import (
	"context"
	"log"
	"os"
	"time"

	"openlitfass/backend/internal/categories"
	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
	"openlitfass/backend/internal/sources"
	"openlitfass/backend/internal/writer"
)

func main() {
	inst, err := instance.Load(instance.Dir(os.Args[1:]))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}
	if err := model.SetTimezone(inst.Config.Timezone); err != nil {
		log.Fatalf("instance timezone %q: %v", inst.Config.Timezone, err)
	}
	log.Printf("instance %q (%s), timezone %s", inst.Config.Name, inst.Dir, model.Local)

	// A generous backstop, not the primary time limit - each source already
	// gets its own bounded timeout (see sources.harden/defaultSourceTimeout),
	// so one slow source can't eat this whole budget. This just caps the
	// worst case of many sources run sequentially.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	outPath := inst.EventsOutput()
	// Always the sibling of outPath - the two files describe the same run
	// and must never drift apart (see writer.WriteCategories doc comment).
	categoriesPath := inst.CategoriesOutput()

	// The hand-edited registry the generated categories.json is built from.
	// Loaded and validated up front, before spending time fetching any
	// source, so a typo in this file fails fast instead of only surfacing
	// after every source has already run.
	categoryConfig, err := categories.Load(inst.CategoryRegistry())
	if err != nil {
		log.Fatalf("load category registry: %v", err)
	}

	all := sources.All(inst.SourcesDir(), inst.InboxDir())
	if len(all) == 0 {
		log.Printf("no sources configured; leaving %s untouched", outPath)
		return
	}

	var events []model.Event
	var failed int
	failedNames := map[string]bool{}

	for _, src := range all {
		// Every src here already went through sources.harden(): a panic or a
		// hang inside one source surfaces as an ordinary error below, it
		// can't crash this loop or block the rest indefinitely.
		got, err := src.Fetch(ctx)
		if err != nil {
			log.Printf("source %q failed: %v", src.Name(), err)
			failed++
			failedNames[src.Name()] = true
			continue
		}
		log.Printf("source %q: %d events", src.Name(), len(got))
		events = append(events, got...)
	}

	// A failed source keeps its upcoming events from the last successful
	// run instead of vanishing from the calendar (see writer.CarryOver).
	if len(failedNames) > 0 && len(events) > 0 {
		prev, err := writer.LoadPrevious(outPath)
		if err != nil {
			log.Printf("warning: could not read previous %s, failed sources lose their events this run: %v", outPath, err)
		}
		today, _ := model.LocalParts(time.Now())
		for name := range failedNames {
			carried := writer.CarryOver(prev, map[string]bool{name: true}, today)
			if len(carried) > 0 {
				log.Printf("source %q: kept %d event(s) from the previous run", name, len(carried))
				events = append(events, carried...)
			}
		}
	}

	if len(events) == 0 {
		log.Printf("all %d source(s) failed or returned nothing; leaving %s untouched", len(all), outPath)
		// Only a failure if something actually failed - a fresh instance
		// with no active sources and an empty inbox is a valid state, not a
		// broken CI run.
		if failed > 0 {
			os.Exit(1)
		}
		return
	}

	final, err := writer.Write(outPath, events, inst.Config.BaseURL, sourcePriorities(inst))
	if err != nil {
		log.Fatalf("write output: %v", err)
	}
	log.Printf("wrote %d events to %s", len(final), outPath)

	if err := writer.WriteCategories(categoriesPath, categoryConfig, final); err != nil {
		// Load-bearing, not best-effort: the frontend's category filter menu
		// and event-card icons depend on this file being in sync with
		// events.json, so a failure here fails the run rather than leaving a
		// stale/missing categories.json next to a freshly updated events.json.
		log.Fatalf("write categories: %v", err)
	}
	log.Printf("wrote category/icon mapping to %s", categoriesPath)

	// Public source list for the frontend ("Über" page, "Quelle: ..." in the
	// event detail). Re-read after the run so lastSuccess reflects this
	// run's status updates. Best-effort: the calendar works without it.
	if err := writer.WriteSources(inst.Path("data", "sources.json"), sourceInfos(inst)); err != nil {
		log.Printf("warning: write sources: %v", err)
	}

	if failed > 0 {
		log.Printf("warning: %d of %d source(s) failed this run", failed, len(all))
	}
}

// sourcePriorities maps source name -> "priority" from its source file, for
// choosing the shown version when several sources report the same event.
func sourcePriorities(inst instance.Instance) map[string]int {
	configs, _ := config.Load(inst.SourcesDir())
	p := make(map[string]int, len(configs))
	for _, c := range configs {
		p[c.Name] = c.Priority
	}
	return p
}

// sourceInfos lists every enabled configured source plus the inbox, in the
// public shape of data/sources.json.
func sourceInfos(inst instance.Instance) []writer.SourceInfo {
	configs, err := config.Load(inst.SourcesDir())
	if err != nil {
		log.Printf("warning: reload sources for sources.json: %v", err)
	}
	infos := make([]writer.SourceInfo, 0, len(configs)+1)
	for _, c := range configs {
		if c.Disabled {
			continue
		}
		info := writer.SourceInfo{Name: c.Name, Label: c.Label, Homepage: c.Homepage}
		if info.Label == "" {
			info.Label = c.Name
		}
		if c.Status != nil {
			info.LastSuccess = c.Status.LastSuccess
		}
		infos = append(infos, info)
	}
	infos = append(infos, writer.SourceInfo{Name: "inbox", Label: "Von Hand eingetragen"})
	return infos
}
