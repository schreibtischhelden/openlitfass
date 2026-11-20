package htmlsel

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const page = `<html><body>
<div id="list">
  <div class="item big" data-id="1">
    <h2>Erster</h2>
    <p>Kategorie</p><p>18:00 - 20:00 Uhr</p><p><strong>Saal</strong></p>
    <a class="more" href="/e/1">mehr</a>
  </div>
  <div class="item" data-id="2">
    <h2>Zweiter</h2>
    <p>Kat</p><p>10:00 Uhr</p><p><strong></strong></p>
  </div>
  <section><div class="item">verschachtelt</div></section>
</div>
<div class="item">außerhalb</div>
</body></html>`

func doc(t *testing.T) *html.Node {
	t.Helper()
	n, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func texts(nodes []*html.Node) []string {
	var out []string
	for _, n := range nodes {
		out = append(out, Text(n))
	}
	return out
}

func TestSelectors(t *testing.T) {
	d := doc(t)
	list := MustParse("#list").Query(d)
	tests := []struct {
		sel  string
		root *html.Node
		want []string
	}{
		{"h2", d, []string{"Erster", "Zweiter"}},
		{"div.item > h2", d, []string{"Erster", "Zweiter"}},
		{"#list > div.item", d, []string{"Erster Kategorie 18:00 - 20:00 Uhr Saal mehr", "Zweiter Kat 10:00 Uhr"}},
		{"#list .item", d, []string{"Erster Kategorie 18:00 - 20:00 Uhr Saal mehr", "Zweiter Kat 10:00 Uhr", "verschachtelt"}},
		{"div.item.big h2", d, []string{"Erster"}},
		{"[data-id=2] h2", d, []string{"Zweiter"}},
		{"div[data-id] p:nth-of-type(2)", d, []string{"18:00 - 20:00 Uhr", "10:00 Uhr"}},
		{"p:first-of-type", list, []string{"Kategorie", "Kat"}},
		{"a[href*=/e/]", d, []string{"mehr"}},
		{"div[class~=big] strong", d, []string{"Saal"}},
		{".item", list, []string{"Erster Kategorie 18:00 - 20:00 Uhr Saal mehr", "Zweiter Kat 10:00 Uhr", "verschachtelt"}},
	}
	for _, tt := range tests {
		got := texts(MustParse(tt.sel).QueryAll(tt.root))
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%q: got %q, want %q", tt.sel, got, tt.want)
		}
	}
}

func TestScopeDoesNotLeak(t *testing.T) {
	d := doc(t)
	first := MustParse("div.item").Query(d)
	// "#list p" must not match inside an item scope: #list is outside it.
	if got := MustParse("#list p").QueryAll(first); len(got) != 0 {
		t.Errorf("selector reached outside its scope: %q", texts(got))
	}
	if got := Attr(MustParse("a.more").Query(first), "href"); got != "/e/1" {
		t.Errorf("href = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"", "a,b", "a + b", "> a", "a >", "p:hover", "p:nth-of-type(odd)", "[", ".", "#"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) should fail", bad)
		}
	}
}
