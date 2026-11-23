package mapping

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

func item(t *testing.T, s string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestTemplate(t *testing.T) {
	r := FieldRule{Template: "{venue.address}, {venue.zip} {venue.city}"}
	tests := []struct{ in, want string }{
		{`{"venue":{"address":"Caligariplatz 1","zip":"13086","city":"Berlin"}}`, "Caligariplatz 1, 13086 Berlin"},
		{`{"venue":{"address":"","zip":"13086","city":"Berlin"}}`, "13086 Berlin"},
		{`{"venue":{"address":"Caligariplatz 1"}}`, "Caligariplatz 1"},
		{`{"venue":[]}`, ""},
		{`{"venue":{"zip":13086,"city":"Berlin"}}`, "13086 Berlin"},
	}
	for _, tt := range tests {
		if got := r.resolveString(item(t, tt.in)); got != tt.want {
			t.Errorf("template on %s = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPriceUnknownIfMissing(t *testing.T) {
	r := PriceRule{AmountPath: "cost.values.0", UnknownIfMissing: true}

	if p := r.resolve(item(t, `{"cost":{"values":[]}}`)); p.Free || p.Amount != nil {
		t.Errorf("missing price should be unknown, got %+v", p)
	}
	if p := r.resolve(item(t, `{"cost":{"values":["0"]}}`)); !p.Free {
		t.Errorf("0 should be free, got %+v", p)
	}
	if p := r.resolve(item(t, `{"cost":{"values":["7","9"]}}`)); p.Free || p.Amount == nil || *p.Amount != 7 {
		t.Errorf("7 should be 7 €, got %+v", p)
	}

	// Default behaviour unchanged: missing = kostenlos.
	if p := (PriceRule{AmountPath: "cost.values.0"}).resolve(item(t, `{}`)); !p.Free {
		t.Errorf("without unknownIfMissing, missing should stay free, got %+v", p)
	}
}

func TestSourceMappingMarshalOmitsEmptyRules(t *testing.T) {
	m := SourceMapping{
		Title: FieldRule{Path: "title"},
		Price: PriceRule{AmountPath: "p", UnknownIfMissing: true},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"title":{"path":"title"},"price":{"amountPath":"p","unknownIfMissing":true}}`
	if string(b) != want {
		t.Errorf("got %s\nwant %s", b, want)
	}
	var back SourceMapping
	if err := json.Unmarshal(b, &back); err != nil || back != m {
		t.Errorf("round trip mismatch: %+v (%v)", back, err)
	}
}

func TestBoolRuleEquals(t *testing.T) {
	r := BoolRule{Path: "status", Equals: "cancelled|canceled|abgesagt"}
	for in, want := range map[string]bool{
		`{"status":"Cancelled"}`: true,
		`{"status":"abgesagt"}`:  true,
		`{"status":"scheduled"}`: false,
		`{}`:                     false,
	} {
		if got := r.resolve(item(t, in)); got != want {
			t.Errorf("%s -> %v, want %v", in, got, want)
		}
	}
}

func TestStartGermanMonthsAndDateOnly(t *testing.T) {
	m := SourceMapping{
		Title: FieldRule{Path: "t"},
		Start: FieldRule{Template: "{d} {m} {y} {s}", Format: "2 Jan 2006 15:04|2 Jan 2006"},
		End:   FieldRule{Template: "{d} {m} {y} {e}", Format: "2 Jan 2006 15:04|2 Jan 2006"},
	}
	tests := []struct{ in, date, start, end string }{
		{`{"t":"a","d":"20","m":"Nov.","y":"2026","s":"18:00","e":"20:00"}`, "2026-11-20", "18:00", "20:00"},
		{`{"t":"b","d":"3","m":"März","y":"2027","s":"9:30","e":""}`, "2027-03-03", "09:30", ""},
		{`{"t":"c","d":"1","m":"Dez","y":"2026","s":"","e":""}`, "2026-12-01", "", ""},                // all-day
		{`{"t":"d","d":"6","m":"Mai","y":"2027","s":"20:00","e":"19:00"}`, "2027-05-06", "20:00", ""}, // end before start ignored
		{`{"t":"e","d":"14","m":"Januar","y":"2027","s":"16:00","e":"17:30"}`, "2027-01-14", "16:00", "17:30"},
	}
	for _, tt := range tests {
		ev := BuildEvent(item(t, tt.in), &m, "s", time.Now())
		if ev.Date != tt.date || ev.StartTime != tt.start || ev.EndTime != tt.end {
			t.Errorf("%s -> %s %q %q, want %s %q %q", tt.in, ev.Date, ev.StartTime, ev.EndTime, tt.date, tt.start, tt.end)
		}
	}
}

func TestGermanMonthsLeaveWordsAlone(t *testing.T) {
	if got := germanMonthsToEnglish("Maifest am 1. Mai"); got != "Maifest am 1. May" {
		t.Errorf("got %q", got)
	}
}

func TestTemplateFallback(t *testing.T) {
	r := FieldRule{Template: "{d} {beginn|einlass}"}
	for in, want := range map[string]string{
		`{"d":"1.2.","beginn":"20:00","einlass":"19:00"}`: "1.2. 20:00",
		`{"d":"1.2.","beginn":"","einlass":"19:00"}`:      "1.2. 19:00",
		`{"d":"1.2."}`: "1.2.",
	} {
		if got := r.resolveString(item(t, in)); got != want {
			t.Errorf("%s -> %q, want %q", in, got, want)
		}
	}
}

func TestInferYear(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"20.11.": "2026-11-20", // today
		"02.11.": "2026-11-02", // a few weeks back: still this year (dropped later as past)
		"12.10.": "2027-10-12", // more than a month back: next year
		"12.01.": "2027-01-12",
		"24.12.": "2026-12-24",
	} {
		p, _ := time.Parse("02.01.", in)
		if got := inferYear(p, now).Format("2006-01-02"); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestPriceDecimalComma(t *testing.T) {
	r := PriceRule{AmountPath: "p", UnknownIfMissing: true}
	if p := r.resolve(item(t, `{"p":"19,62"}`)); p.Free || p.Amount == nil || *p.Amount != 19.62 {
		t.Errorf("19,62 -> %+v", p)
	}
	if p := r.resolve(item(t, `{"p":"frei"}`)); !p.Free {
		t.Errorf("a non-number like 'frei' should count as 0 = free, got %+v", p)
	}
}

func TestRegexValue(t *testing.T) {
	cases := []struct{ re, in, want string }{
		{`Beginn (?P<value>\d{2}:\d{2})`, "Einlass 18:00, Beginn 19:00", "19:00"},
		{`(\d+)\s*€|Eintritt (frei)`, "Eintritt frei", "frei"},
		{`(\d+)\s*€|Eintritt (frei)`, "ab 27 €", "27"},
		{`\d{2}:\d{2}`, "um 19:30", "19:30"},
		{`(x)`, "nichts", ""},
		// Parts from different places combined: date from the title, time
		// from the text; the time is optional.
		{`(?s)(\d{1,2}\.\d{1,2}\.\d{2,4}).*?\|(?:.*?(\d{1,2}(?::\d{2})?)\s*Uhr)?`, "21.11.26 jazz | <h4>20 Uhr, Aula</h4>", "21.11.26 20"},
		{`(?s)(\d{1,2}\.\d{1,2}\.\d{2,4}).*?\|(?:.*?(\d{1,2}(?::\d{2})?)\s*Uhr)?`, "21.11.26 jazz | ohne Zeit", "21.11.26"},
	}
	for _, c := range cases {
		if got := RegexValue(regexp.MustCompile(c.re), c.in); got != c.want {
			t.Errorf("RegexValue(%q, %q) = %q, want %q", c.re, c.in, got, c.want)
		}
	}
}

func TestPriceAmountRegex(t *testing.T) {
	r := PriceRule{AmountPath: "text", AmountRegex: `(?i)eintritt\W{0,3}(frei)|(\d+(?:,\d+)?)\s*€`, UnknownIfMissing: true}
	cases := []struct {
		text    string
		free    bool
		amount  float64
		unknown bool
	}{
		{"<p>Eintritt frei</p>", true, 0, false},
		{"Tickets 12,50 € an der Abendkasse", false, 12.5, false},
		{"Eintritt 10-15 Euro", false, 0, true},
	}
	for _, c := range cases {
		p := r.resolve(item(t, `{"text":"`+c.text+`"}`))
		switch {
		case c.unknown:
			if p.Free || p.Amount != nil {
				t.Errorf("%q: want unknown, got %+v", c.text, p)
			}
		case c.free:
			if !p.Free {
				t.Errorf("%q: want free, got %+v", c.text, p)
			}
		default:
			if p.Amount == nil || *p.Amount != c.amount {
				t.Errorf("%q: want %v, got %+v", c.text, c.amount, p)
			}
		}
	}
}

func TestPlaceRef(t *testing.T) {
	// Arrange
	m := &SourceMapping{PlaceRef: FieldRule{Template: "brotfabrik:{venue.id}"}}

	// Act
	ev := BuildEvent(item(t, `{"venue":{"id":42}}`), m, "brotfabrik", time.Now())

	// Assert
	if ev.Place != "brotfabrik:42" {
		t.Errorf("place = %q", ev.Place)
	}
	if data, _ := json.Marshal(m); string(data) != `{"placeRef":{"template":"brotfabrik:{venue.id}"}}` {
		t.Errorf("marshal = %s", data)
	}
}
