// Package htmlsel is a small CSS selector engine on top of
// golang.org/x/net/html - just enough for scraping event listings with the
// "html" source type, without pulling in a full CSS/jQuery library.
//
// Supported:
//
//	tag  *  .class  #id  [attr]  [attr=value]  [attr~=word]  [attr*=part]
//	:first-of-type  :nth-of-type(n)
//	descendant ("a b") and child ("a > b") combinators
//
// Several simple selectors can be combined in one compound
// ("div.event.big[data-id]"). Selector lists (","), sibling combinators and
// other pseudo-classes are not supported and return an error.
package htmlsel

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

type attrCond struct {
	name, op, value string // op: "" (exists), "=", "~=", "*="
}

type compound struct {
	tag       string // "" or "*" = any
	id        string
	classes   []string
	attrs     []attrCond
	nthOfType int // 0 = no constraint
}

type step struct {
	c     compound
	child bool // combinator to the previous step is ">" (else descendant)
}

// Selector is a parsed selector, matched right to left.
type Selector struct {
	steps []step
	src   string
}

func (s Selector) String() string { return s.src }

// MustParse is Parse for selectors known to be valid (tests, constants).
func MustParse(src string) Selector {
	s, err := Parse(src)
	if err != nil {
		panic(err)
	}
	return s
}

// Parse parses a selector.
func Parse(src string) (Selector, error) {
	sel := Selector{src: src}
	s := strings.TrimSpace(src)
	if s == "" {
		return sel, errors.New("empty selector")
	}
	child := false
	for len(s) > 0 {
		s = strings.TrimLeft(s, " \t\n")
		if strings.HasPrefix(s, ">") {
			if len(sel.steps) == 0 || child {
				return sel, fmt.Errorf("selector %q: misplaced '>'", src)
			}
			child = true
			s = s[1:]
			continue
		}
		if s == "" {
			break
		}
		c, rest, err := parseCompound(s)
		if err != nil {
			return sel, fmt.Errorf("selector %q: %w", src, err)
		}
		sel.steps = append(sel.steps, step{c: c, child: child})
		child = false
		s = rest
	}
	if child {
		return sel, fmt.Errorf("selector %q: dangling '>'", src)
	}
	return sel, nil
}

func isIdentChar(r byte) bool {
	return r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func readIdent(s string) (string, string) {
	i := 0
	for i < len(s) && isIdentChar(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func parseCompound(s string) (compound, string, error) {
	var c compound
	if strings.HasPrefix(s, "*") {
		c.tag, s = "*", s[1:]
	} else if len(s) > 0 && isIdentChar(s[0]) {
		c.tag, s = readIdent(s)
		c.tag = strings.ToLower(c.tag)
	}
	for len(s) > 0 {
		switch s[0] {
		case '.':
			var name string
			name, s = readIdent(s[1:])
			if name == "" {
				return c, s, errors.New("empty class name")
			}
			c.classes = append(c.classes, name)
		case '#':
			var name string
			name, s = readIdent(s[1:])
			if name == "" {
				return c, s, errors.New("empty id")
			}
			c.id = name
		case '[':
			end := strings.IndexByte(s, ']')
			if end < 0 {
				return c, s, errors.New("unclosed '['")
			}
			body := s[1:end]
			s = s[end+1:]
			cond := attrCond{}
			for _, op := range []string{"~=", "*=", "="} {
				if i := strings.Index(body, op); i >= 0 {
					cond.name, cond.op, cond.value = body[:i], op, body[i+len(op):]
					break
				}
			}
			if cond.op == "" {
				cond.name = body
			}
			cond.name = strings.ToLower(strings.TrimSpace(cond.name))
			cond.value = strings.Trim(strings.TrimSpace(cond.value), `"'`)
			if cond.name == "" {
				return c, s, errors.New("empty attribute name")
			}
			c.attrs = append(c.attrs, cond)
		case ':':
			var name string
			name, s = readIdent(s[1:])
			switch name {
			case "first-of-type":
				c.nthOfType = 1
			case "nth-of-type":
				if !strings.HasPrefix(s, "(") {
					return c, s, errors.New(":nth-of-type needs (n)")
				}
				end := strings.IndexByte(s, ')')
				if end < 0 {
					return c, s, errors.New("unclosed :nth-of-type(")
				}
				n, err := strconv.Atoi(strings.TrimSpace(s[1:end]))
				if err != nil || n < 1 {
					return c, s, errors.New(":nth-of-type only supports a plain number")
				}
				c.nthOfType = n
				s = s[end+1:]
			default:
				return c, s, fmt.Errorf("pseudo-class :%s not supported", name)
			}
		case ' ', '\t', '\n', '>':
			return c, s, nil
		default:
			return c, s, fmt.Errorf("unexpected %q", s[0])
		}
	}
	return c, s, nil
}

func attr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val, true
		}
	}
	return "", false
}

// Attr returns the value of attribute name on n ("" if absent).
func Attr(n *html.Node, name string) string {
	v, _ := attr(n, name)
	return v
}

func (c compound) match(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	if c.tag != "" && c.tag != "*" && n.Data != c.tag {
		return false
	}
	if c.id != "" && Attr(n, "id") != c.id {
		return false
	}
	if len(c.classes) > 0 {
		have := strings.Fields(Attr(n, "class"))
		for _, want := range c.classes {
			found := false
			for _, h := range have {
				if h == want {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	for _, a := range c.attrs {
		v, ok := attr(n, a.name)
		if !ok {
			return false
		}
		switch a.op {
		case "=":
			if v != a.value {
				return false
			}
		case "~=":
			found := false
			for _, w := range strings.Fields(v) {
				if w == a.value {
					found = true
				}
			}
			if !found {
				return false
			}
		case "*=":
			if !strings.Contains(v, a.value) {
				return false
			}
		}
	}
	if c.nthOfType > 0 {
		idx := 0
		for s := n.Parent.FirstChild; s != nil; s = s.NextSibling {
			if s.Type == html.ElementNode && s.Data == n.Data {
				idx++
			}
			if s == n {
				break
			}
		}
		if idx != c.nthOfType {
			return false
		}
	}
	return true
}

// Match reports whether n matches the selector. Ancestors are only
// considered up to (excluding) scope's parent when scope is non-nil, so
// selectors used inside an item can't reach outside it.
func (s Selector) Match(n *html.Node, scope *html.Node) bool {
	return s.matchAt(len(s.steps)-1, n, scope)
}

func (s Selector) matchAt(i int, n *html.Node, scope *html.Node) bool {
	if !s.steps[i].c.match(n) {
		return false
	}
	if i == 0 {
		return true
	}
	limit := (*html.Node)(nil)
	if scope != nil {
		limit = scope.Parent
	}
	if s.steps[i].child {
		p := n.Parent
		return p != nil && p != limit && s.matchAt(i-1, p, scope)
	}
	for p := n.Parent; p != nil && p != limit; p = p.Parent {
		if s.matchAt(i-1, p, scope) {
			return true
		}
	}
	return false
}

// QueryAll returns all descendants of root (not root itself) matching s,
// in document order. Matching is scoped to root.
func (s Selector) QueryAll(root *html.Node) []*html.Node {
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if s.Match(c, root) {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(root)
	return out
}

// Query returns the first match or nil.
func (s Selector) Query(root *html.Node) *html.Node {
	if all := s.QueryAll(root); len(all) > 0 {
		return all[0]
	}
	return nil
}

// Text returns the text content of n with whitespace collapsed.
func Text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			if n.Data == "script" || n.Data == "style" {
				return
			}
			if n.Data == "br" {
				b.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && (n.Data == "p" || n.Data == "div" || n.Data == "li") {
			b.WriteString(" ")
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// InnerHTML renders the children of n back to HTML.
func InnerHTML(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}
