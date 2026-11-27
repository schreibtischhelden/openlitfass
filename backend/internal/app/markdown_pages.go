package app

import (
	"errors"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"openlitfass/backend/internal/instance"
)

// Markdown pages: an instance writes pages such as the Impressum in
// site/pages/<name>.md. They are copied to pages/ like the rest of site/,
// and build-site renders one shell for all of them, MarkdownPageFile, from
// web/templates/page.html. In the browser markdown-pages.js
// (web/vendor/markdown-pages/) loads ?page=<name> into it, so links look
// like "seite.html?page=impressum".
const (
	MarkdownPageFile = "seite.html"
	MarkdownPagesDir = "pages"
)

// validMarkdownPage follows the path segments markdown-pages.js accepts
// (isSafePath): no leading dot, only letters, digits, "_", "-" and ".".
var validMarkdownPage = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,150}$`)

// buildMarkdownPages renders MarkdownPageFile into out when the site has
// Markdown pages (out/pages/*.md). Without pages, or without the template
// (an older or customised web dir), nothing is written.
func buildMarkdownPages(inst instance.Instance, webDir, out string, logo template.HTML, opts SiteOptions) error {
	names, err := markdownPageNames(filepath.Join(out, MarkdownPagesDir))
	if err != nil || len(names) == 0 {
		return err
	}
	tdir := filepath.Join(webDir, templatesDir)
	if _, err := os.Stat(filepath.Join(tdir, "page.html")); errors.Is(err, fs.ErrNotExist) {
		log.Printf("markdown pages: no page.html in %s, skipped", tdir)
		return nil
	}
	tmpl, err := template.ParseFiles(filepath.Join(tdir, "page.html"), filepath.Join(tdir, "layout.html"))
	if err != nil {
		return err
	}
	p := staticPage{Config: inst.Config, Logo: logo, SiteURL: strings.TrimRight(inst.Config.BaseURL, "/"),
		CustomScript:  fileExists(filepath.Join(out, CustomScriptFile)),
		StaticPages:   opts.StaticPages,
		PageTitle:     inst.Config.Name,
		MetaDesc:      inst.Config.Description,
		MarkdownPages: names,
	}
	p.Imprint = relToRoot(p.Root, inst.Config.ImprintURL)
	p.Privacy = relToRoot(p.Root, inst.Config.PrivacyURL)
	if err := renderTo(tmpl, "page.html", p, filepath.Join(out, MarkdownPageFile)); err != nil {
		return err
	}
	log.Printf("markdown pages: %d in %s/", len(names), MarkdownPagesDir)
	return nil
}

// markdownPageNames lists the pages (file names without ".md") directly in
// dir, sorted. A missing dir means no pages.
func markdownPageNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !ok {
			continue
		}
		if !validMarkdownPage.MatchString(name) {
			log.Printf("markdown pages: %q not usable as page name, skipped", e.Name())
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
