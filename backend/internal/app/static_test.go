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
	if err := BuildSite(inst, webDir, out); err != nil {
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
	for _, want := range []string{`<?xml-stylesheet type="text/xsl" href="sitemap.xsl"?>`, `title="Lesung &lt;im&gt; Hof"`, `cancelled="true"`, `xmlns="urn:openlitfass:sitemap"`} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "sitemap.xsl")); err != nil {
		t.Error("sitemap.xsl missing in output")
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
	if err := BuildSite(inst, webDir, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "sitemap.xml")); err == nil {
		t.Error("sitemap.xml without baseUrl")
	}
	if _, err := os.Stat(filepath.Join(out, StaticDir, "index.html")); err != nil {
		t.Error("list page missing")
	}
}
