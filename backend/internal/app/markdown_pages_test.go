package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openlitfass/backend/internal/instance"
)

func TestBuildSiteMarkdownPages(t *testing.T) {
	// Arrange
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	writeFile(t, filepath.Join(instDir, "instance.json"),
		`{"name":"Testkiez","imprintUrl":"seite.html?page=impressum","privacyUrl":"https://example.org/datenschutz"}`)
	writeFile(t, filepath.Join(instDir, "site", "pages", "impressum.md"), "# Impressum\n")
	writeFile(t, filepath.Join(instDir, "site", "pages", "datenschutz.md"), "# Datenschutz\n")
	writeFile(t, filepath.Join(instDir, "site", "pages", ".versteckt.md"), "# nein\n")
	writeFile(t, filepath.Join(instDir, "site", "pages", "notiz.txt"), "kein Markdown")
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")

	// Act
	err = BuildSite(inst, webDir, out, SiteOptions{})

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	page := readFile(t, filepath.Join(out, MarkdownPageFile))
	for _, want := range []string{
		`<title>Testkiez</title>`,
		`<main id="main">`,
		`<script src="vendor/markdown-pages/markdown-pages.js"></script>`,
		`<a href="pages/datenschutz.md">datenschutz</a>`,
		`<a href="pages/impressum.md">impressum</a>`,
		`<a href="seite.html?page=impressum">Impressum</a>`,
		`<a href="https://example.org/datenschutz">Datenschutz</a>`,
		`role="banner"`,
		`role="contentinfo"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("%s lacks %q", MarkdownPageFile, want)
		}
	}
	// markdown-pages.js replaces <header>/<footer> with header.md/footer.md
	// and the content of <main>, so the shell has neither and keeps its
	// footer outside <main>.
	for _, unwanted := range []string{"<header", "<footer", "versteckt", "notiz", "termine/"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("%s should not contain %q", MarkdownPageFile, unwanted)
		}
	}
	if strings.Index(page, `role="contentinfo"`) < strings.Index(page, "</main>") {
		t.Error("footer must come after </main>")
	}
	for _, name := range []string{"pages/impressum.md", "vendor/markdown-pages/markdown-pages.js", "vendor/markdown-pages/assets/vendor/dompurify/purify.min.js"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s missing in the build", name)
		}
	}
}

func TestBuildSiteMarkdownPagesLinkStaticList(t *testing.T) {
	// Arrange
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Testkiez"}`)
	writeFile(t, filepath.Join(instDir, "site", "pages", "impressum.md"), "# Impressum\n")
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")

	// Act
	err = BuildSite(inst, webDir, out, SiteOptions{StaticPages: true})

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if page := readFile(t, filepath.Join(out, MarkdownPageFile)); !strings.Contains(page, `<a href="termine/">Alle Termine</a>`) {
		t.Errorf("%s should link termine/ with static pages", MarkdownPageFile)
	}
}

func TestBuildSiteWithoutMarkdownPages(t *testing.T) {
	// Arrange
	webDir, _ := filepath.Abs(filepath.Join("..", "..", "..", "web"))
	root := t.TempDir()
	instDir := filepath.Join(root, "inst")
	writeFile(t, filepath.Join(instDir, "instance.json"), `{"name":"Ohne Seiten"}`)
	inst, err := instance.Load(instDir)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "out")

	// Act
	err = BuildSite(inst, webDir, out, SiteOptions{})

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, MarkdownPageFile)); err == nil {
		t.Errorf("%s should not exist without site/pages/*.md", MarkdownPageFile)
	}
}
