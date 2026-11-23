// Command serve runs one openLitfass instance self-contained: it serves the
// site over HTTP and fetches + rebuilds on an interval (see
// internal/app.Server). Default command of the container image:
//
//	docker run -v ./weissens.ee:/instance -p 8080:8080 ghcr.io/schreibtischhelden/openlitfass
//
// Usage:
//
//	serve [-addr :8080] [-interval 6h] [-metrics-addr :9090] [-web DIR] [-public DIR]
//	      [-static-pages] [-sitemap] [INSTANCE_DIR]
//
// Flags fall back to environment variables (see internal/app/env.go):
// $OL_LISTEN_ADDR, $OL_FETCH_INTERVAL, $OL_METRICS_ADDR, $OL_WEB_DIR,
// $OL_PUBLIC_DIR, $OL_STATIC_PAGES, $OL_SITEMAP_GENERATOR, $OL_INSTANCE_DIR.
// -interval 0 only serves (data updated elsewhere, e.g. CI).
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"openlitfass/backend/internal/app"
	"openlitfass/backend/internal/instance"
)

func main() {
	env := app.SiteOptionsFromEnv()
	addr := flag.String("addr", app.EnvOr(app.EnvListenAddr, ":8080"), "listen address")
	interval := flag.Duration("interval", app.DefaultFetchInterval(), "fetch interval (0 = never fetch)")
	webDir := flag.String("web", app.DefaultWebDir(), "core frontend directory")
	publicDir := flag.String("public", os.Getenv(app.EnvPublicDir), "build directory (default <instance>/.openlitfass-serve)")
	metricsAddr := flag.String("metrics-addr", os.Getenv(app.EnvMetricsAddr), "separate listen address for /metrics (default: on -addr)")
	staticPages := flag.Bool("static-pages", env.StaticPages, "static event pages and pre-rendered start page")
	sitemap := flag.Bool("sitemap", env.Sitemap, "sitemap.xml and robots.txt (needs baseUrl)")
	flag.Parse()

	inst, err := instance.Load(instance.Dir(flag.Args()))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	opts := app.SiteOptions{StaticPages: *staticPages, Sitemap: *sitemap}
	s := &app.Server{Instance: inst, WebDir: *webDir, PublicDir: *publicDir, Interval: *interval, MetricsAddr: *metricsAddr, Options: opts}
	log.Printf("openLitfass %s, instance %q (%s), fetch interval %s, static pages %t, sitemap %t",
		app.Version, inst.Config.Name, inst.Dir, *interval, opts.StaticPages, opts.Sitemap)
	err = s.Run(ctx, *addr)
	stop()
	if err != nil {
		log.Fatal(err)
	}
}
