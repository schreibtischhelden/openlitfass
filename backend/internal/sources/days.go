package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// dayPattern is one entry of an inbox "dates"/"except" list or of a day list
// in <instance>/holidays.json:
//
//	"2027-05-06"   once
//	"12-25"        every year
//	"*-01"         every month
//	"easter+1"     relative to Western Easter Sunday ("easter", "easter-2", ...)
type dayPattern struct {
	kind   dayKind
	year   int
	month  time.Month
	day    int
	offset int
}

type dayKind int

const (
	dayOnce dayKind = iota
	dayYearly
	dayMonthly
	dayEaster
)

func parseDayPattern(s string) (dayPattern, error) {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "easter"); ok {
		if rest == "" {
			return dayPattern{kind: dayEaster}, nil
		}
		if rest[0] == '+' || rest[0] == '-' {
			if n, err := strconv.Atoi(rest); err == nil {
				return dayPattern{kind: dayEaster, offset: n}, nil
			}
		}
		return dayPattern{}, fmt.Errorf(`invalid day %q (want "easter", "easter+N" or "easter-N")`, s)
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return dayPattern{kind: dayOnce, year: t.Year(), month: t.Month(), day: t.Day()}, nil
	}
	if d, ok := strings.CutPrefix(s, "*-"); ok {
		if n, err := strconv.Atoi(d); err == nil && len(d) == 2 && n >= 1 && n <= 31 {
			return dayPattern{kind: dayMonthly, day: n}, nil
		}
	}
	// 2000 is a leap year, so "02-29" is accepted.
	if t, err := time.Parse("2006-01-02", "2000-"+s); err == nil && len(s) == 5 {
		return dayPattern{kind: dayYearly, month: t.Month(), day: t.Day()}, nil
	}
	return dayPattern{}, fmt.Errorf(`invalid day %q (want YYYY-MM-DD, MM-DD, *-DD or easter±N)`, s)
}

func (p dayPattern) matches(t time.Time) bool {
	switch p.kind {
	case dayOnce:
		return t.Year() == p.year && t.Month() == p.month && t.Day() == p.day
	case dayYearly:
		return t.Month() == p.month && t.Day() == p.day
	case dayMonthly:
		return t.Day() == p.day
	case dayEaster:
		e := easterSunday(t.Year()).AddDate(0, 0, p.offset)
		return t.Month() == e.Month() && t.Day() == e.Day()
	}
	return false
}

// easterSunday returns Western (Gregorian) Easter Sunday of year, using the
// anonymous Gregorian algorithm (Meeus/Jones/Butcher).
func easterSunday(year int) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// dayLists are the named lists of <instance>/holidays.json, for example
// {"ostern": ["easter-2", "easter", "easter+1"], "feiertage": ["01-01", "ostern"]}.
// The instance maintains them itself; openLitfass knows no holidays of its
// own, only how to compute Easter.
type dayLists map[string][]string

// loadDayLists reads path. A missing file yields no lists, not an error.
func loadDayLists(path string) (dayLists, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var lists dayLists
	if err := json.Unmarshal(data, &lists); err != nil {
		return nil, fmt.Errorf("%s: invalid json: %w", path, err)
	}
	for name := range lists {
		if _, err := parseDayPattern(name); err == nil {
			return nil, fmt.Errorf("%s: list name %q looks like a day, pick another name", path, name)
		}
	}
	// Resolve every list once, so a typo or a cycle shows up even if no
	// inbox file uses that list yet.
	for name := range lists {
		if _, err := lists.expand([]string{name}); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return lists, nil
}

// expand turns entries (day patterns and list names, lists may contain other
// lists) into day patterns.
func (l dayLists) expand(entries []string) ([]dayPattern, error) {
	var out []dayPattern
	var walk func(entries []string, path []string) error
	walk = func(entries []string, path []string) error {
		for _, e := range entries {
			name := strings.TrimSpace(e)
			if sub, ok := l[name]; ok {
				for _, p := range path {
					if p == name {
						return fmt.Errorf("day list %q includes itself (%s)", name, strings.Join(append(path, name), " → "))
					}
				}
				if err := walk(sub, append(path, name)); err != nil {
					return err
				}
				continue
			}
			p, err := parseDayPattern(name)
			if err != nil {
				return fmt.Errorf("%w, and no day list of that name in holidays.json", err)
			}
			out = append(out, p)
		}
		return nil
	}
	if err := walk(entries, nil); err != nil {
		return nil, err
	}
	return out, nil
}
