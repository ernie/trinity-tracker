package natsbus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// sourceUUIDFilename is the on-disk location under a collector's
// data_dir where the source's UUIDv4 is persisted. Generated on first
// run; stable forever after. Losing this file fragments a source's
// identity on the hub.
const sourceUUIDFilename = "source_uuid"

// LoadOrCreateSourceUUID reads <dataDir>/source_uuid. If the file is
// missing, a UUIDv4 is generated and written. The returned string is
// the canonical lowercase 36-character form.
func LoadOrCreateSourceUUID(dataDir string) (string, error) {
	if dataDir == "" {
		return "", fmt.Errorf("natsbus: data_dir is required to locate source_uuid")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("natsbus: ensuring data_dir %s: %w", dataDir, err)
	}
	path := filepath.Join(dataDir, sourceUUIDFilename)

	if data, err := os.ReadFile(path); err == nil {
		s := strings.TrimSpace(string(data))
		parsed, err := uuid.Parse(s)
		if err != nil {
			return "", fmt.Errorf("natsbus: %s has invalid UUID %q: %w", path, s, err)
		}
		return parsed.String(), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("natsbus: reading %s: %w", path, err)
	}

	fresh := uuid.NewString()
	if err := os.WriteFile(path, []byte(fresh+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("natsbus: writing %s: %w", path, err)
	}
	return fresh, nil
}
