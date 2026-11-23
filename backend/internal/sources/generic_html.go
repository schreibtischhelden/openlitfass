package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/htmlsel"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

// HTMLSource scrapes event listings from a web page without any feed:
// itemSelector finds each event container, extract pulls named values out
// of it (text, attribute or inner HTML, optionally narrowed by regex), and
// the regular mapping block turns those values into an Event - the same
// mapping engine as for json sources, with the extract names as paths.
//
// Site-specific knowledge stays in the instance's source file; this type is
// generic.
type HTMLSource struct {
	cfg config.SourceConfig
}

func NewHTMLSource(cfg config.SourceConfig) *HTMLSource {
	return &HTMLSource{cfg: cfg}
}

func (s *HTMLSource) Name() string { return s.cfg.Name }

func (s *HTMLSource) Fetch(ctx context.Context) ([]model.Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html, */*;q=0.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.cfg.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", s.cfg.URL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return parseHTMLEvents(string(body), s.cfg, time.Now().UTC())
}

type compiledExtract struct {
	name string
	rule config.ExtractRule
	sel  *htmlsel.Selector
	re   *regexp.Regexp
}

func parseHTMLEvents(body string, cfg config.SourceConfig, now time.Time) ([]model.Event, error) {
	if cfg.ItemSelector == "" {
		return nil, fmt.Errorf(`html source %q: "itemSelector" is required`, cfg.Name)
	}
	itemSel, err := htmlsel.Parse(cfg.ItemSelector)
	if err != nil {
		return nil, fmt.Errorf("itemSelector: %w", err)
	}

	// Compile extract rules once, in a stable order (map order is random).
	names := make([]string, 0, len(cfg.Extract))
	for n := range cfg.Extract {
		names = append(names, n)
	}
	sort.Strings(names)
	var rules []compiledExtract
	for _, n := range names {
		r := cfg.Extract[n]
		ce := compiledExtract{name: n, rule: r}
		if r.Selector != "" {
			sel, err := htmlsel.Parse(r.Selector)
			if err != nil {
				return nil, fmt.Errorf("extract %q: %w", n, err)
			}
			ce.sel = &sel
		}
		if r.Regex != "" {
			re, err := regexp.Compile(r.Regex)
			if err != nil {
				return nil, fmt.Errorf("extract %q: regex: %w", n, err)
			}
			ce.re = re
		}
		rules = append(rules, ce)
	}

	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	base, _ := url.Parse(cfg.URL)

	m := mapping.SourceMapping{}
	if cfg.Mapping != nil {
		m = *cfg.Mapping
	}

	items := itemSel.QueryAll(doc)
	if len(items) == 0 {
		// Most likely the site changed its markup - make it visible in the
		// source's status instead of silently returning nothing.
		return nil, fmt.Errorf("itemSelector %q matched nothing on %s", cfg.ItemSelector, cfg.URL)
	}

	var sections []string
	if cfg.SectionSelector != "" {
		secSel, err := htmlsel.Parse(cfg.SectionSelector)
		if err != nil {
			return nil, fmt.Errorf("sectionSelector: %w", err)
		}
		sections = precedingSections(doc, items, secSel)
	}

	events := make([]model.Event, 0, len(items))
	for i, it := range items {
		values := map[string]interface{}{}
		if sections != nil {
			values[sectionValue] = sections[i]
		}
		for _, r := range rules {
			values[r.name] = extractValue(it, r, base)
		}
		events = append(events, mapping.BuildEvent(values, &m, cfg.Name, now))
	}
	return events, nil
}

// sectionValue is the mapping name under which an item's section heading
// (cfg.SectionSelector) is available.
const sectionValue = "section"

// precedingSections returns, for each item (in document order), the text
// of the last node matching sec that comes before it in document order -
// "" for items before the first heading. Works for headings that are
// siblings of the items as well as for ones nested elsewhere before them.
func precedingSections(doc *html.Node, items []*html.Node, sec htmlsel.Selector) []string {
	isItem := make(map[*html.Node]int, len(items))
	for i, it := range items {
		isItem[it] = i
	}
	out := make([]string, len(items))
	current := ""
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if sec.Match(c, doc) {
				current = htmlsel.Text(c)
			}
			if i, ok := isItem[c]; ok {
				out[i] = current
			}
			walk(c)
		}
	}
	walk(doc)
	return out
}

func extractValue(item *html.Node, r compiledExtract, base *url.URL) string {
	var nodes []*html.Node
	switch {
	case r.sel == nil:
		nodes = []*html.Node{item}
	case r.rule.All:
		nodes = r.sel.QueryAll(item)
		if r.rule.Skip > 0 {
			if r.rule.Skip >= len(nodes) {
				nodes = nil
			} else {
				nodes = nodes[r.rule.Skip:]
			}
		}
	default:
		if n := r.sel.Query(item); n != nil {
			nodes = []*html.Node{n}
		}
	}

	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		var v string
		switch {
		case r.rule.Attr != "":
			v = strings.TrimSpace(htmlsel.Attr(n, r.rule.Attr))
			if v != "" && base != nil && (r.rule.Attr == "href" || r.rule.Attr == "src") {
				if u, err := base.Parse(v); err == nil {
					v = u.String()
				}
			}
		case r.rule.HTML:
			v = htmlsel.InnerHTML(n)
		default:
			v = htmlsel.Text(n)
		}
		if strings.TrimSpace(v) != "" {
			parts = append(parts, v)
		}
	}
	sep := "\n"
	if r.rule.HTML {
		sep = "</p>" // each match becomes its own paragraph
	}
	v := strings.Join(parts, sep)

	if r.re != nil && v != "" {
		v = mapping.RegexValue(r.re, v)
	}
	return strings.TrimSpace(v)
}
