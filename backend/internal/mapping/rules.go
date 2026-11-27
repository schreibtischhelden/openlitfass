package mapping

import (
	"regexp"
	"strconv"
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
	// Template combines several values into one, as an alternative to
	// Path: every {dot.path} is replaced by its resolved value, e.g.
	// "{venue.address}, {venue.zip} {venue.city}". Separators next to
	// empty values are tidied up ("Musterweg 1, , Berlin" can't happen).
	Template string `json:"template,omitempty"`
}

var placeholderRe = regexp.MustCompile(`\{([^{}]+)\}`)

func (r FieldRule) resolveString(item interface{}) string {
	s := ""
	if r.Path != "" {
		s = String(item, r.Path)
	} else if r.Template != "" {
		s = renderTemplate(item, r.Template)
	}
	if r.Regex != "" && s != "" {
		if re, err := regexp.Compile(r.Regex); err == nil {
			s = RegexValue(re, s)
		}
	}
	if s == "" {
		s = r.Default
	}
	return s
}

// RegexValue applies re to s and returns, in this order:
//   - the named group "value", if re has one;
//   - all non-empty capture groups joined by a space - so alternatives
//     work ("(\d+)\s*€|Eintritt (frei)" gives "27" or "frei") and parts
//     from different places can be combined ("(\d+\.\d+\.\d+).*?(\d+) Uhr"
//     on "{title} {content}" gives "21.11.26 20");
//   - the whole match, if re has no groups;
//   - "" if re doesn't match.
func RegexValue(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	if idx := re.SubexpIndex("value"); idx > 0 {
		return m[idx]
	}
	if len(m) == 1 {
		return m[0]
	}
	var parts []string
	for _, g := range m[1:] {
		if g = strings.TrimSpace(g); g != "" {
			parts = append(parts, g)
		}
	}
	return strings.Join(parts, " ")
}

// resolveTimeClock parses the resolved string with Format. Format may list
// several layouts separated by "|" (tried in order, e.g.
// "2 Jan 2006 15:04|2 Jan 2006"); hasClock reports whether the layout that
// matched contains a time of day - if not, the event is all-day.
//
// German month names and abbreviations ("Nov.", "März", "Dezember") are
// translated to the English ones Go's parser knows when a layout uses
// month names.
func (r FieldRule) resolveTimeClock(item interface{}) (t time.Time, hasClock bool, ok bool) {
	s := strings.TrimSpace(r.resolveString(item))
	if s == "" {
		return time.Time{}, false, false
	}
	for _, name := range strings.Split(r.Format, "|") {
		layout := resolveLayout(strings.TrimSpace(name))
		in := s
		if strings.Contains(layout, "Jan") {
			in = germanMonthsToEnglish(in)
		}
		// ParseInLocation: if s carries an explicit offset/zone matching the
		// layout (e.g. RFC3339 with "Z" or "+02:00"), that wins; otherwise s
		// is interpreted as instance-local time (model.Local). Covers both
		// "source gives UTC/offset timestamps" and "source gives naive local
		// strings".
		if t, err := time.ParseInLocation(layout, in, model.Local); err == nil {
			if !strings.Contains(layout, "2006") && !strings.Contains(layout, "06") {
				t = inferYear(t, time.Now().In(model.Local))
			}
			return t, layoutHasClock(layout), true
		}
	}
	return time.Time{}, false, false
}

// inferYear completes a date parsed without a year ("FR | 20.11."): event
// listings only show upcoming dates, so it's this year unless that lies
// more than a month in the past - then it's next year (a December page
// listing "12.01." means January of the next year).
func inferYear(t, now time.Time) time.Time {
	y := now.Year()
	d := time.Date(y, t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, model.Local)
	if d.Before(now.AddDate(0, -1, 0)) {
		d = d.AddDate(1, 0, 0)
	}
	return d
}

func layoutHasClock(layout string) bool {
	return strings.Contains(layout, "15") || strings.Contains(layout, "03") ||
		strings.Contains(layout, "3:04") || strings.Contains(layout, ":04")
}

var germanMonthRe = regexp.MustCompile(`(?i)\b(januar|jänner|jan|februar|feb|märz|maerz|mär|mrz|april|apr|mai|juni|jun|juli|jul|august|aug|september|sept|sep|oktober|okt|november|nov|dezember|dez)\b\.?`)

var germanMonths = map[string]string{
	"januar": "Jan", "jänner": "Jan", "jan": "Jan",
	"februar": "Feb", "feb": "Feb",
	"märz": "Mar", "maerz": "Mar", "mär": "Mar", "mrz": "Mar",
	"april": "Apr", "apr": "Apr", "mai": "May",
	"juni": "Jun", "jun": "Jun", "juli": "Jul", "jul": "Jul",
	"august": "Aug", "aug": "Aug",
	"september": "Sep", "sept": "Sep", "sep": "Sep",
	"oktober": "Oct", "okt": "Oct",
	"november": "Nov", "nov": "Nov",
	"dezember": "Dec", "dez": "Dec",
}

func germanMonthsToEnglish(s string) string {
	return germanMonthRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.ToLower(strings.TrimSuffix(m, "."))
		if en, ok := germanMonths[key]; ok {
			return en
		}
		return m
	})
}

var (
	emptySepRe   = regexp.MustCompile(`\s*,(\s*,)+`)
	multiSpaceRe = regexp.MustCompile(`\s{2,}`)
)

func renderTemplate(item interface{}, tmpl string) string {
	// {a|b|c}: the first of these paths with a non-empty value.
	s := placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		for _, p := range strings.Split(m[1:len(m)-1], "|") {
			if v := strings.TrimSpace(String(item, strings.TrimSpace(p))); v != "" {
				return v
			}
		}
		return ""
	})
	s = multiSpaceRe.ReplaceAllString(s, " ")
	s = emptySepRe.ReplaceAllString(s, ",")
	s = strings.ReplaceAll(s, " ,", ",")
	return strings.Trim(s, " ,")
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
	// UnknownIfMissing: when AmountPath doesn't resolve at all, the price
	// is unknown ({free: false, amount: null}, nothing shown in the
	// frontend) instead of the default "missing = kostenlos". For sources
	// that omit the price when they don't know it, not when it's free.
	UnknownIfMissing bool `json:"unknownIfMissing,omitempty"`
	// AmountRegex, if set, pulls the amount out of the text at AmountPath
	// (same rules as FieldRule.Regex), e.g. from a description:
	// `(?i)eintritt\W{0,3}(frei)|(\d+(?:,\d+)?)\s*€`. A result that isn't
	// a number ("frei") counts as 0 = free; no match counts as missing.
	AmountRegex string `json:"amountRegex,omitempty"`
}

func (r PriceRule) resolve(item interface{}) model.Price {
	if r.AmountRegex != "" && r.AmountPath != "" {
		re, err := regexp.Compile(r.AmountRegex)
		if err != nil {
			return model.Price{Free: false, Amount: nil}
		}
		v := RegexValue(re, String(item, r.AmountPath))
		if v == "" {
			if r.UnknownIfMissing {
				return model.Price{Free: false, Amount: nil}
			}
			return model.Price{Free: true, Amount: nil}
		}
		amount, _ := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
		if amount == 0 {
			return model.Price{Free: true, Amount: nil}
		}
		return model.Price{Free: false, Amount: &amount}
	}
	if r.UnknownIfMissing && r.AmountPath != "" && String(item, r.AmountPath) == "" {
		return model.Price{Free: false, Amount: nil}
	}
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
	// Equals, if set, makes the rule true when the resolved value equals one
	// of these "|"-separated strings (case-insensitive) instead of being
	// read as a boolean - e.g. {"path": "status", "equals": "cancelled|canceled"}.
	Equals string `json:"equals,omitempty"`
}

func (r BoolRule) resolve(item interface{}) bool {
	if r.Path == "" {
		return false
	}
	if r.Equals != "" {
		v := strings.TrimSpace(String(item, r.Path))
		for _, want := range strings.Split(r.Equals, "|") {
			if v != "" && strings.EqualFold(v, strings.TrimSpace(want)) {
				return true
			}
		}
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
	SourceID    FieldRule `json:"sourceId"`
	Title       FieldRule `json:"title"`
	Description FieldRule `json:"description"`
	Category    FieldRule `json:"category"`
	Organizer   FieldRule `json:"organizer"`
	URL         FieldRule `json:"url"`
	// Image resolves the URL of the event's image, e.g. {"path": "image.url"}
	// for JSON or {"path": "enclosure@url"} for an RSS enclosure. Only kept
	// when the source sets "images": true.
	Image           FieldRule `json:"image"`
	LocationName    FieldRule `json:"locationName"`
	LocationAddress FieldRule `json:"locationAddress"`
	// PlaceRef resolves the event's place reference (see model.Event.Place),
	// for sources with several venues, e.g. {"template": "brotfabrik:{venue.id}"}
	// matched against the refs in places.json.
	PlaceRef FieldRule `json:"placeRef"`

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
	// Cancelled marks events the source reports as called off (see
	// model.Event.Cancelled), e.g. {"path": "status", "equals": "cancelled"}.
	Cancelled BoolRule `json:"cancelled"`
}

// BuildEvent applies m to a single generic item (an unmarshalled JSON node,
// or the tag-name map built from an RSS <item>) and produces a model.Event.
// sourceName becomes Event.Source.Name and feeds the stable id derivation.
//
// Text fields are run through CleanText/CleanMultiline (HTML tags and
// entities, soft hyphens, whitespace) - feeds and CMS APIs routinely ship
// HTML where the event model expects plain text.
func BuildEvent(item interface{}, m *SourceMapping, sourceName string, now time.Time) model.Event {
	ev := model.Event{
		Title:       CleanText(m.Title.resolveString(item)),
		Description: CleanMultiline(m.Description.resolveString(item)),
		Category:    CleanText(m.Category.resolveString(item)),
		Organizer:   CleanText(m.Organizer.resolveString(item)),
		URL:         strings.TrimSpace(m.URL.resolveString(item)),
		Image:       strings.TrimSpace(m.Image.resolveString(item)),
		Location: model.Location{
			Name:    CleanText(m.LocationName.resolveString(item)),
			Address: CleanText(m.LocationAddress.resolveString(item)),
		},
		Place:                strings.TrimSpace(m.PlaceRef.resolveString(item)),
		Price:                m.Price.resolve(item),
		TicketRequired:       m.TicketRequired.resolve(item),
		RegistrationRequired: m.RegistrationRequired.resolve(item),
		FamilyFriendly:       m.FamilyFriendly.resolve(item),
		Cancelled:            m.Cancelled.resolve(item),
		Source: model.SourceRef{
			Name:     sourceName,
			SourceID: m.SourceID.resolveString(item),
		},
		LastSeen: now,
	}

	if start, hasClock, ok := m.Start.resolveTimeClock(item); ok {
		date, startTime := model.LocalParts(start)
		ev.Date = date
		if hasClock { // a date-only layout matched: all-day event
			ev.StartTime = startTime
			if end, endClock, ok := m.End.resolveTimeClock(item); ok && endClock && end.After(start) {
				_, endTime := model.LocalParts(end)
				ev.EndTime = endTime
			}
		}
	} else {
		ev.Date = m.Date.resolveString(item)
		ev.StartTime = m.StartTime.resolveString(item)
		ev.EndTime = m.EndTime.resolveString(item)
	}

	return ev
}

// MarshalJSON omits rules that aren't set. config.RecordAttempt rewrites a
// source file through SourceConfig after every fetch; without this, every
// unused rule would show up as `"date": {}` etc. in the hand-written file.
func (m SourceMapping) MarshalJSON() ([]byte, error) { //nolint:gocritic // value receiver so json.Marshal also uses it for non-pointer values
	fr := func(r FieldRule) *FieldRule {
		if r == (FieldRule{}) {
			return nil
		}
		return &r
	}
	br := func(r BoolRule) *BoolRule {
		if r == (BoolRule{}) {
			return nil
		}
		return &r
	}
	var price *PriceRule
	if m.Price != (PriceRule{}) {
		price = &m.Price
	}
	return MarshalNoEscape(struct {
		SourceID             *FieldRule `json:"sourceId,omitempty"`
		Title                *FieldRule `json:"title,omitempty"`
		Description          *FieldRule `json:"description,omitempty"`
		Category             *FieldRule `json:"category,omitempty"`
		Organizer            *FieldRule `json:"organizer,omitempty"`
		URL                  *FieldRule `json:"url,omitempty"`
		Image                *FieldRule `json:"image,omitempty"`
		LocationName         *FieldRule `json:"locationName,omitempty"`
		LocationAddress      *FieldRule `json:"locationAddress,omitempty"`
		PlaceRef             *FieldRule `json:"placeRef,omitempty"`
		Start                *FieldRule `json:"start,omitempty"`
		End                  *FieldRule `json:"end,omitempty"`
		Date                 *FieldRule `json:"date,omitempty"`
		StartTime            *FieldRule `json:"startTime,omitempty"`
		EndTime              *FieldRule `json:"endTime,omitempty"`
		Price                *PriceRule `json:"price,omitempty"`
		TicketRequired       *BoolRule  `json:"ticketRequired,omitempty"`
		RegistrationRequired *BoolRule  `json:"registrationRequired,omitempty"`
		FamilyFriendly       *BoolRule  `json:"familyFriendly,omitempty"`
		Cancelled            *BoolRule  `json:"cancelled,omitempty"`
	}{
		fr(m.SourceID), fr(m.Title), fr(m.Description), fr(m.Category), fr(m.Organizer), fr(m.URL), fr(m.Image),
		fr(m.LocationName), fr(m.LocationAddress), fr(m.PlaceRef), fr(m.Start), fr(m.End),
		fr(m.Date), fr(m.StartTime), fr(m.EndTime), price,
		br(m.TicketRequired), br(m.RegistrationRequired), br(m.FamilyFriendly), br(m.Cancelled),
	})
}
