// Command serve runs one openLitfass instance self-contained: it serves the
// site over HTTP and fetches + rebuilds on an interval (see
// internal/app.Server). Default command of the container image:
//
//	docker run -v ./weissens.ee:/instance -p 8080:8080 ghcr.io/<owner>/openlitfass
//
// Usage:
//
//	serve [-addr :8080] [-interval 6h] [-web DIR] [-public DIR] [INSTANCE_DIR]
//
// Flags fall back to $ADDR, $FETCH_INTERVAL, $WEB_DIR, $PUBLIC_DIR,
// $INSTANCE_DIR. -interval 0 only serves (data updated elsewhere, e.g. CI).
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"openlitfass/backend/internal/app"
	"openlitfass/backend/internal/instance"
)

func main() {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "listen address")
	interval := flag.Duration("interval", envDuration("FETCH_INTERVAL", 6*time.Hour), "fetch interval (0 = never fetch)")
	webDir := flag.String("web", envOr("WEB_DIR", "/opt/openlitfass/web"), "core frontend directory")
	publicDir := flag.String("public", os.Getenv("PUBLIC_DIR"), "build directory (default <instance>/.openlitfass-serve)")
	flag.Parse()

	inst, err := instance.Load(instance.Dir(flag.Args()))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := &app.Server{Instance: inst, WebDir: *webDir, PublicDir: *publicDir, Interval: *interval}
	log.Printf("instance %q (%s), fetch interval %s", inst.Config.Name, inst.Dir, *interval)
	if err := s.Run(ctx, *addr); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("ignoring invalid %s=%q", key, v)
	}
	return def
}
