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
//	    "lat": 52.546, "lon": 13.456,         // optional, for the map link and snippet
//	    "url": "https://…",                    // optional, own website
//	    "openingHours": "…", "season": "…",    // optional, free text
//	    "match": ["Jüdischer Friedhof"],       // optional, see Matches
//	    "refs": ["osm:way/123", "vow:5809"]    // optional, see Lookup
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
	"sort"
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
	// Refs are further identifiers of the place, e.g. its id in another
	// directory ("vow:5809", "osm:way/20130679"). Sources refer to a place
	// by its id or one of these (model.Event.Place, see Lookup).
	Refs []string `json:"refs,omitempty"`
}

type File struct {
	Types  []Type  `json:"types"`
	Places []Place `json:"places"`

	byKey map[string]int // place id or ref -> index in Places, see Lookup
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
	// A ref must lead to exactly one place, so it may not repeat another
	// ref or a place id.
	refs := map[string]string{}
	for _, p := range f.Places {
		for _, r := range p.Refs {
			switch {
			case strings.TrimSpace(r) != r || r == "":
				return nil, fmt.Errorf("%s: place %q: ref %q must not be empty or start or end with spaces", path, p.ID, r)
			case ids[r]:
				return nil, fmt.Errorf("%s: place %q: ref %q is the id of a place", path, p.ID, r)
			case refs[r] != "":
				return nil, fmt.Errorf("%s: ref %q is used by %q and %q", path, r, refs[r], p.ID)
			}
			refs[r] = p.ID
		}
	}
	f.index()
	return &f, nil
}

// index builds the lookup table for Lookup.
func (f *File) index() {
	f.byKey = make(map[string]int, len(f.Places))
	for i, p := range f.Places {
		f.byKey[p.ID] = i
		for _, r := range p.Refs {
			if _, taken := f.byKey[r]; !taken {
				f.byKey[r] = i
			}
		}
	}
}

// Lookup returns the place with id or ref key (exact, case-sensitive).
func (f *File) Lookup(key string) (*Place, bool) {
	if f.byKey == nil {
		f.index()
	}
	i, ok := f.byKey[strings.TrimSpace(key)]
	if !ok {
		return nil, false
	}
	return &f.Places[i], true
}

// PlaceFor returns the place an event belongs to: the one its Place
// reference names, else the first place whose match terms fit (see
// Matches), else nil. known is false when the event names a reference
// that no place has; the match terms decide then.
func (f *File) PlaceFor(e model.Event) (p *Place, known bool) {
	known = true
	if e.Place != "" {
		if p, ok := f.Lookup(e.Place); ok {
			return p, true
		}
		known = false
	}
	for i := range f.Places {
		if f.Places[i].Matches(e) {
			return &f.Places[i], known
		}
	}
	return nil, known
}

// Assign sets PlaceID on every event (see PlaceFor) and returns the
// references that no place has, sorted and without repeats, so the caller
// can warn about them once each.
func (f *File) Assign(events []model.Event) (unknown []string) {
	seen := map[string]bool{}
	for i := range events {
		e := &events[i]
		e.PlaceID = ""
		p, known := f.PlaceFor(*e)
		if p != nil {
			e.PlaceID = p.ID
		}
		if !known && !seen[e.Place] {
			seen[e.Place] = true
			unknown = append(unknown, e.Place)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// Matches reports whether an event takes place at p: one of p.Match (or,
// without Match, p.Name) occurs in the event's location name or address,
// compared without case, accents and with ß = ss. It is the fallback for
// events without a known Place reference (see PlaceFor).
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
