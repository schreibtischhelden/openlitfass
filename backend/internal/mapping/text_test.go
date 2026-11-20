package mapping

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCleanText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"Plain title", "Plain title"},
		{"Bike the Line &#8211; 80 Jahre", "Bike the Line – 80 Jahre"},
		{"Lena Tietgen, Elke Cremer &#038; Ingeborg Robles", "Lena Tietgen, Elke Cremer & Ingeborg Robles"},
		{"Cali­ga­ri­platz 1", "Caligariplatz 1"},
		{"  <b>Fett</b>   und\n<i>kursiv</i> ", "Fett und kursiv"},
		{"Harald Naegeli  – Der Sprayer", "Harald Naegeli – Der Sprayer"},
		{"A B", "A B"},
	}
	for _, tt := range tests {
		if got := CleanText(tt.in); got != tt.want {
			t.Errorf("CleanText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanMultilineKeepsParagraphs(t *testing.T) {
	in := "<p class=\"p2\"><strong>16.10. – 29.11.26</strong></p>\n<div><b>Geräusche</b></div>\n<div></div>\n<div><i>Soundprojekt</i><br />\nzweite Zeile</div>"
	// Raw newlines in HTML are whitespace; <br> is a line break, block ends
	// are paragraph breaks.
	want := "16.10. – 29.11.26\n\nGeräusche\n\nSoundprojekt\nzweite Zeile"
	if got := CleanMultiline(in); got != want {
		t.Errorf("CleanMultiline =\n%q\nwant\n%q", got, want)
	}
}

func TestCleanMultilineTruncates(t *testing.T) {
	in := strings.Repeat("Wort ", 1000)
	got := CleanMultiline(in)
	if n := utf8.RuneCountInString(got); n > MaxDescriptionRunes+2 {
		t.Fatalf("length %d exceeds cap", n)
	}
	if !strings.HasSuffix(got, " …") {
		t.Errorf("truncated text should end with an ellipsis, got %q", got[len(got)-10:])
	}
}
