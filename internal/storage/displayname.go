package storage

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/ernie/trinity-tracker/internal/domain"
)

var displayNameMultiSpace = regexp.MustCompile(`\s+`)

// reservedNames contains Q3 + Team Arena bot names (missionpack pak0
// scripts/bots.txt), the TA clan aliases the mod synthesizes as bots,
// plus engine defaults that must stay unclaimed.
var reservedNames = func() map[string]bool {
	names := []string{
		"Anarki", "Angel", "Biker", "Bitterman", "Bones", "Cadavre",
		"Callisto", "Crash", "Daemia", "Doom", "Flayer", "Fritzkrieg",
		"Gammy", "Gaunt", "Gorre", "Grunt", "Hossman", "Hunter",
		"James", "Janet", "Keel", "Khan", "Klesk", "Lucy", "Major",
		"Megan", "Morgan", "Mynx", "Neptune", "Orbb", "Patriot",
		"Phobos", "Pi", "Ranger", "Razor", "Sarge", "Slash", "Sorlag",
		"Stripe", "TankJr", "Uriel", "Ursula", "Visor", "Wrack", "Xaero",
		"Abominator", "Aria", "Bradamante", "Darkangel", "Deadeye",
		"Enforcer", "Furioso", "Gargouille", "Gladiator", "Icarus",
		"Lionheart", "November", "Paracyte", "Punisher", "Sinister",
		"Stalker", "WarLokk", "Whyrlwynd",
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

// nameEntitled reports whether playerID may have rawName recorded in its
// alias ("also known as") history. It is false for reserved names (bot names
// + the engine default UnnamedPlayer), which nobody owns, and for a display
// name locked by a *different* user — both are names a player only lands on
// transiently before the server forces a rename. Runs on the caller's querier
// (typically the in-flight tx) so the ownership read sees the same data the
// surrounding write does.
func nameEntitled(ctx context.Context, q sqlQuerier, rawName string, playerID int64) (bool, error) {
	cano := CanonicalizeDisplayName(rawName)
	if cano == "" {
		return true, nil // no canonical form to match; record as before
	}
	if isReservedName(cano) {
		return false, nil
	}
	var ownedByOther int
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		WHERE display_name_canonical = ? AND (player_id IS NULL OR player_id != ?)
	`, cano, playerID).Scan(&ownedByOther); err != nil {
		return false, fmt.Errorf("checking display name ownership: %w", err)
	}
	return ownedByOther == 0, nil
}

// IsNameReserved reports whether a raw in-game name conflicts with a bot
// name or a verified user's locked display name. excludePlayerID prevents
// a player from conflicting with their own reserved name.
func (s *Store) IsNameReserved(ctx context.Context, rawName string, excludePlayerID int64) bool {
	cano := CanonicalizeDisplayName(rawName)
	if cano == "" {
		return false
	}
	if isReservedName(cano) {
		return true
	}
	return s.IsDisplayNameTaken(ctx, cano, excludePlayerID)
}

// CanonicalizeDisplayName returns the per-account-uniqueness form of a raw
// display name. Composition:
//  1. domain.StripVRTag   — removes "[VR]" prefix/suffix decoration
//  2. domain.CleanQ3Name  — strips Q3 color codes (^0-^7)
//  3. collapse whitespace runs to a single space
//  4. trim ends
//  5. lowercase — "Foo" and "FOO" resolve to the same identity
func CanonicalizeDisplayName(raw string) string {
	s := domain.StripVRTag(raw)
	s = domain.CleanQ3Name(s)
	s = displayNameMultiSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}

var (
	ErrDisplayNameReserved = errors.New("display name is reserved")
	ErrDisplayNameTaken    = errors.New("display name is taken")
)

// AdminSetUserDisplayName sets a user's locked display name to a new
// identity (self-service may only recolor). raw must be cleaned and
// canonical derived from it. Uniqueness is checked by user id so
// unlinked users don't conflict with their own name.
func (s *Store) AdminSetUserDisplayName(ctx context.Context, userID int64, raw, canonical string) error {
	if canonical == "" {
		return errors.New("display name canonical form is empty")
	}
	if isReservedName(canonical) {
		return ErrDisplayNameReserved
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var taken int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users
		WHERE display_name_canonical = ? AND id != ?
	`, canonical, userID).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return ErrDisplayNameTaken
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET display_name = ?, display_name_canonical = ?
		WHERE id = ?
	`, raw, canonical, userID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found: %d", userID)
	}
	return tx.Commit()
}
