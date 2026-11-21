package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"openlitfass/backend/internal/instance"
	"openlitfass/backend/internal/model"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBuildSiteStaticPages(t *testing.T) {
	webDir, err := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	tomorrow := time.Now().In(model.Local).AddDate(0, 0, 1).Format("2006-01-02")
	writeFile(t, filepath.Join(instDir, "instance.json"),
		`{"name":"Testkiez","baseUrl":"https://example.org/","imprintUrl":"impressum.html"}`)
	writeFile(t, filepath.Join(instDir, "data", "events.json"), `{"events":[
	  {"id":"lesung-abc123","title":"Lesung <im> Hof","description":"Mehr: https://example.org/x","category":"Kultur",
	   "date":"`+tomorrow+`","startTime":"19:00","endTime":"21:00","location":{"name":"Hof","address":"Weg 1"},
	   "price":{"free":true,"amount":null},"cancelled":true},
	  {"id":"alt-1","title":"Vorbei","date":"2000-01-01","location":{"name":""},"price":{"free":false,"amount":null}},
	  {"id":"../boese","title":"Böse ID","date":"`+tomorrow+`","location":{"name":""},"price":{"free":false,"amount":null}}
	]}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := BuildSite(inst, webDir, out, SiteOptions{StaticPages: true, Sitemap: true}); err != nil {
		t.Fatal(err)
	}

	page := readFile(t, filepath.Join(out, StaticDir, "lesung-abc123.html"))
	for _, want := range []string{
		"Lesung &lt;im&gt; Hof",
		`<link rel="canonical" href="https://example.org/termine/lesung-abc123.html">`,
		`"@type":"Event"`,
		`EventCancelled`,
		`"isAccessibleForFree":true`,
		`href="../#lesung-abc123"`,
		`href="../impressum.html"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("event page lacks %q", want)
		}
	}

	list := readFile(t, filepath.Join(out, StaticDir, "index.html"))
	if !strings.Contains(list, `href="lesung-abc123.html"`) || strings.Contains(list, "Vorbei") || strings.Contains(list, "Böse") {
		t.Errorf("list page has wrong events:\n%s", list)
	}
	for _, name := range []string{"alt-1.html", "boese.html"} {
		if _, err := os.Stat(filepath.Join(out, StaticDir, name)); err == nil {
			t.Errorf("%s should not exist", name)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "templates")); err == nil {
		t.Error("templates/ must not be copied to the output")
	}

	sm := readFile(t, filepath.Join(out, "sitemap.xml"))
	if !strings.Contains(sm, "<loc>https://example.org/termine/lesung-abc123.html</loc>") || !strings.Contains(sm, "<loc>https://example.org/</loc>") {
		t.Errorf("sitemap:\n%s", sm)
	}
	// Plain sitemap: no stylesheet, so browsers show their XML tree view.
	if strings.Contains(sm, "xml-stylesheet") || strings.Contains(sm, "urn:openlitfass") {
		t.Errorf("sitemap should be plain sitemaps.org XML:\n%s", sm)
	}
	if rb := readFile(t, filepath.Join(out, "robots.txt")); !strings.Contains(rb, "Sitemap: https://example.org/sitemap.xml") {
		t.Errorf("robots.txt:\n%s", rb)
	}
	idx := readFile(t, filepath.Join(out, "index.html"))
	if !strings.Contains(idx, `<link rel="canonical" href="https://example.org/">`) {
		t.Error("index.html lacks canonical link")
	}
	// No event today: the start page pre-renders the next day with events.
	if !strings.Contains(idx, `href="termine/lesung-abc123.html"`) || !strings.Contains(idx, "Lesung &lt;im&gt; Hof") || strings.Contains(idx, "Heute ·") {
		t.Error("index.html lacks the pre-rendered next day")
	}
}

func TestBuildSiteWithoutBaseURLHasNoSitemap(t *testing.T) {
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Ohne URL"}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := BuildSite(inst, webDir, out, SiteOptions{StaticPages: true, Sitemap: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "sitemap.xml")); err == nil {
		t.Error("sitemap.xml without baseUrl")
	}
	if _, err := os.Stat(filepath.Join(out, StaticDir, "index.html")); err != nil {
		t.Error("list page missing")
	}
}

// Default (no OL_STATIC_PAGES / OL_SITEMAP_GENERATOR): no SEO extras.
func TestBuildSiteDefaultsWithoutExtras(t *testing.T) {
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	tomorrow := time.Now().In(model.Local).AddDate(0, 0, 1).Format("2006-01-02")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Schlicht","baseUrl":"https://example.org"}`)
	writeFile(t, filepath.Join(instDir, "data", "events.json"), `{"events":[
	  {"id":"lesung-abc123","title":"Lesung","date":"`+tomorrow+`","location":{"name":""},"price":{"free":true,"amount":null}}]}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := BuildSite(inst, webDir, out, SiteOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{StaticDir, "sitemap.xml", "robots.txt"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s should not exist by default", name)
		}
	}
	if idx := readFile(t, filepath.Join(out, "index.html")); strings.Contains(idx, "lesung-abc123") || strings.Contains(idx, StaticDir+"/") {
		t.Error("index.html should not pre-render or link static pages by default")
	}
}

func TestSitemapWithoutStaticPagesListsOnlyStart(t *testing.T) {
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Nur Sitemap","baseUrl":"https://example.org"}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := BuildSite(inst, webDir, out, SiteOptions{Sitemap: true}); err != nil {
		t.Fatal(err)
	}
	sm := readFile(t, filepath.Join(out, "sitemap.xml"))
	if !strings.Contains(sm, "<loc>https://example.org/</loc>") || strings.Contains(sm, StaticDir) {
		t.Errorf("sitemap:\n%s", sm)
	}
}

func TestEnvBool(t *testing.T) {
	for v, want := range map[string]bool{"": false, "true": true, "1": true, "false": false, "ja": false} {
		t.Setenv("OL_TEST_BOOL", v)
		if got := EnvBool("OL_TEST_BOOL", false); got != want {
			t.Errorf("EnvBool(%q) = %t, want %t", v, got, want)
		}
	}
}

// places.json: data/places.json for the app, orte/ pages with the events
// held there, sitemap entries.
func TestBuildSitePlaces(t *testing.T) {
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	tomorrow := time.Now().In(model.Local).AddDate(0, 0, 1).Format("2006-01-02")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Testkiez","baseUrl":"https://example.org"}`)
	writeFile(t, filepath.Join(instDir, "data", "events.json"), `{"events":[
	  {"id":"film-1","title":"Film am Hof","date":"`+tomorrow+`","startTime":"20:00","location":{"name":"KINO Hof"},"price":{"free":true,"amount":null}},
	  {"id":"anders-1","title":"Woanders","date":"`+tomorrow+`","location":{"name":"Markt"},"price":{"free":true,"amount":null}}]}`)
	writeFile(t, filepath.Join(instDir, "places.json"), `{
	  "types":[{"name":"Kultur","icon":["M0 0"]}],
	  "places":[{"id":"hof","name":"Hof","type":"Kultur","address":"Weg 1, 13086 Berlin","lat":52.55,"lon":13.45,"description":"Ein Hof."}]}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")
	if err := BuildSite(inst, webDir, out, SiteOptions{StaticPages: true, Sitemap: true}); err != nil {
		t.Fatal(err)
	}
	if pj := readFile(t, filepath.Join(out, "data", "places.json")); !strings.Contains(pj, `"id": "hof"`) {
		t.Errorf("data/places.json:\n%s", pj)
	}
	page := readFile(t, filepath.Join(out, PlacesDir, "hof.html"))
	for _, want := range []string{"Ein Hof.", "Film am Hof", `href="../termine/film-1.html"`, `"@type":"Place"`, "mlat=52.550000"} {
		if !strings.Contains(page, want) {
			t.Errorf("place page lacks %q", want)
		}
	}
	if strings.Contains(page, "Woanders") {
		t.Error("place page lists an event held elsewhere")
	}
	if list := readFile(t, filepath.Join(out, PlacesDir, "index.html")); !strings.Contains(list, `href="hof.html"`) {
		t.Error("places list lacks the place")
	}
	if sm := readFile(t, filepath.Join(out, "sitemap.xml")); !strings.Contains(sm, "https://example.org/orte/hof.html") {
		t.Error("sitemap lacks the place page")
	}
}
