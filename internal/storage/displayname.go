package storage

import (
	"regexp"
	"strings"

	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
)

var displayNameMultiSpace = regexp.MustCompile(`\s+`)

// canonicalizeDisplayName returns the per-account-uniqueness form of a raw
// display name. Composition:
//  1. discord.StripVRTag  — removes "[VR]" prefix/suffix decoration
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
	s := discord.StripVRTag(raw)
	s = domain.CleanQ3Name(s)
	s = displayNameMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
