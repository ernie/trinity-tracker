package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SourceProgress is the hub's per-source dedup watermark. The
// (LastConsumedTS, ConsumedSeq) tuple is monotonic: a future advance
// must strictly follow it or be ignored.
type SourceProgress struct {
	ConsumedSeq    uint64
	LastConsumedTS time.Time
}

// GetSourceProgress returns the watermark for the given source, or a
// zero value (epoch TS, seq 0) when the source has no row yet — the
// envelope handler treats that as "everything is forward progress."
func (s *Store) GetSourceProgress(ctx context.Context, source string) (SourceProgress, error) {
	var sp SourceProgress
	var tsStr string
	err := s.db.QueryRowContext(ctx,
		"SELECT consumed_seq, last_consumed_ts FROM source_progress WHERE source = ?",
		source,
	).Scan(&sp.ConsumedSeq, &tsStr)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceProgress{}, nil
	}
	if err != nil {
		return SourceProgress{}, fmt.Errorf("storage: GetSourceProgress(%s): %w", source, err)
	}
	if tsStr != "" {
		if t, perr := time.Parse(time.RFC3339, tsStr); perr == nil {
			sp.LastConsumedTS = t.UTC()
		}
	}
	return sp, nil
}

// AdvanceSourceProgress upserts the watermark monotonically on seq:
// the row is updated only when seq strictly follows the stored value.
// last_consumed_ts is recorded alongside as forward-progress telemetry
// (seen TS at the new high-water seq) but is not consulted by the
// envelope dedup check.
func (s *Store) AdvanceSourceProgress(ctx context.Context, source string, seq uint64, ts time.Time) error {
	tsStr := formatTimestamp(ts)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_progress (source, consumed_seq, last_consumed_ts, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (source) DO UPDATE SET
			consumed_seq     = excluded.consumed_seq,
			last_consumed_ts = excluded.last_consumed_ts,
			updated_at       = excluded.updated_at
		WHERE source_progress.consumed_seq < excluded.consumed_seq
	`, source, seq, tsStr)
	if err != nil {
		return fmt.Errorf("storage: AdvanceSourceProgress(%s, %d, %v): %w", source, seq, ts, err)
	}
	return nil
}
