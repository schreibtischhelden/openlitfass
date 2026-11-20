package mapping

import (
	"encoding/json"
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
		ev := BuildEvent(item(t, tt.in), m, "s", time.Now())
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
