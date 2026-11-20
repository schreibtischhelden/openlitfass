package mapping

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxDescriptionRunes caps Event.Description. Some sources (WordPress
// event plugins especially) ship the full article body; the frontend only
// needs enough to decide whether to click through to the source.
const MaxDescriptionRunes = 1200

var (
	// In HTML input, the end of a block becomes a paragraph break and <br>
	// a line break before all tags are stripped, so paragraphs survive (the
	// frontend renders descriptions with white-space: pre-line). Raw
	// newlines in HTML are just whitespace.
	blockEndRe   = regexp.MustCompile(`(?i)<\s*(/p|/div|/li|/h[1-6]|/tr|/ul|/ol|/blockquote)\s*>`)
	lineBreakRe  = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	looksHTMLRe  = regexp.MustCompile(`(?i)<\s*/?\s*(p|br|div|li|ul|ol|h[1-6]|strong|em|b|i|a|span|tr|td|blockquote)\b[^>]*>`)
	tagRe        = regexp.MustCompile(`<[^>]*>`)
	spacesRe     = regexp.MustCompile(`[ \t\f\v\x{00A0}]+`)
	blankLinesRe = regexp.MustCompile(`\n\s*\n\s*\n+`)
)

// CleanText turns a possibly HTML-encoded string from a feed or API into
// plain text: tags removed, entities decoded (&#8211; -> –, &amp; -> &),
// soft hyphens and zero-width characters dropped, whitespace collapsed to
// single spaces. Used for every single-line field (title, organizer,
// location, ...).
//
// Applied unconditionally to rss/json sources: Event fields are plain text
// by contract (the frontend inserts them via textContent/escaping), so a
// literal "<" in a title is the only thing lost, while HTML-in-the-feed is
// common.
func CleanText(s string) string {
	if s == "" {
		return s
	}
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = stripInvisible(s)
	s = spacesRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// CleanMultiline is CleanText for descriptions: like CleanText, but
// paragraph and line breaks are kept (at most one blank line in a row) and
// the result is capped at MaxDescriptionRunes with an ellipsis.
func CleanMultiline(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if looksHTMLRe.MatchString(s) {
		s = strings.ReplaceAll(s, "\n", " ")
		s = blockEndRe.ReplaceAllString(s, "\n\n")
		s = lineBreakRe.ReplaceAllString(s, "\n")
	}
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = stripInvisible(s)
	s = spacesRe.ReplaceAllString(s, " ")

	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = strings.Join(lines, "\n")
	s = blankLinesRe.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	return truncateRunes(s, MaxDescriptionRunes)
}

// stripInvisible removes soft hyphens (U+00AD, e.g. "Cali­ga­ri­platz") and
// zero-width characters that some CMSs insert for hyphenation/layout.
func stripInvisible(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 0x00AD, 0x200B, 0x200C, 0x200D, 0x2060, 0xFEFF:
			return -1
		}
		return r
	}, s)
}

// truncateRunes cuts s to at most max runes, preferring the last word
// boundary, and appends "…" if anything was cut.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)[:max]
	cut := string(r)
	if i := strings.LastIndexAny(cut, " \n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n,;:.-–") + " …"
}
