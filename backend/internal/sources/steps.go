package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	"openlitfass/backend/internal/config"
	"openlitfass/backend/internal/htmlsel"
	"openlitfass/backend/internal/mapping"
	"openlitfass/backend/internal/model"
)

// defaultMaxRequests caps the main requests of a forEach source per run.
const defaultMaxRequests = 20

// defaultStepDays is the length of the {today}..{until} window when a
// source sets no daysAhead.
const defaultStepDays = 30

// usesSteps reports whether cfg needs stepSource: preparatory steps, a
// forEach main request or placeholders in the main URL.
func usesSteps(cfg *config.SourceConfig) bool {
	return len(cfg.Steps) > 0 || cfg.ForEach != nil || urlPlaceholderRe.MatchString(cfg.URL)
}

// stepSource runs cfg.Steps, expands the main URL (once, or once per
// forEach request) and hands each resulting URL to an ordinary source of
// cfg.Type built by build. The typed sources stay unaware of steps.
type stepSource struct {
	cfg   config.SourceConfig
	build func(config.SourceConfig) Source
}

func newStepSource(cfg config.SourceConfig, build func(config.SourceConfig) Source) *stepSource {
	return &stepSource{cfg: cfg, build: build}
}

func (s *stepSource) Name() string { return s.cfg.Name }

func (s *stepSource) Fetch(ctx context.Context) ([]model.Event, error) {
	vars := baseVars(s.cfg.DaysAhead, time.Now())
	for i, st := range s.cfg.Steps {
		if err := runStep(ctx, st, vars); err != nil {
			return nil, fmt.Errorf("step %d: %w", i+1, err)
		}
	}

	urls, err := mainURLs(&s.cfg, vars)
	if err != nil {
		return nil, err
	}
	maxReq := s.cfg.MaxRequests
	if maxReq <= 0 {
		maxReq = defaultMaxRequests
	}
	if len(urls) > maxReq {
		log.Printf("source %q: %d requests needed, maxRequests is %d - skipping the rest", s.cfg.Name, len(urls), maxReq)
		urls = urls[:maxReq]
	}

	var events []model.Event
	var firstErr error
	for i, u := range urls {
		if ctx.Err() != nil {
			// Out of time: keep what the earlier requests returned.
			log.Printf("source %q: time limit reached after %d of %d requests", s.cfg.Name, i, len(urls))
			if firstErr == nil {
				firstErr = ctx.Err()
			}
			break
		}
		c := s.cfg
		c.URL = u
		c.Steps, c.ForEach = nil, nil
		evs, err := s.build(c).Fetch(ctx)
		if err != nil {
			// One failing batch shouldn't throw away the others (same rule
			// as later pages of a json source).
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("source %q: request failed: %v", s.cfg.Name, err)
			continue
		}
		events = append(events, evs...)
	}
	if len(events) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return events, nil
}

// baseVars are the placeholders available without any step: {today} and
// {until} (today + daysAhead, instance-local, YYYY-MM-DD).
func baseVars(daysAhead int, now time.Time) map[string][]string {
	if daysAhead <= 0 {
		daysAhead = defaultStepDays
	}
	today := now.In(model.Local)
	return map[string][]string{
		"today": {today.Format("2006-01-02")},
		"until": {today.AddDate(0, 0, daysAhead).Format("2006-01-02")},
	}
}

func runStep(ctx context.Context, st config.Step, vars map[string][]string) error {
	if len(st.Collect) == 0 {
		return errors.New(`"collect" is empty`)
	}
	u, err := expandURL(st.URL, vars, nil)
	if err != nil {
		return err
	}
	accept := "text/html, */*;q=0.5"
	if st.Type == "json" {
		accept = "application/json"
	}
	body, err := fetchBody(ctx, u, accept)
	if err != nil {
		return err
	}

	var collected map[string][]string
	switch st.Type {
	case "", "html":
		collected, err = collectHTML(body, u, st.Collect)
	case "json":
		collected, err = collectJSON(body, st.Collect)
	default:
		return fmt.Errorf("unknown step type %q", st.Type)
	}
	if err != nil {
		return err
	}
	for name, vals := range collected {
		if len(vals) == 0 {
			return fmt.Errorf("collect %q found nothing on %s", name, u)
		}
		vars[name] = vals
	}
	return nil
}

func fetchBody(ctx context.Context, u, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", u, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

type compiledCollect struct {
	rule config.CollectRule
	re   *regexp.Regexp
}

func compileCollect(name string, r config.CollectRule) (compiledCollect, error) {
	c := compiledCollect{rule: r}
	if r.Regex != "" {
		re, err := regexp.Compile(r.Regex)
		if err != nil {
			return c, fmt.Errorf("collect %q: regex: %w", name, err)
		}
		c.re = re
	}
	return c, nil
}

// finish applies regex and split to raw values, trims them, drops empty
// ones and removes duplicates.
func (c compiledCollect) finish(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range raw {
		if c.re != nil {
			v = mapping.RegexValue(c.re, v)
		}
		parts := []string{v}
		if c.rule.Split != "" {
			parts = strings.Split(v, c.rule.Split)
		}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" && !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func collectHTML(body []byte, pageURL string, rules map[string]config.CollectRule) (map[string][]string, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	base, _ := url.Parse(pageURL)
	out := make(map[string][]string, len(rules))
	for name, r := range rules {
		if r.Selector == "" {
			return nil, fmt.Errorf("collect %q: \"selector\" is required", name)
		}
		sel, err := htmlsel.Parse(r.Selector)
		if err != nil {
			return nil, fmt.Errorf("collect %q: %w", name, err)
		}
		cc, err := compileCollect(name, r)
		if err != nil {
			return nil, err
		}
		var raw []string
		for _, n := range sel.QueryAll(doc) {
			v := htmlsel.Text(n)
			if r.Attr != "" {
				v = strings.TrimSpace(htmlsel.Attr(n, r.Attr))
				if v != "" && base != nil && (r.Attr == "href" || r.Attr == "src") {
					if abs, err := base.Parse(v); err == nil {
						v = abs.String()
					}
				}
			}
			raw = append(raw, v)
		}
		out[name] = cc.finish(raw)
	}
	return out, nil
}

func collectJSON(body []byte, rules map[string]config.CollectRule) (map[string][]string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // keep ids like 21063490 from turning into 2.106349e+07
	var root interface{}
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	out := make(map[string][]string, len(rules))
	for name, r := range rules {
		if r.Path == "" {
			return nil, fmt.Errorf("collect %q: \"path\" is required", name)
		}
		cc, err := compileCollect(name, r)
		if err != nil {
			return nil, err
		}
		var raw []string
		for _, v := range jsonValues(root, strings.Split(r.Path, ".")) {
			raw = append(raw, mapping.String(v, ""))
		}
		out[name] = cc.finish(raw)
	}
	return out, nil
}

// jsonValues resolves a dot-path where "*" stands for every array element;
// a path ending on an array yields its elements.
func jsonValues(node interface{}, path []string) []interface{} {
	if node == nil {
		return nil
	}
	if len(path) == 0 {
		if list, ok := node.([]interface{}); ok {
			return list
		}
		return []interface{}{node}
	}
	if path[0] == "*" {
		list, ok := node.([]interface{})
		if !ok {
			return nil
		}
		var out []interface{}
		for _, el := range list {
			out = append(out, jsonValues(el, path[1:])...)
		}
		return out
	}
	v, ok := mapping.Get(node, path[0])
	if !ok {
		return nil
	}
	return jsonValues(v, path[1:])
}

var urlPlaceholderRe = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// expandURL replaces {name} by the first value of vars[name] (or by item
// for {item}). A placeholder that is the whole URL is inserted as it is (a
// collected link); everywhere else values are query-escaped. Unknown
// placeholders are an error, so a typo doesn't turn into a request to a
// wrong URL.
func expandURL(tmpl string, vars map[string][]string, item *string) (string, error) {
	if tmpl == "" {
		return "", errors.New(`"url" is empty`)
	}
	whole := urlPlaceholderRe.FindString(tmpl) == tmpl
	var missing []string
	out := urlPlaceholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := m[1 : len(m)-1]
		var v string
		switch vals, ok := vars[name]; {
		case name == "item" && item != nil:
			v = *item
		case ok && len(vals) > 0:
			v = vals[0]
		default:
			missing = append(missing, m)
			return m
		}
		if whole {
			return v
		}
		return url.QueryEscape(v)
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("url %q: unknown placeholder %s", tmpl, strings.Join(missing, ", "))
	}
	return out, nil
}

// mainURLs returns the URLs of the main request: one, or one per forEach
// value or batch.
func mainURLs(cfg *config.SourceConfig, vars map[string][]string) ([]string, error) {
	fe := cfg.ForEach
	if fe == nil {
		u, err := expandURL(cfg.URL, vars, nil)
		if err != nil {
			return nil, err
		}
		return []string{u}, nil
	}

	vals, ok := vars[fe.Var]
	if !ok {
		return nil, fmt.Errorf("forEach: no variable %q (collect it in a step)", fe.Var)
	}

	if fe.Param == "" {
		if !strings.Contains(cfg.URL, "{item}") {
			return nil, errors.New(`forEach without "param" needs {item} in "url"`)
		}
		urls := make([]string, 0, len(vals))
		for i := range vals {
			u, err := expandURL(cfg.URL, vars, &vals[i])
			if err != nil {
				return nil, err
			}
			urls = append(urls, u)
		}
		return urls, nil
	}

	base, err := expandURL(cfg.URL, vars, nil)
	if err != nil {
		return nil, err
	}
	batch := fe.Batch
	if batch <= 0 {
		batch = 1
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
		if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
			sep = ""
		}
	}
	key := url.QueryEscape(fe.Param)
	var urls []string
	for start := 0; start < len(vals); start += batch {
		end := min(start+batch, len(vals))
		var b strings.Builder
		b.WriteString(base)
		for i, v := range vals[start:end] {
			if i == 0 {
				b.WriteString(sep)
			} else {
				b.WriteByte('&')
			}
			b.WriteString(key)
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
		urls = append(urls, b.String())
	}
	return urls, nil
}
