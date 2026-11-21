// Command build-site assembles the deployable static site for an instance
// (see internal/app.BuildSite).
//
// Usage:
//
//	build-site [-web DIR] [-out DIR] [-static-pages] [-sitemap] [INSTANCE_DIR]
//
// Flags fall back to environment variables (see internal/app/env.go):
// -web $OL_WEB_DIR (else /opt/openlitfass/web, where the container image
// puts it), -static-pages $OL_STATIC_PAGES, -sitemap $OL_SITEMAP_GENERATOR
// (both off by default), INSTANCE_DIR $OL_INSTANCE_DIR. -out defaults to
// <instance>/public.
package main

import (
	"flag"
	"log"

	"openlitfass/backend/internal/app"
	"openlitfass/backend/internal/instance"
)

func main() {
	env := app.SiteOptionsFromEnv()
	webDir := flag.String("web", app.DefaultWebDir(), "core frontend directory")
	outDir := flag.String("out", "", "output directory (default <instance>/public)")
	staticPages := flag.Bool("static-pages", env.StaticPages, "static event pages and pre-rendered start page")
	sitemap := flag.Bool("sitemap", env.Sitemap, "sitemap.xml and robots.txt (needs baseUrl)")
	flag.Parse()

	inst, err := instance.Load(instance.Dir(flag.Args()))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}
	out := *outDir
	if out == "" {
		out = inst.Path("public")
	}
	if err := app.BuildSite(inst, *webDir, out, app.SiteOptions{StaticPages: *staticPages, Sitemap: *sitemap}); err != nil {
		log.Fatal(err)
	}
}
