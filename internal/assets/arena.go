package assets

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// ArenaMeta is one maps.json entry, keyed by lowercase map id.
type ArenaMeta struct {
	Map      string `json:"-"`
	LongName string `json:"longname,omitempty"`
}

// ExtractMapLongNames reads each map's worldspawn message across the pk3s.
// Later pk3s override earlier ones, matching how the engine resolves
// overlapping files.
func ExtractMapLongNames(pk3s []string) (map[string]ArenaMeta, error) {
	out := make(map[string]ArenaMeta)
	for _, pk3Path := range pk3s {
		if err := IteratePk3(pk3Path, func(name string, open func() (io.ReadCloser, error)) error {
			lower := strings.ToLower(name)
			if !strings.HasPrefix(lower, "maps/") || !strings.HasSuffix(lower, ".bsp") {
				return nil
			}
			rc, err := open()
			if err != nil {
				return fmt.Errorf("open %s in %s: %w", name, pk3Path, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return fmt.Errorf("read %s in %s: %w", name, pk3Path, err)
			}
			msg, err := BSPWorldspawnMessage(bytes.NewReader(data), int64(len(data)))
			if err != nil || msg == "" {
				return nil
			}
			mapID := strings.TrimSuffix(strings.TrimPrefix(lower, "maps/"), ".bsp")
			out[mapID] = ArenaMeta{Map: mapID, LongName: msg}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}
