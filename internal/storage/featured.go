package storage

import (
	"context"
	"fmt"
)

// SetMatchFeatured toggles the is_featured flag on a single match.
func (s *Store) SetMatchFeatured(ctx context.Context, matchID int64, featured bool) error {
	v := 0
	if featured {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE matches SET is_featured = ? WHERE id = ?`,
		v, matchID,
	)
	if err != nil {
		return fmt.Errorf("set match featured: %w", err)
	}
	return nil
}

// GetFeaturedMatches returns the IDs of matches currently flagged is_featured = 1
// AND demo_available = 1, ordered by ended_at DESC. Limit caps the result.
func (s *Store) GetFeaturedMatches(ctx context.Context, limit int) ([]int64, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM matches
		WHERE is_featured = 1 AND demo_available = 1
		ORDER BY ended_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query featured matches: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
