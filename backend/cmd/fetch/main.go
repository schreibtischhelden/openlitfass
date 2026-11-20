// Command fetch runs every source configured in an instance directory and
// writes <instance>/data/events.json, categories.json and sources.json (see
// internal/app.Fetch). Intended for CI workflows; `serve` does the same on
// a schedule. Works the same locally:
//
//	go run ./cmd/fetch ../path/to/instance     (or $INSTANCE_DIR, else ".")
//
// Exit code 1 if nothing could be written because sources failed; 0 for a
// fresh instance without sources.
package main

import (
	"context"
	"log"
	"os"

	"openlitfass/backend/internal/app"
	"openlitfass/backend/internal/instance"
)

func main() {
	inst, err := instance.Load(instance.Dir(os.Args[1:]))
	if err != nil {
		log.Fatalf("load instance: %v", err)
	}
	log.Printf("instance %q (%s), timezone %s", inst.Config.Name, inst.Dir, inst.Config.Timezone)

	res, err := app.Fetch(context.Background(), inst)
	if err != nil {
		log.Fatal(err)
	}
	if !res.Written && len(res.Failed) > 0 {
		os.Exit(1)
	}
}
