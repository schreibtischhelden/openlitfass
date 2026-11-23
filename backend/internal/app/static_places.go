package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
	"openlitfass/backend/internal/places"
	"openlitfass/backend/internal/writer"
)

// PlacesDir is the output subdirectory of the static place pages.
const PlacesDir = "orte"

// maxPlaceEvents caps the upcoming events listed on a static place page.
const maxPlaceEvents = 30

type staticPlace struct {
	places.Place
	Page     string // file name inside PlacesDir
	TypeName string
	MapURL   string
	Events   []staticEvent // upcoming events at this place
	More     int           // further events not listed
}

type staticPlaceGroup struct {
	Type   string
	Places []staticPlace
}

// writePlaces writes data/places.json for the frontend. Without an instance
// places.json nothing is written and the app hides its places view.
func writePlaces(inst instance.Instance, out string) (*places.File, error) {
	pl, err := places.Load(inst.Path(places.FileName))
	if err != nil || pl == nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(out, "data", "places.json"), pl); err != nil {
		return nil, fmt.Errorf("write places.json: %w", err)
	}
	return pl, nil
}

// buildPlacePages renders orte/index.html and orte/<id>.html (with
// OL_STATIC_PAGES). events are the upcoming events (loadStaticEvents).
func buildPlacePages(inst instance.Instance, webDir, out string, logo template.HTML, pl *places.File, events []staticEvent) error {
	tdir := filepath.Join(webDir, templatesDir)
	if _, err := os.Stat(filepath.Join(tdir, "place.html")); errors.Is(err, fs.ErrNotExist) {
		log.Printf("place pages: no templates in %s, skipped", tdir)
		return nil
	}
	tmpl, err := template.ParseFiles(filepath.Join(tdir, "place.html"), filepath.Join(tdir, "places.html"), filepath.Join(tdir, "layout.html"))
	if err != nil {
		return err
	}
	dir := filepath.Join(out, PlacesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	siteURL := strings.TrimRight(inst.Config.BaseURL, "/")
	abs := func(rel string) string {
		if siteURL == "" {
			return ""
		}
		return siteURL + "/" + rel
	}
	base := staticPage{Config: inst.Config, Logo: logo, SiteURL: siteURL, Root: "../",
		Generated: time.Now().In(model.Local).Format("02.01.2006, 15:04")}
	if base.Title == "" {
		base.Title = base.Name
	}
	base.Imprint = relToRoot(base.Root, inst.Config.ImprintURL)
	base.Privacy = relToRoot(base.Root, inst.Config.PrivacyURL)

	all := make([]staticPlace, 0, len(pl.Places))
	for _, p := range pl.Places {
		sp := staticPlace{Place: p, Page: p.ID + ".html", TypeName: p.Type, MapURL: placeMapURL(p)}
		for _, e := range events {
			if e.PlaceID == p.ID {
				if len(sp.Events) < maxPlaceEvents {
					sp.Events = append(sp.Events, e)
				} else {
					sp.More++
				}
			}
		}
		all = append(all, sp)
	}

	// List page, grouped by type in the order of types[].
	var groups []staticPlaceGroup
	for _, t := range pl.Types {
		g := staticPlaceGroup{Type: t.Name}
		for _, p := range all {
			if p.Type == t.Name {
				g.Places = append(g.Places, p)
			}
		}
		if len(g.Places) > 0 {
			groups = append(groups, g)
		}
	}
	lp := base
	lp.PageTitle = "Orte – " + base.Name
	lp.MetaDesc = "Sehenswürdigkeiten, Parks, Spielstätten und Treffpunkte – " + base.Name
	lp.Canonical = abs(PlacesDir + "/")
	lp.PlaceGroups = groups
	if err := renderTo(tmpl, "places.html", lp, filepath.Join(dir, "index.html")); err != nil {
		return fmt.Errorf("orte/index.html: %w", err)
	}

	for i := range all {
		p := &all[i]
		pp := base
		pp.Place = p
		pp.PageTitle = p.Name + " – " + base.Name
		pp.MetaDesc = truncate(firstNonEmpty(p.Description, p.Type+" in "+base.Name), 160)
		pp.Canonical = abs(PlacesDir + "/" + p.Page)
		pp.JSONLD = placeJSONLD(*p, pp.Canonical)
		if err := renderTo(tmpl, "place.html", pp, filepath.Join(dir, p.Page)); err != nil {
			return fmt.Errorf("%s: %w", p.Page, err)
		}
	}
	log.Printf("place pages: %d places", len(all))
	return nil
}

// placeMapURL links to OpenStreetMap: at the coordinates if known, else a
// search for the address or name.
func placeMapURL(p places.Place) string {
	if p.Lat != 0 || p.Lon != 0 {
		lat := strconv.FormatFloat(p.Lat, 'f', 6, 64)
		lon := strconv.FormatFloat(p.Lon, 'f', 6, 64)
		return "https://www.openstreetmap.org/?mlat=" + lat + "&mlon=" + lon + "#map=17/" + lat + "/" + lon
	}
	return "https://www.openstreetmap.org/search?query=" + urlQueryEscape(firstNonEmpty(p.Address, p.Name))
}

// placeJSONLD is the schema.org Place shown to search engines.
func placeJSONLD(p staticPlace, canonical string) map[string]any {
	ld := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Place",
		"name":     p.Name,
	}
	if p.Description != "" {
		ld["description"] = p.Description
	}
	if p.Address != "" {
		ld["address"] = p.Address
	}
	if p.Lat != 0 || p.Lon != 0 {
		ld["geo"] = map[string]any{"@type": "GeoCoordinates", "latitude": p.Lat, "longitude": p.Lon}
	}
	if p.URL != "" {
		ld["sameAs"] = p.URL
	}
	if canonical != "" {
		ld["url"] = canonical
	}
	return ld
}

// withPlaceGeo gives events without coordinates those of their place (see
// assignPlaces), so their static page can show the map snippet too (the app
// does the same with placeForEvent).
func withPlaceGeo(events []staticEvent, pl *places.File) {
	if pl == nil {
		return
	}
	for i := range events {
		e := &events[i]
		if e.Lat != nil && e.Lon != nil {
			continue
		}
		if e.PlaceID == "" {
			continue
		}
		if p, ok := pl.Lookup(e.PlaceID); ok && (p.Lat != 0 || p.Lon != 0) {
			lat, lon := p.Lat, p.Lon
			e.Lat, e.Lon = &lat, &lon
		}
	}
}

// assignPlaces resolves the place of every event (places.File.Assign): in
// the built data/events.json, which the app reads, and in events, the
// upcoming events for the static pages. References that no place has are
// logged once each; those events fall back to the match terms.
func assignPlaces(out string, pl *places.File, events []staticEvent) error {
	if pl == nil {
		return nil
	}
	path := filepath.Join(out, "data", "events.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var o writer.Output
	if err := json.Unmarshal(data, &o); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	for _, ref := range pl.Assign(o.Events) {
		log.Printf("warning: place reference %q matches no place in %s, using the match terms", ref, places.FileName)
	}
	if err := writeJSON(path, o); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	for i := range events {
		events[i].PlaceID = ""
		if p, _ := pl.PlaceFor(events[i].Event); p != nil {
			events[i].PlaceID = p.ID
		}
	}
	return nil
}
