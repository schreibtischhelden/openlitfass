package instance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The *.example.json templates are never loaded by fetch, so a syntax error
// in them only shows up when someone copies one into their instance.
func TestExampleInstanceJSONIsValid(t *testing.T) {
	// Arrange
	root := filepath.Join("..", "..", "..", "example-instance")
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no JSON files found in %s", root)
	}

	for _, path := range files {
		t.Run(filepath.ToSlash(path[len(root)+1:]), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			// Act
			var v any
			err = json.Unmarshal(data, &v)

			// Assert
			if err != nil {
				t.Errorf("%s is not valid JSON: %v", path, err)
			}
		})
	}
}

func TestConfigFetchTimeout(t *testing.T) {
	for minutes, want := range map[int]time.Duration{
		0:  DefaultFetchTimeout,
		-1: DefaultFetchTimeout,
		15: 15 * time.Minute,
	} {
		if got := (Config{FetchTimeoutMinutes: minutes}).FetchTimeout(); got != want {
			t.Errorf("FetchTimeout() with %d minutes = %s, want %s", minutes, got, want)
		}
	}
}
