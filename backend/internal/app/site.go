package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openlitfass/backend/internal/instance"
)

// BuildMarker marks an output directory created by BuildSite; only such
// directories are ever wiped.
const BuildMarker = ".openlitfass-build"

const marker = BuildMarker

// page is what web/index.html gets rendered with.
type page struct {
	instance.Config
	Logo        template.HTML
	SiteURL     string // BaseURL without trailing slash
	StaticDir   string // static event pages, see static.go
	StaticPages bool   // OL_STATIC_PAGES: termine/ exists
	TouchIcon   bool   // site/icon-192.png exists (apple-touch-icon)
	// Start is pre-rendered into the list (today's events, else the next
	// day with events) for crawlers and visitors without JavaScript; the
	// app replaces it once it has loaded the data. nil = no events.
	Start *startDay
}

// BuildSite assembles the deployable static site for an instance into out:
//
//  1. the core frontend (webDir, everything except index.html)
//  2. overlaid with <instance>/site/ (theme.css, logo.svg, fonts, ...)
//  3. plus <instance>/data/*.json (written by Fetch)
//  4. webDir/index.html rendered as a Go html/template with the values from
//     instance.json and the instance logo inlined
//  5. optional (SiteOptions): static pages for search engines and visitors
//     without JavaScript (termine/, pre-rendered start page), sitemap.xml
//     and robots.txt; see static.go
//  6. data/site.json, manifest.webmanifest, sw.js with a build id
//
// out is wiped first, but only if BuildSite created it (BuildMarker) or it
// is empty/missing.
func BuildSite(inst instance.Instance, webDir, out string, opts SiteOptions) error {
	if err := prepareOut(out, inst.Dir, webDir); err != nil {
		return fmt.Errorf("prepare %s: %w", out, err)
	}
	if err := copyTree(webDir, out, func(rel string) bool { return rel == "index.html" || strings.HasPrefix(rel, templatesDir+"/") }); err != nil {
		return fmt.Errorf("copy core web: %w", err)
	}
	if err := copyTree(inst.SiteDir(), out, nil); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("copy instance site: %w", err)
	}
	if err := copyTree(inst.DataDir(), filepath.Join(out, "data"), func(rel string) bool {
		return !strings.HasSuffix(rel, "/") && !strings.HasSuffix(rel, ".json")
	}); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("copy data: %w", err)
	}
	logo, err := readLogo(webDir, inst)
	if err != nil {
		return err
	}
	fav, err := makeFavicon(logo, inst.Config.ThemeColor)
	if err != nil {
		return fmt.Errorf("favicon: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, FaviconFile), fav, 0o644); err != nil {
		return fmt.Errorf("favicon: %w", err)
	}
	// Optional parts (SiteOptions, off by default): static pages incl. the
	// pre-rendered start page, and the sitemap.
	var events []staticEvent
	var start *startDay
	if opts.StaticPages {
		if events, err = loadStaticEvents(inst); err != nil {
			return err
		}
		start = newStartDay(events)
	}
	if err := renderIndex(webDir, inst, logo, start, opts, filepath.Join(out, "index.html")); err != nil {
		return fmt.Errorf("render index.html: %w", err)
	}
	// Places (<instance>/places.json, optional): data/places.json for the
	// app, with OL_STATIC_PAGES also orte/ pages.
	pl, err := writePlaces(inst, out)
	if err != nil {
		return fmt.Errorf("places: %w", err)
	}
	var placeIDs []string
	if opts.StaticPages {
		if err := buildStatic(inst, webDir, out, logo, events); err != nil {
			return fmt.Errorf("static pages: %w", err)
		}
		if pl != nil {
			if err := buildPlacePages(inst, webDir, out, logo, pl, events); err != nil {
				return fmt.Errorf("place pages: %w", err)
			}
			for _, p := range pl.Places {
				placeIDs = append(placeIDs, p.ID)
			}
		}
	}
	if opts.Sitemap {
		if err := buildSitemap(inst, out, events, opts.StaticPages, placeIDs); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(out, "data", "site.json"), inst.Config); err != nil {
		return fmt.Errorf("write site.json: %w", err)
	}
	if err := writeJSON(filepath.Join(out, "manifest.webmanifest"), manifest(inst.Config, out)); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := stampServiceWorker(filepath.Join(out, "sw.js")); err != nil {
		return fmt.Errorf("service worker: %w", err)
	}
	log.Printf("built site for %q in %s", inst.Config.Name, out)
	return nil
}

// prepareOut empties out, but only if it's safe: never the instance or web
// directory themselves, and only a directory that a previous build-site run
// created (or a directory that doesn't exist / is empty yet).
func prepareOut(out, instDir, webDir string) error {
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	for _, other := range []string{instDir, webDir} {
		if absOther, err := filepath.Abs(other); err == nil && absOther == absOut {
			return fmt.Errorf("refusing to use %s as output: it is an input directory", out)
		}
	}

	entries, err := os.ReadDir(out)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// nothing to clean
	case err != nil:
		return err
	case len(entries) > 0:
		if _, err := os.Stat(filepath.Join(out, marker)); err != nil {
			return fmt.Errorf("refusing to wipe %s: not empty and not created by build-site (no %s)", out, marker)
		}
		if err := os.RemoveAll(out); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, marker), []byte("generated by build-site, safe to delete\n"), 0o644)
}

// copyTree copies every regular file under src into dst, keeping relative
// paths and overwriting existing files. skip (may be nil) gets the
// slash-separated path relative to src; directories end in "/" and are
// skipped as a whole.
func copyTree(src, dst string, skip func(rel string) bool) error {
	if _, err := os.Stat(src); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if rel != "." && skip != nil && skip(filepath.ToSlash(rel)+"/") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if skip != nil && skip(filepath.ToSlash(rel)) {
			return nil
		}
		return copyFile(path, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// readLogo returns the instance logo if present, else the core default.
func readLogo(webDir string, inst instance.Instance) (template.HTML, error) {
	// Instance logo if present, else the core's neutral default. Inlined
	// (not <img>) so it can use currentColor and the theme's CSS classes.
	// The file comes from the instance repo itself, so it is trusted.
	logoPath := filepath.Join(inst.SiteDir(), "logo.svg")
	logo, err := os.ReadFile(logoPath)
	if errors.Is(err, fs.ErrNotExist) {
		logo, err = os.ReadFile(filepath.Join(webDir, "logo.svg"))
	}
	if err != nil {
		return "", fmt.Errorf("read logo: %w", err)
	}
	return template.HTML(logo), nil
}

func renderIndex(webDir string, inst instance.Instance, logo template.HTML, start *startDay, opts SiteOptions, dst string) error {
	tmpl, err := template.ParseFiles(filepath.Join(webDir, "index.html"))
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, page{Config: inst.Config, Logo: logo, SiteURL: strings.TrimRight(inst.Config.BaseURL, "/"), StaticDir: StaticDir, StaticPages: opts.StaticPages, Start: start,
		TouchIcon: fileExists(filepath.Join(filepath.Dir(dst), "icon-192.png"))}); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
}

// manifest builds the PWA web app manifest from instance.json. Icons:
// favicon.svg always (scalable, see makeFavicon), plus icon-192.png / icon-512.png if the
// instance ships them in site/ (recommended - some platforms ignore SVG
// icons when installing).
func manifest(c instance.Config, out string) map[string]any {
	icons := []manifestIcon{{Src: FaviconFile, Sizes: "any", Type: "image/svg+xml"}}
	for _, size := range []string{"192", "512"} {
		name := "icon-" + size + ".png"
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			icons = append(icons, manifestIcon{Src: name, Sizes: size + "x" + size, Type: "image/png", Purpose: "any maskable"})
		}
	}
	m := map[string]any{
		"name":        c.Title,
		"short_name":  c.ShortName,
		"description": c.Description,
		"lang":        c.Language,
		"start_url":   ".",
		"scope":       ".",
		"display":     "standalone",
		"icons":       icons,
	}
	if c.ThemeColor != "" {
		m["theme_color"] = c.ThemeColor
		m["background_color"] = c.ThemeColor
	}
	return m
}

// stampServiceWorker replaces the __BUILD_ID__ placeholder in sw.js with a
// per-build value, so every deploy gets a new cache name and old caches are
// dropped (see web/sw.js).
func stampServiceWorker(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	id := time.Now().UTC().Format("20060102T150405")
	return os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "__BUILD_ID__", id)), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
