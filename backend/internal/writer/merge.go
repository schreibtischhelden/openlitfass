package writer

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"

	"openlitfass/backend/internal/model"
)

// Cross-source de-duplication.
//
// The same real-world event can come from several sources - the venue's
// own feed and an aggregator calendar. Events are matched by a
// source-independent key: date + start time + normalised title. The venue
// is deliberately not part of the key, because every source names it
// differently ("Peter Edel" vs "Bildungs- und Kulturzentrum Peter Edel");
// instead, two events whose addresses carry different postcodes are never
// merged.
//
// Matching is exact on the normalised key: reworded titles ("Kiezchor:
// Weihnachtskonzert" vs "Weihnachtskonzert des Kiezchors") are not
// recognised. A fuzzy stage can be added once aggregator sources provide
// real data to tune it on.

var (
	bracketHintRe = regexp.MustCompile(`\[[^\]]*\]`)
	nonWordRe     = regexp.MustCompile(`[^a-z0-9]+`)
	zipRe         = regexp.MustCompile(`\b\d{5}\b`)
	foldReplacer  = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "è", "e", "á", "a", "à", "a", "ó", "o", "í", "i")
)

// NormalizeTitle reduces a title to what identifies the event for matching:
// lower case, umlauts folded, hints in square brackets ("[AUSVERKAUFT!]",
// "[ZUSATZTERMIN]") dropped, punctuation and quotes removed. Only used for
// matching - the displayed title stays exactly as the source wrote it.
func NormalizeTitle(title string) string {
	s := strings.ToLower(title)
	s = bracketHintRe.ReplaceAllString(s, " ")
	s = foldReplacer.Replace(s)
	s = nonWordRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

// MatchKey is the source-independent identity of an event.
func MatchKey(e model.Event) string {
	return e.Date + "|" + e.StartTime + "|" + NormalizeTitle(e.Title)
}

// zipOf returns the first 5-digit postcode in the event's address or place
// name, or "".
func zipOf(e model.Event) string {
	if z := zipRe.FindString(e.Location.Address); z != "" {
		return z
	}
	return zipRe.FindString(e.Location.Name)
}

// EventID derives the public id from the match key: a short readable slug
// of the title plus a hash, e.g. "tanztee-fuer-alle-3f9a1b2c". Being
// source-independent, it stays the same when an event later also arrives
// from a second source, so shared links and saved stars keep working.
func EventID(key, title string) string {
	sum := sha256.Sum256([]byte(key))
	words := strings.Fields(NormalizeTitle(title))
	if len(words) > 5 {
		words = words[:5]
	}
	slug := strings.Join(words, "-")
	if len(slug) > 40 {
		slug = strings.TrimRight(slug[:40], "-")
	}
	if slug == "" {
		slug = "termin"
	}
	return slug + "-" + hex.EncodeToString(sum[:])[:8]
}

// MergeAcrossSources merges events from different sources that share a
// MatchKey (and don't have conflicting postcodes). The event from the
// source with the highest priority wins; empty fields are filled from the
// others; Sources lists all contributing sources, winner first. Events
// from the same source are never merged with each other here (Dedupe
// already handled same-source repeats; equal keys within one source are
// genuinely different events, e.g. two rooms).
func MergeAcrossSources(events []model.Event, priority map[string]int) []model.Event {
	byKey := map[string][]int{}
	var order []string
	for i, e := range events {
		k := MatchKey(e)
		if _, ok := byKey[k]; !ok {
			order = append(order, k)
		}
		byKey[k] = append(byKey[k], i)
	}

	out := make([]model.Event, 0, len(events))
	for _, k := range order {
		group := byKey[k]
		if len(group) == 1 {
			e := events[group[0]]
			e.Sources = []string{e.Source.Name}
			out = append(out, e)
			continue
		}
		// Rank by priority (higher first), then source name for stable output.
		sort.SliceStable(group, func(a, b int) bool {
			ea, eb := events[group[a]], events[group[b]]
			pa, pb := priority[ea.Source.Name], priority[eb.Source.Name]
			if pa != pb {
				return pa > pb
			}
			return ea.Source.Name < eb.Source.Name
		})
		// Greedy clustering: each event joins the first cluster from another
		// source with a compatible postcode, else starts its own.
		var clusters [][]model.Event
		for _, idx := range group {
			e := events[idx]
			placed := false
			for c := range clusters {
				if canMerge(clusters[c], e) {
					clusters[c] = append(clusters[c], e)
					placed = true
					break
				}
			}
			if !placed {
				clusters = append(clusters, []model.Event{e})
			}
		}
		for _, c := range clusters {
			out = append(out, mergeCluster(c))
		}
	}
	return out
}

func canMerge(cluster []model.Event, e model.Event) bool {
	for _, m := range cluster {
		if m.Source.Name == e.Source.Name {
			return false
		}
		if za, zb := zipOf(m), zipOf(e); za != "" && zb != "" && za != zb {
			return false
		}
	}
	return true
}

// mergeCluster returns cluster[0] (the winner) with gaps filled from the
// others. A cancellation reported by any source wins: better to warn than
// to send people to an event that doesn't happen.
func mergeCluster(cluster []model.Event) model.Event {
	m := cluster[0]
	for _, e := range cluster {
		m.Sources = append(m.Sources, e.Source.Name)
	}
	for _, e := range cluster[1:] {
		if m.Description == "" {
			m.Description = e.Description
		}
		if m.URL == "" {
			m.URL = e.URL
		}
		if m.Organizer == "" {
			m.Organizer = e.Organizer
		}
		if m.Image == "" {
			m.Image = e.Image
		}
		if m.EndTime == "" {
			m.EndTime = e.EndTime
		}
		if m.Location.Address == "" {
			m.Location.Address = e.Location.Address
		}
		if m.Location.Name == "" {
			m.Location.Name = e.Location.Name
		}
		if m.Place == "" {
			m.Place = e.Place
		}
		if m.Category == "" || m.Category == "Unkategorisiert" {
			if e.Category != "" {
				m.Category = e.Category
			}
		}
		if !m.Price.Free && m.Price.Amount == nil {
			m.Price = e.Price
		}
		m.TicketRequired = m.TicketRequired || e.TicketRequired
		m.RegistrationRequired = m.RegistrationRequired || e.RegistrationRequired
		m.FamilyFriendly = m.FamilyFriendly || e.FamilyFriendly
		m.Cancelled = m.Cancelled || e.Cancelled
	}
	return m
}
