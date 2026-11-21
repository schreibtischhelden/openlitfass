// Package places loads an instance's curated places (<instance>/places.json):
// sights, parks, venues, markets - things without a date that the frontend
// shows in its own "Orte" view, each with the upcoming events held there.
//
// Format:
//
//	{
//	  "types": [ { "name": "Sehenswürdigkeit", "icon": ["M…"] }, … ],
//	  "places": [ {
//	    "id": "juedischer-friedhof",          // stable, used in links: #ort/<id>
//	    "name": "Jüdischer Friedhof Weißensee",
//	    "type": "Sehenswürdigkeit",            // one of types[].name
//	    "description": "…", "address": "…",
//	    "lat": 52.546, "lon": 13.456,         // optional, for the map link
//	    "url": "https://…",                    // optional, own website
//	    "openingHours": "…", "season": "…",    // optional, free text
//	    "match": ["Jüdischer Friedhof"]        // optional, see Matches
//	  } ]
//	}
package places

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"

	"openlitfass/backend/internal/model"
)

// FileName is the places file in the instance directory.
const FileName = "places.json"

type Type struct {
	Name string   `json:"name"`
	Icon []string `json:"icon"`
}

type Place struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Description  string   `json:"description,omitempty"`
	Address      string   `json:"address,omitempty"`
	Lat          float64  `json:"lat,omitempty"`
	Lon          float64  `json:"lon,omitempty"`
	URL          string   `json:"url,omitempty"`
	OpeningHours string   `json:"openingHours,omitempty"`
	Season       string   `json:"season,omitempty"`
	Match        []string `json:"match,omitempty"`
}

type File struct {
	Types  []Type  `json:"types"`
	Places []Place `json:"places"`
}

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)

// Load reads and checks places.json. A missing file is not an error: it
// returns (nil, nil) and the instance simply has no places view.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	types := map[string]bool{}
	for _, t := range f.Types {
		if t.Name == "" || len(t.Icon) == 0 {
			return nil, fmt.Errorf("%s: every type needs a name and an icon", path)
		}
		types[t.Name] = true
	}
	ids := map[string]bool{}
	for i, p := range f.Places {
		switch {
		case !validID.MatchString(p.ID):
			return nil, fmt.Errorf("%s: place %d (%q): id %q must be lower-case letters, digits and hyphens", path, i+1, p.Name, p.ID)
		case ids[p.ID]:
			return nil, fmt.Errorf("%s: place id %q is used twice", path, p.ID)
		case strings.TrimSpace(p.Name) == "":
			return nil, fmt.Errorf("%s: place %q has no name", path, p.ID)
		case !types[p.Type]:
			return nil, fmt.Errorf("%s: place %q: type %q is not one of the listed types", path, p.ID, p.Type)
		}
		ids[p.ID] = true
	}
	return &f, nil
}

// Matches reports whether an event takes place at p: one of p.Match (or,
// without Match, p.Name) occurs in the event's location name or address,
// compared without case, accents and with ß = ss. The frontend uses the
// same rule (app.js placeMatches).
func (p Place) Matches(e model.Event) bool {
	hay := Fold(e.Location.Name + " " + e.Location.Address)
	terms := p.Match
	if len(terms) == 0 {
		terms = []string{p.Name}
	}
	for _, t := range terms {
		if t = Fold(t); t != "" && strings.Contains(hay, t) {
			return true
		}
	}
	return false
}

// foldReplacer removes the accents that occur in German and neighbouring
// languages (enough for place and venue names; no extra dependency).
var foldReplacer = strings.NewReplacer(
	"ä", "a", "ö", "o", "ü", "u", "ß", "ss",
	"á", "a", "à", "a", "â", "a", "å", "a", "ã", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u",
	"ç", "c", "č", "c", "ñ", "n", "ł", "l", "š", "s", "ž", "z",
)

// Fold lower-cases s, removes accents and turns ß into ss.
func Fold(s string) string {
	return foldReplacer.Replace(strings.ToLower(s))
}
