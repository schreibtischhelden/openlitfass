package app

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Environment variables: all start with OL_ (openLitfass), so they don't
// clash with others in a container or CI job. Optional features are off
// by default - not every instance wants SEO extras.
const (
	EnvInstanceDir     = "OL_INSTANCE_DIR"      // instance directory (default: argument, else .)
	EnvWebDir          = "OL_WEB_DIR"           // core frontend directory
	EnvListenAddr      = "OL_LISTEN_ADDR"       // serve: HTTP listen address
	EnvMetricsAddr     = "OL_METRICS_ADDR"      // serve: separate listen address for /metrics
	EnvFetchInterval   = "OL_FETCH_INTERVAL"    // serve: fetch interval, 0 = only serve
	EnvPublicDir       = "OL_PUBLIC_DIR"        // serve: build directory
	EnvStaticPages     = "OL_STATIC_PAGES"      // static event pages + pre-rendered start page
	EnvSitemap         = "OL_SITEMAP_GENERATOR" // sitemap.xml + robots.txt
	defaultWebDir      = "/opt/openlitfass/web"
	defaultFetchPeriod = 6 * time.Hour
)

// SiteOptions switches optional parts of the built site on.
type SiteOptions struct {
	// StaticPages renders termine/ (a page per event, list of all events)
	// and pre-renders today's events into index.html - for search engines,
	// link previews and visitors without JavaScript. Shared links then point
	// to the event's static page.
	StaticPages bool
	// Sitemap writes sitemap.xml and robots.txt (needs baseUrl).
	Sitemap bool
}

// SiteOptionsFromEnv reads OL_STATIC_PAGES and OL_SITEMAP_GENERATOR.
func SiteOptionsFromEnv() SiteOptions {
	return SiteOptions{
		StaticPages: EnvBool(EnvStaticPages, false),
		Sitemap:     EnvBool(EnvSitemap, false),
	}
}

// EnvOr returns $key, or def if unset or empty.
func EnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvBool parses $key as a boolean (true/false, 1/0, …); unset, empty or
// invalid values give def (invalid ones with a log line).
func EnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("ignoring invalid %s=%q (expected true or false)", key, v)
		return def
	}
	return b
}

// EnvDuration parses $key as a Go duration ("6h", "30m", "0").
func EnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("ignoring invalid %s=%q", key, v)
		return def
	}
	return d
}

// DefaultWebDir is where the container image puts the core frontend.
func DefaultWebDir() string { return EnvOr(EnvWebDir, defaultWebDir) }

// DefaultFetchInterval is OL_FETCH_INTERVAL, else 6h.
func DefaultFetchInterval() time.Duration { return EnvDuration(EnvFetchInterval, defaultFetchPeriod) }
