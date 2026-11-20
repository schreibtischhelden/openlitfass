package mapping

import (
	"regexp"
	"strings"
	"time"

	"openlitfass/backend/internal/model"
)

// FieldRule describes how to pull one Event field out of a generic item (a
// map[string]interface{} for JSON sources, or the tag-name map built from an
// RSS <item> — see sources/generic_rss.go).
type FieldRule struct {
	// Path is a dot-notation path into the item, e.g. "location.title" or
	// "tags.0". Empty + no Default means "no value".
	Path string `json:"path,omitempty"`
	// Regex, if set, is applied to the string resolved via Path. The named
	// group "value" is used if present, otherwise the first capture group,
	// otherwise the whole match. Useful for pulling e.g. a time out of free
	// text ("Einlass 18:00, Beginn 19:00" -> `Beginn (?P<value>\d{2}:\d{2})`).
	Regex string `json:"regex,omitempty"`
	// Format is a Go reference time layout (or a friendly alias: "RFC3339",
	// "RFC1123Z", "RFC1123") used to parse the resolved string as a
	// time.Time. Only relevant for the Start/End fields of SourceMapping.
	Format string `json:"format,omitempty"`
	// Default is used verbatim when Path resolves to nothing/empty.
	Default string `json:"default,omitempty"`
}

func (r FieldRule) resolveString(item interface{}) string {
	s := ""
	if r.Path != "" {
		s = String(item, r.Path)
	}
	if r.Regex != "" && s != "" {
		if re, err := regexp.Compile(r.Regex); err == nil {
			if m := re.FindStringSubmatch(s); m != nil {
				if idx := re.SubexpIndex("value"); idx >= 0 && idx < len(m) && m[idx] != "" {
					s = m[idx]
				} else if len(m) > 1 {
					s = m[1]
				} else {
					s = m[0]
				}
			} else {
				s = ""
			}
		}
	}
	if s == "" {
		s = r.Default
	}
	return s
}

func (r FieldRule) resolveTime(item interface{}) (time.Time, bool) {
	s := r.resolveString(item)
	if s == "" {
		return time.Time{}, false
	}
	layout := resolveLayout(r.Format)
	// ParseInLocation: if s carries an explicit offset/zone matching the
	// layout (e.g. RFC3339 with "Z" or "+02:00"), that wins; otherwise s is
	// interpreted as instance-local time (model.Local). Covers both "source gives
	// UTC/offset timestamps" and "source gives naive local strings".
	t, err := time.ParseInLocation(layout, s, model.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func resolveLayout(name string) string {
	switch name {
	case "", "RFC3339":
		return time.RFC3339
	case "RFC1123Z":
		return time.RFC1123Z
	case "RFC1123":
		return time.RFC1123
	default:
		return name
	}
}

// PriceRule maps a source's price representation onto model.Price.
type PriceRule struct {
	// AmountPath resolves a numeric price in euros. Missing or 0 means free
	// unless FreePath overrides that.
	AmountPath string `json:"amountPath,omitempty"`
	// FreePath, if set, resolves an explicit boolean and takes precedence
	// over the amount-based inference.
	FreePath string `json:"freePath,omitempty"`
}

func (r PriceRule) resolve(item interface{}) model.Price {
	if r.FreePath != "" {
		if Bool(item, r.FreePath) {
			return model.Price{Free: true, Amount: nil}
		}
		amount := Float(item, r.AmountPath)
		return model.Price{Free: false, Amount: &amount}
	}
	if r.AmountPath == "" {
		return model.Price{Free: true, Amount: nil}
	}
	amount := Float(item, r.AmountPath)
	if amount == 0 {
		return model.Price{Free: true, Amount: nil}
	}
	return model.Price{Free: false, Amount: &amount}
}

// BoolRule maps a source's representation of a cross-cutting attribute
// (ticketRequired, registrationRequired, familyFriendly - see
// model.Event) onto a plain bool. Unlike FieldRule there's no Regex/Format:
// these fields are already boolean at the source, or absent.
type BoolRule struct {
	// Path resolves the value via mapping.Bool (true/false, "true"/"false",
	// or a non-zero number all count as true). Empty Path means "this
	// source doesn't say" - resolves to false, not an error.
	Path string `json:"path,omitempty"`
}

func (r BoolRule) resolve(item interface{}) bool {
	if r.Path == "" {
		return false
	}
	return Bool(item, r.Path)
}

// SourceMapping declares, per source, how to translate one generic item into
// a model.Event.
//
// For timing, either:
//   - set Start (with Format) and optionally End, for sources that give a
//     single parseable timestamp — this is the recommended path, since it
//     goes through model.LocalParts and always produces correctly
//     zoned Date/StartTime/EndTime; or
//   - set Date/StartTime/EndTime directly for sources that already hand you
//     those exact strings (they're used as-is/regex-extracted, not
//     reformatted, so they must already match YYYY-MM-DD / HH:MM).
type SourceMapping struct {
	SourceID        FieldRule `json:"sourceId"`
	Title           FieldRule `json:"title"`
	Description     FieldRule `json:"description"`
	Category        FieldRule `json:"category"`
	Organizer       FieldRule `json:"organizer"`
	URL             FieldRule `json:"url"`
	LocationName    FieldRule `json:"locationName"`
	LocationAddress FieldRule `json:"locationAddress"`

	Start FieldRule `json:"start"`
	End   FieldRule `json:"end"`

	Date      FieldRule `json:"date"`
	StartTime FieldRule `json:"startTime"`
	EndTime   FieldRule `json:"endTime"`

	Price PriceRule `json:"price"`

	// Cross-cutting attributes (see model.Event doc comment) - all optional,
	// default to false when unset or when Path doesn't resolve.
	TicketRequired       BoolRule `json:"ticketRequired"`
	RegistrationRequired BoolRule `json:"registrationRequired"`
	FamilyFriendly       BoolRule `json:"familyFriendly"`
}

// BuildEvent applies m to a single generic item (an unmarshalled JSON node,
// or the tag-name map built from an RSS <item>) and produces a model.Event.
// sourceName becomes Event.Source.Name and feeds the stable id derivation.
//
// Text fields are run through CleanText/CleanMultiline (HTML tags and
// entities, soft hyphens, whitespace) - feeds and CMS APIs routinely ship
// HTML where the event model expects plain text.
func BuildEvent(item interface{}, m SourceMapping, sourceName string, now time.Time) model.Event {
	ev := model.Event{
		Title:       CleanText(m.Title.resolveString(item)),
		Description: CleanMultiline(m.Description.resolveString(item)),
		Category:    CleanText(m.Category.resolveString(item)),
		Organizer:   CleanText(m.Organizer.resolveString(item)),
		URL:         strings.TrimSpace(m.URL.resolveString(item)),
		Location: model.Location{
			Name:    CleanText(m.LocationName.resolveString(item)),
			Address: CleanText(m.LocationAddress.resolveString(item)),
		},
		Price:                m.Price.resolve(item),
		TicketRequired:       m.TicketRequired.resolve(item),
		RegistrationRequired: m.RegistrationRequired.resolve(item),
		FamilyFriendly:       m.FamilyFriendly.resolve(item),
		Source: model.SourceRef{
			Name:     sourceName,
			SourceID: m.SourceID.resolveString(item),
		},
		LastSeen: now,
	}

	if start, ok := m.Start.resolveTime(item); ok {
		date, startTime := model.LocalParts(start)
		ev.Date, ev.StartTime = date, startTime
		if end, ok := m.End.resolveTime(item); ok {
			_, endTime := model.LocalParts(end)
			ev.EndTime = endTime
		}
	} else {
		ev.Date = m.Date.resolveString(item)
		ev.StartTime = m.StartTime.resolveString(item)
		ev.EndTime = m.EndTime.resolveString(item)
	}

	return ev
}
