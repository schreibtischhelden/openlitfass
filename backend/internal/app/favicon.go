package app

import (
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"strings"
)

// FaviconFile is the stand-alone icon build-site writes next to index.html.
const FaviconFile = "favicon.svg"

// Colours of the generated favicon when the instance sets no themeColor:
// the core's primary and accent (web/css/tokens.css).
const (
	defaultFaviconBackground = "#2F3E4E"
	faviconAccent            = "#B4532A"
)

var (
	svgOpenTag = regexp.MustCompile(`(?s)<svg\b[^>]*>`)
	cssColor   = regexp.MustCompile(`^#[0-9A-Fa-f]{3,8}$`)
)

// makeFavicon turns the instance logo into a stand-alone SVG icon. The logo
// itself is written for inlining into the page (no xmlns, colours from
// currentColor and the theme's .logo-mark__accent class), which browsers
// don't render as an image file. The icon wraps the logo's drawing in a
// rounded square in the theme colour, draws it white and fills the accent
// class itself - readable on light and dark tab bars.
func makeFavicon(logo template.HTML, themeColor string) ([]byte, error) {
	s := string(logo)
	open := svgOpenTag.FindStringIndex(s)
	end := strings.LastIndex(s, "</svg>")
	if len(open) != 2 || end < open[1] {
		return nil, errors.New("logo is not an <svg> element")
	}
	viewBox := "0 0 24 24"
	if m := regexp.MustCompile(`viewBox="([^"]+)"`).FindStringSubmatch(s[open[0]:open[1]]); m != nil {
		viewBox = m[1]
	}
	bg := themeColor
	if !cssColor.MatchString(bg) {
		bg = defaultFaviconBackground
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%s" width="32" height="32">`, viewBox)
	fmt.Fprintf(&b, `<style>.logo-mark__accent{fill:%s}</style>`, faviconAccent)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="100%%" height="100%%" rx="5" fill="%s"/>`, bg)
	b.WriteString(`<g style="color:#fff">`)
	b.WriteString(strings.TrimSpace(s[open[1]:end]))
	b.WriteString("</g></svg>\n")
	return []byte(b.String()), nil
}
