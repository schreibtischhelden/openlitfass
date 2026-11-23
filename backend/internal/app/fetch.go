// Package app holds the two operations openLitfass performs on an instance
// - fetching events (Fetch) and assembling the static site (BuildSite) -
// so the commands fetch, build-site and serve share one implementation.
package app

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"openlitfass/backend/internal/categories"
	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
	"openlitfass/backend/internal/sources"
	"openlitfass/backend/internal/writer"
)

// FetchResult summarises one fetch run.
type FetchResult struct {
	Sources   int           // configured sources incl. inbox
	Failed    []string      // names of sources that failed this run
	Events    int           // events written (0 if nothing was written)
	Written   bool          // data/*.json were (re)written
	Duration  time.Duration // whole run
	PerSource []SourceRun   // one entry per source, in run order
}

// SourceRun is the outcome of one source in one fetch run.
type SourceRun struct {
	Name     string
	Events   int // events returned (0 on error)
	Carried  int // events kept from the previous run after an error
	Duration time.Duration
	Err      error
}

// FetchTimeout is a generous backstop for a whole run - each source already
// gets its own bounded timeout (see sources.harden), so one slow source
// can't eat this budget.
const FetchTimeout = 5 * time.Minute

// Fetch runs every source of the instance and writes
// <instance>/data/events.json, categories.json and sources.json.
//
// Side effects inside the instance directory, by design: every source file
// gets its "status" field updated, and inbox files whose date has passed are
// deleted. A source that fails keeps its upcoming events from the previous
// run (writer.CarryOver).
//
// It returns an error only for problems that make the run pointless
// (broken category registry, write failure). "Nothing to write" is not an
// error: Written is false then, and Failed tells whether that's because
// sources failed.
func Fetch(ctx context.Context, inst instance.Instance) (res FetchResult, err error) {
	if err := model.SetTimezone(inst.Config.Timezone); err != nil {
		return res, fmt.Errorf("instance timezone %q: %w", inst.Config.Timezone, err)
	}

	outPath := inst.EventsOutput()
	// Loaded and validated up front, before spending time fetching, so a
	// typo in the registry fails fast.
	categoryConfig, err := categories.Load(inst.CategoryRegistry())
	if err != nil {
		return res, fmt.Errorf("load category registry: %w", err)
	}

	all := sources.All(inst.SourcesDir(), inst.InboxDir(), inst.HolidaysFile())
	res.Sources = len(all)
	if len(all) == 0 {
		log.Printf("no sources configured; leaving %s untouched", outPath)
		return res, nil
	}

	ctx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()

	started := time.Now()
	defer func() { res.Duration = time.Since(started) }()

	var events []model.Event
	failed := map[string]bool{}
	for _, src := range all {
		// Every src went through sources.harden(): a panic or a hang inside
		// one source surfaces as an ordinary error here.
		t0 := time.Now()
		got, err := src.Fetch(ctx)
		res.PerSource = append(res.PerSource, SourceRun{Name: src.Name(), Events: len(got), Duration: time.Since(t0), Err: err})
		if err != nil {
			log.Printf("source %q failed: %v", src.Name(), err)
			failed[src.Name()] = true
			continue
		}
		log.Printf("source %q: %d events", src.Name(), len(got))
		events = append(events, got...)
	}
	for name := range failed {
		res.Failed = append(res.Failed, name)
	}
	sort.Strings(res.Failed)

	// A failed source keeps its upcoming events from the last successful
	// run instead of vanishing from the calendar.
	if len(failed) > 0 && len(events) > 0 {
		prev, err := writer.LoadPrevious(outPath)
		if err != nil {
			log.Printf("warning: could not read previous %s, failed sources lose their events this run: %v", outPath, err)
		}
		today, _ := model.LocalParts(time.Now())
		for _, name := range res.Failed {
			carried := writer.CarryOver(prev, map[string]bool{name: true}, today)
			for i := range res.PerSource {
				if res.PerSource[i].Name == name {
					res.PerSource[i].Carried = len(carried)
				}
			}
			if len(carried) > 0 {
				log.Printf("source %q: kept %d event(s) from the previous run", name, len(carried))
				events = append(events, carried...)
			}
		}
	}

	if len(events) == 0 {
		log.Printf("all %d source(s) failed or returned nothing; leaving %s untouched", len(all), outPath)
		return res, nil
	}

	final, err := writer.Write(outPath, events, inst.Config.BaseURL, sourcePriorities(inst))
	if err != nil {
		return res, fmt.Errorf("write output: %w", err)
	}
	log.Printf("wrote %d events to %s", len(final), outPath)

	// Load-bearing: the category menu and card icons depend on this file
	// being in sync with events.json.
	if err := writer.WriteCategories(inst.CategoriesOutput(), categoryConfig, final); err != nil {
		return res, fmt.Errorf("write categories: %w", err)
	}

	// Public source list for the frontend. Re-read after the run so
	// lastSuccess reflects this run. Best-effort.
	if err := writer.WriteSources(inst.Path("data", "sources.json"), sourceInfos(inst)); err != nil {
		log.Printf("warning: write sources: %v", err)
	}

	if len(res.Failed) > 0 {
		log.Printf("warning: %d of %d source(s) failed this run: %v", len(res.Failed), len(all), res.Failed)
	}
	res.Events = len(final)
	res.Written = true
	return res, nil
}

// sourcePriorities maps source name -> "priority" from its source file.
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
