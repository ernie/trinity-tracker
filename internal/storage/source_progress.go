package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetConsumedSeq returns the highest event Seq the hub has finished
// processing for the given source. A missing row yields 0, which is
// what the subscriber wants: every envelope's Seq (>= 1) will be
// treated as new.
func (s *Store) GetConsumedSeq(ctx context.Context, sourceUUID string) (uint64, error) {
	var seq uint64
	err := s.db.QueryRowContext(ctx,
		"SELECT consumed_seq FROM source_progress WHERE source_uuid = ?",
		sourceUUID,
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("storage: GetConsumedSeq(%s): %w", sourceUUID, err)
	}
	return seq, nil
}

// AdvanceConsumedSeq upserts source_progress for the given source,
// setting consumed_seq to seq. Monotonic: if seq is not strictly
// greater than the stored value, the row is left untouched. Matches
// the subscriber's dedup invariant — consumed_seq may never regress.
func (s *Store) AdvanceConsumedSeq(ctx context.Context, sourceUUID string, seq uint64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_progress (source_uuid, consumed_seq, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (source_uuid) DO UPDATE SET
			consumed_seq = excluded.consumed_seq,
			updated_at   = excluded.updated_at
		WHERE source_progress.consumed_seq < excluded.consumed_seq
	`, sourceUUID, seq)
	if err != nil {
		return fmt.Errorf("storage: AdvanceConsumedSeq(%s, %d): %w", sourceUUID, seq, err)
	}
	return nil
}
