package app

import (
	"encoding/xml"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
	"openlitfass/backend/internal/writer"
)

// Static pages: besides the JavaScript app, build-site renders plain HTML
// for search engines, link previews (Open Graph) and visitors without
// JavaScript:
//
//	termine/index.html       all upcoming events, grouped by day
//	termine/<id>.html        one page per event (with schema.org JSON-LD)
//	sitemap.xml, robots.txt  only with baseUrl (sitemaps need absolute URLs)
//
// The templates live in <webDir>/templates/ and are not copied to the
// output themselves.

// StaticDir is the output subdirectory of the static event pages. The
// frontend links shared events to StaticDir/<id>.html.
const StaticDir = "termine"

const templatesDir = "templates"

// validPageID limits which event IDs become file names. writer.EventID
// produces such slugs; anything else (hand-written inbox IDs) is skipped
// instead of escaped, so a page name can never leave StaticDir.
var validPageID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,150}$`)

var (
	weekdaysDE = []string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	monthsDE   = []string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}
)

type staticEvent struct {
	model.Event
	Page      string // file name inside StaticDir
	DateLong  string // "Samstag, 21. November 2026"
	TimeText  string // "19:00 – 21:00 Uhr", "ganztägig"
	PriceText string
	MapURL    string
	Summary   string // short plain text for meta description / list
}

type staticDay struct {
	Heading string
	Events  []staticEvent
}

type staticPage struct {
	instance.Config
	Logo      template.HTML
	SiteURL   string // BaseURL without trailing slash, "" if unset
	Root      string // relative path from the page to the site root
	Canonical string // absolute URL of this page, "" without BaseURL
	PageTitle string
	MetaDesc  string
	Event     *staticEvent
	JSONLD    map[string]any
	Days      []staticDay
	Count     int
	Imprint   string // ImprintURL / PrivacyURL relative to this page
	Privacy   string
	Generated string
}

// buildStatic writes the static pages, sitemap.xml and robots.txt into out.
// Missing templates (an older or customised web dir) only skip the pages.
func buildStatic(inst instance.Instance, webDir, out string, logo template.HTML, events []staticEvent) error {
	tdir := filepath.Join(webDir, templatesDir)
	if _, err := os.Stat(filepath.Join(tdir, "event.html")); errors.Is(err, fs.ErrNotExist) {
		log.Printf("static pages: no templates in %s, skipped", tdir)
		return nil
	}
	tmpl, err := template.ParseFiles(filepath.Join(tdir, "event.html"), filepath.Join(tdir, "list.html"), filepath.Join(tdir, "layout.html"))
	if err != nil {
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

	dir := filepath.Join(out, StaticDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// List page.
	lp := base
	lp.PageTitle = "Alle Termine – " + base.Name
	lp.MetaDesc = base.Description
	lp.Canonical = abs(StaticDir + "/")
	lp.Days = groupDays(events)
	lp.Count = len(events)
	if err := renderTo(tmpl, "list.html", lp, filepath.Join(dir, "index.html")); err != nil {
		return fmt.Errorf("termine/index.html: %w", err)
	}

	// Event pages.
	for i := range events {
		ev := &events[i]
		p := base
		p.Event = ev
		p.PageTitle = ev.Title + " – " + ev.DateLong + " – " + base.Name
		p.MetaDesc = ev.Summary
		p.Canonical = abs(StaticDir + "/" + ev.Page)
		p.JSONLD = eventJSONLD(*ev, p.Canonical)
		if err := renderTo(tmpl, "event.html", p, filepath.Join(dir, ev.Page)); err != nil {
			return fmt.Errorf("%s: %w", ev.Page, err)
		}
	}

	if siteURL == "" {
		log.Printf("static pages: %d events; no baseUrl in instance.json, so no sitemap.xml", len(events))
		return nil
	}
	if err := writeSitemap(filepath.Join(out, "sitemap.xml"), siteURL, base.Name, events); err != nil {
		return fmt.Errorf("sitemap.xml: %w", err)
	}
	// robots.txt only if the instance doesn't bring its own (site/).
	robots := filepath.Join(out, "robots.txt")
	if _, err := os.Stat(robots); errors.Is(err, fs.ErrNotExist) {
		body := "User-agent: *\nAllow: /\n\nSitemap: " + siteURL + "/sitemap.xml\n"
		if err := os.WriteFile(robots, []byte(body), 0o644); err != nil {
			return err
		}
	}
	log.Printf("static pages: %d events, sitemap.xml", len(events))
	return nil
}

// loadStaticEvents reads data/events.json: upcoming events (from today on)
// whose ID is usable as file name, sorted by date and time.
func loadStaticEvents(inst instance.Instance) ([]staticEvent, error) {
	all, err := writer.LoadPrevious(inst.EventsOutput())
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	today := time.Now().In(model.Local).Format("2006-01-02")
	var events []staticEvent
	for _, e := range all {
		if e.Date < today {
			continue
		}
		if !validPageID.MatchString(e.ID) {
			log.Printf("static pages: event id %q not usable as file name, skipped", e.ID)
			continue
		}
		events = append(events, newStaticEvent(e))
	}
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		return a.StartTime < b.StartTime
	})
	return events, nil
}

// startDay is what index.html shows before (or without) JavaScript: today's
// events, or - if today has none - the next day that has events.
type startDay struct {
	staticDay
	IsToday bool
	More    int // upcoming events not shown
}

func newStartDay(events []staticEvent) *startDay {
	days := groupDays(events)
	if len(days) == 0 {
		return nil
	}
	today := time.Now().In(model.Local).Format("2006-01-02")
	return &startDay{staticDay: days[0], IsToday: days[0].Events[0].Date == today,
		More: len(events) - len(days[0].Events)}
}

func renderTo(t *template.Template, name string, data any, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := t.ExecuteTemplate(f, name, data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func newStaticEvent(e model.Event) staticEvent {
	se := staticEvent{Event: e, Page: e.ID + ".html", DateLong: longDateDE(e.Date)}
	switch {
	case e.StartTime != "" && e.EndTime != "":
		se.TimeText = e.StartTime + " – " + e.EndTime + " Uhr"
	case e.StartTime != "":
		se.TimeText = e.StartTime + " Uhr"
	default:
		se.TimeText = "ganztägig"
	}
	if e.Price.Free {
		se.PriceText = "kostenlos"
	} else if e.Price.Amount != nil {
		se.PriceText = strings.Replace(strconv.FormatFloat(*e.Price.Amount, 'f', 2, 64), ".", ",", 1) + " €"
		se.PriceText = strings.Replace(se.PriceText, ",00 €", " €", 1)
	}
	if q := firstNonEmpty(e.Location.Address, e.Location.Name); q != "" {
		se.MapURL = "https://www.openstreetmap.org/search?query=" + urlQueryEscape(q)
	}
	parts := []string{se.DateLong + ", " + se.TimeText}
	if e.Location.Name != "" {
		parts = append(parts, e.Location.Name)
	}
	sum := strings.Join(parts, " · ")
	if e.Cancelled {
		sum = "Abgesagt: " + sum
	}
	if d := strings.Join(strings.Fields(e.Description), " "); d != "" {
		sum += ". " + truncate(d, 150)
	}
	se.Summary = sum
	return se
}

func groupDays(events []staticEvent) []staticDay {
	var days []staticDay
	for _, e := range events {
		if len(days) == 0 || days[len(days)-1].Events[0].Date != e.Date {
			days = append(days, staticDay{Heading: e.DateLong})
		}
		days[len(days)-1].Events = append(days[len(days)-1].Events, e)
	}
	return days
}

func longDateDE(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%s, %d. %s %d", weekdaysDE[t.Weekday()], t.Day(), monthsDE[t.Month()-1], t.Year())
}

// isoDateTime renders date + clock as ISO 8601 with the instance's UTC
// offset ("2026-11-21T19:00:00+01:00"), or the bare date without a time.
func isoDateTime(date, clock string) string {
	if clock == "" {
		return date
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, model.Local)
	if err != nil {
		return date
	}
	return t.Format(time.RFC3339)
}

// eventJSONLD is the schema.org Event shown to search engines.
func eventJSONLD(e staticEvent, canonical string) map[string]any {
	status := "https://schema.org/EventScheduled"
	if e.Cancelled {
		status = "https://schema.org/EventCancelled"
	}
	ld := map[string]any{
		"@context":            "https://schema.org",
		"@type":               "Event",
		"name":                e.Title,
		"startDate":           isoDateTime(e.Date, e.StartTime),
		"eventStatus":         status,
		"eventAttendanceMode": "https://schema.org/OfflineEventAttendanceMode",
	}
	if e.EndTime != "" && e.EndTime > e.StartTime {
		ld["endDate"] = isoDateTime(e.Date, e.EndTime)
	}
	if e.Description != "" {
		ld["description"] = e.Description
	}
	if canonical != "" {
		ld["url"] = canonical
	}
	if e.Location.Name != "" || e.Location.Address != "" {
		place := map[string]any{"@type": "Place", "name": firstNonEmpty(e.Location.Name, e.Location.Address)}
		if e.Location.Address != "" {
			place["address"] = e.Location.Address
		}
		if e.Location.Lat != nil && e.Location.Lng != nil {
			place["geo"] = map[string]any{"@type": "GeoCoordinates", "latitude": *e.Location.Lat, "longitude": *e.Location.Lng}
		}
		ld["location"] = place
	}
	if e.Organizer != "" {
		ld["organizer"] = map[string]any{"@type": "Organization", "name": e.Organizer}
	}
	if e.Price.Free {
		ld["isAccessibleForFree"] = true
	} else if e.Price.Amount != nil {
		offer := map[string]any{"@type": "Offer", "price": *e.Price.Amount, "priceCurrency": "EUR"}
		if e.URL != "" {
			offer["url"] = e.URL
		}
		ld["offers"] = offer
	}
	return ld
}

// SitemapXSL is the stylesheet (in webDir) that makes sitemap.xml readable
// in a browser. Search engines ignore it.
const SitemapXSL = "sitemap.xsl"

// sitemapNS is openLitfass' own namespace for the event details in the
// sitemap that only the stylesheet uses. Sitemaps may carry elements of
// other namespaces; search engines ignore those they don't know.
const sitemapNS = "urn:openlitfass:sitemap"

type sitemapURL struct {
	Loc     string        `xml:"loc"`
	LastMod string        `xml:"lastmod,omitempty"`
	Event   *sitemapEvent `xml:",omitempty"`
}

type sitemapEvent struct {
	XMLName   xml.Name `xml:"urn:openlitfass:sitemap event"`
	Title     string   `xml:"title,attr"`
	Date      string   `xml:"date,attr"`
	Day       string   `xml:"day,attr"`
	Time      string   `xml:"time,attr,omitempty"`
	Place     string   `xml:"place,attr,omitempty"`
	Category  string   `xml:"category,attr,omitempty"`
	Cancelled bool     `xml:"cancelled,attr,omitempty"`
}

type sitemapSite struct {
	XMLName xml.Name `xml:"urn:openlitfass:sitemap site"`
	Name    string   `xml:"name,attr"`
	Base    string   `xml:"base,attr"`
	Count   int      `xml:"events,attr"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	Site    sitemapSite  `xml:"site"`
	URLs    []sitemapURL `xml:"url"`
}

func writeSitemap(path, siteURL, siteName string, events []staticEvent) error {
	today := time.Now().In(model.Local).Format("2006-01-02")
	set := sitemapSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		Site: sitemapSite{Name: siteName, Base: siteURL, Count: len(events)},
		URLs: []sitemapURL{
			{Loc: siteURL + "/", LastMod: today},
			{Loc: siteURL + "/" + StaticDir + "/", LastMod: today},
		}}
	for _, e := range events {
		ev := &sitemapEvent{Title: e.Title, Date: e.Date, Day: e.DateLong, Place: e.Location.Name,
			Category: e.Category, Cancelled: e.Cancelled}
		if e.StartTime != "" {
			ev.Time = e.StartTime
		}
		set.URLs = append(set.URLs, sitemapURL{Loc: siteURL + "/" + StaticDir + "/" + e.Page, Event: ev})
	}
	data, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	head := xml.Header + `<?xml-stylesheet type="text/xsl" href="` + SitemapXSL + `"?>` + "\n"
	return os.WriteFile(path, append([]byte(head), append(data, '\n')...), 0o644)
}

// relToRoot makes a site-relative link from the instance config (e.g.
// "impressum.html") work from a page root levels below the site root;
// absolute URLs, mailto: and fragment links stay as they are.
func relToRoot(root, u string) string {
	if u == "" || strings.Contains(u, ":") || strings.HasPrefix(u, "/") || strings.HasPrefix(u, "#") {
		return u
	}
	return root + u
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + " …"
}

func urlQueryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
