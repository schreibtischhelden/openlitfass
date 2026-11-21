package app

import (
	"html/template"
	"strings"
	"testing"
)

func TestMakeFavicon(t *testing.T) {
	logo := `<svg class="logo-mark" viewBox="0 0 24 24" width="26" height="26" aria-hidden="true">
  <path d="M1 1h2" stroke="currentColor"/>
  <rect x="9" y="9" width="5" height="7" class="logo-mark__accent"/>
</svg>`
	got, err := makeFavicon(logoHTML(logo), "#17383D")
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		`xmlns="http://www.w3.org/2000/svg"`,
		`viewBox="0 0 24 24"`,
		`fill="#17383D"`,
		`.logo-mark__accent{fill:` + faviconAccent,
		`style="color:#fff"`,
		`<path d="M1 1h2" stroke="currentColor"/>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("favicon lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "aria-hidden") {
		t.Error("the logo's own <svg> tag should be replaced")
	}
	// Invalid colour falls back to the default; non-SVG input is an error.
	if got, _ := makeFavicon(logoHTML(logo), "red; x"); !strings.Contains(string(got), defaultFaviconBackground) {
		t.Error("invalid themeColor should fall back to the default")
	}
	if _, err := makeFavicon(logoHTML("<p>kein svg</p>"), ""); err == nil {
		t.Error("expected an error for a logo that is not SVG")
	}
}

func logoHTML(s string) template.HTML { return template.HTML(s) }
