package mapping

import (
	"encoding/json"
	"testing"
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
