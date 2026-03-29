package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

func generateGameToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) RotateGameToken(ctx context.Context, userID int64) (string, error) {
	token, err := generateGameToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET game_token = ? WHERE id = ?`,
		token, userID)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) EnsureGameToken(ctx context.Context, userID int64) (string, error) {
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT game_token FROM users WHERE id = ?`,
		userID).Scan(&existing)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	return s.RotateGameToken(ctx, userID)
}

func (s *Store) GetGameTokenByGUID(ctx context.Context, guid string) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `
		SELECT u.game_token
		FROM users u
		JOIN players p ON p.id = u.player_id
		JOIN player_guids pg ON pg.player_id = p.id
		WHERE pg.guid = ? AND u.game_token != ''
	`, guid).Scan(&token)
	return token, err
}

func (s *Store) GetGameTokenByUsername(ctx context.Context, username string) (int64, string, error) {
	var playerID int64
	var token string
	err := s.db.QueryRowContext(ctx, `
		SELECT player_id, game_token FROM users
		WHERE username = ? AND game_token != '' AND player_id IS NOT NULL
	`, username).Scan(&playerID, &token)
	return playerID, token, err
}

// AssociateGUIDWithPlayer merges the GUID's current player into targetPlayerID,
// unless the GUID is already on the target player or belongs to another user's player.
// Returns true if a merge was performed (i.e. the GUID was previously unlinked).
func (s *Store) AssociateGUIDWithPlayer(ctx context.Context, guid string, targetPlayerID int64) (bool, error) {
	// Look up GUID's current player
	var currentPlayerID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT player_id FROM player_guids WHERE guid = ?
	`, guid).Scan(&currentPlayerID)
	if err != nil {
		return false, err
	}

	if currentPlayerID == targetPlayerID {
		return false, nil // already associated
	}

	// Check if the GUID's current player is linked to a different user account
	var ownerCount int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users WHERE player_id = ?
	`, currentPlayerID).Scan(&ownerCount)
	if err != nil {
		return false, err
	}
	if ownerCount > 0 {
		return false, nil // belongs to another user, don't steal
	}

	return true, s.MergePlayers(ctx, targetPlayerID, currentPlayerID)
}
