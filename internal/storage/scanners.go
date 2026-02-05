package storage

import (
	"database/sql"
	"time"

	"github.com/ernie/trinity-tools/internal/domain"
)

// Null scanner helpers - reduce repetitive nil-checking code

func scanNullString(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func scanNullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func scanNullTime(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

func scanNullInt64ToIntPtr(ni sql.NullInt64) *int {
	if ni.Valid {
		v := int(ni.Int64)
		return &v
	}
	return nil
}

func scanNullInt64Ptr(ni sql.NullInt64) *int64 {
	if ni.Valid {
		return &ni.Int64
	}
	return nil
}

func scanNullFloat64(nf sql.NullFloat64) *float64 {
	if nf.Valid {
		return &nf.Float64
	}
	return nil
}

// scanner is an interface satisfied by both *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

// scanUser scans a user row from the database
func scanUser(s scanner) (*User, error) {
	var user User
	var lastLogin sql.NullTime
	var playerID sql.NullInt64
	err := s.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.IsAdmin,
		&playerID, &user.PasswordChangeRequired, &user.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	user.LastLogin = scanNullTime(lastLogin)
	user.PlayerID = scanNullInt64Ptr(playerID)
	return &user, nil
}

// scanMatchSummaryRow scans match summary fields from a row
func scanMatchSummaryRow(s scanner) (*domain.MatchSummary, error) {
	var m domain.MatchSummary
	var endedAt sql.NullTime
	var exitReason sql.NullString
	var gameType sql.NullString
	var redScore, blueScore sql.NullInt64

	err := s.Scan(&m.ID, &m.ServerID, &m.ServerName, &m.MapName, &gameType,
		&m.StartedAt, &endedAt, &exitReason, &redScore, &blueScore)
	if err != nil {
		return nil, err
	}

	m.EndedAt = scanNullTime(endedAt)
	m.ExitReason = scanNullStringValue(exitReason)
	m.GameType = scanNullStringValue(gameType)
	m.RedScore = scanNullInt64ToIntPtr(redScore)
	m.BlueScore = scanNullInt64ToIntPtr(blueScore)

	return &m, nil
}

// scanMatchPlayerSummary scans a player summary row and returns the match ID and player data
func scanMatchPlayerSummary(s scanner, includeMatchID bool) (int64, *domain.MatchPlayerSummary, error) {
	var matchID int64
	var ps domain.MatchPlayerSummary
	var skill sql.NullFloat64
	var score, team sql.NullInt64
	var model sql.NullString

	var err error
	if includeMatchID {
		err = s.Scan(&matchID, &ps.PlayerID, &ps.Name, &ps.CleanName, &ps.Frags, &ps.Deaths,
			&ps.Completed, &ps.IsBot, &skill, &score, &team, &model,
			&ps.Impressives, &ps.Excellents, &ps.Humiliations, &ps.Defends, &ps.Captures, &ps.Assists, &ps.IsVR)
	} else {
		err = s.Scan(&ps.PlayerID, &ps.Name, &ps.CleanName, &ps.Frags, &ps.Deaths,
			&ps.Completed, &ps.IsBot, &skill, &score, &team, &model,
			&ps.Impressives, &ps.Excellents, &ps.Humiliations, &ps.Defends, &ps.Captures, &ps.Assists, &ps.IsVR)
	}
	if err != nil {
		return 0, nil, err
	}

	ps.Skill = scanNullFloat64(skill)
	ps.Score = scanNullInt64ToIntPtr(score)
	ps.Team = scanNullInt64ToIntPtr(team)
	ps.Model = scanNullStringValue(model)

	return matchID, &ps, nil
}
