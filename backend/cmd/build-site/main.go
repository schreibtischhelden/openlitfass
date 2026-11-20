// Command build-site assembles the deployable static site for an instance
// (see internal/app.BuildSite).
//
// Usage:
//
//	build-site [-web DIR] [-out DIR] [INSTANCE_DIR]
//
// -web defaults to $WEB_DIR, else /opt/openlitfass/web (where the
// container image puts it). -out defaults to <instance>/public.
package main

import (
	"flag"
	"log"
	"os"

	"openlitfass/backend/internal/app"
	"openlitfass/backend/internal/instance"
)

func main() {
	webDir := flag.String("web", envOr("WEB_DIR", "/opt/openlitfass/web"), "core frontend directory")
	outDir := flag.String("out", "", "output directory (default <instance>/public)")
	flag.Parse()

	inst, err := instance.Load(instance.Dir(flag.Args()))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}
	out := *outDir
	if out == "" {
		out = inst.Path("public")
	}
	if err := app.BuildSite(inst, *webDir, out); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
