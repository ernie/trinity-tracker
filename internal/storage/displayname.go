package storage

import (
	"context"
	"regexp"
	"strings"

	"github.com/ernie/trinity-tracker/internal/domain"
)

var displayNameMultiSpace = regexp.MustCompile(`\s+`)

// reservedNames contains Q3 + Team Arena bot names (from missionpack pak0
// scripts/bots.txt) plus engine defaults that must stay unclaimed.
var reservedNames = func() map[string]bool {
	names := []string{
		"Anarki", "Angel", "Biker", "Bitterman", "Bones", "Cadavre",
		"Callisto", "Crash", "Daemia", "Doom", "Flayer", "Fritzkrieg",
		"Gammy", "Gaunt", "Gorre", "Grunt", "Hossman", "Hunter",
		"James", "Janet", "Keel", "Khan", "Klesk", "Lucy", "Major",
		"Megan", "Morgan", "Mynx", "Neptune", "Orbb", "Patriot",
		"Phobos", "Pi", "Ranger", "Razor", "Sarge", "Slash", "Sorlag",
		"Stripe", "TankJr", "Uriel", "Ursula", "Visor", "Wrack", "Xaero",
		"UnnamedPlayer",
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[strings.ToLower(n)] = true
	}
	return m
}()

func isReservedName(canonical string) bool {
	return reservedNames[strings.ToLower(canonical)]
}

// IsNameReserved reports whether a raw in-game name conflicts with a bot
// name or a verified user's locked display name. excludePlayerID prevents
// a player from conflicting with their own reserved name.
func (s *Store) IsNameReserved(ctx context.Context, rawName string, excludePlayerID int64) bool {
	cano := canonicalizeDisplayName(rawName)
	if cano == "" {
		return false
	}
	if isReservedName(cano) {
		return true
	}
	return s.IsDisplayNameTaken(ctx, cano, excludePlayerID)
}

// canonicalizeDisplayName returns the per-account-uniqueness form of a raw
// display name. Composition:
//  1. domain.StripVRTag   — removes "[VR]" prefix/suffix decoration
//  2. domain.CleanQ3Name  — strips Q3 color codes (^0-^7)
//  3. collapse whitespace runs to a single space
//  4. trim ends
//
// Case is preserved — "Foo" and "FOO" canonicalize to different values per
// design (matches the spec "vr-and-color-stripped-unique with multiple
// spaces collapsed to one"). The unique partial index on
// users.display_name_canonical enforces no two accounts can lock names
// that produce the same canonical form.
func canonicalizeDisplayName(raw string) string {
	s := domain.StripVRTag(raw)
	s = domain.CleanQ3Name(s)
	s = displayNameMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
