package setup

import "testing"

func TestLookupChecksum(t *testing.T) {
	manifest := `
abc123  trinity-linux-x86_64.zip
def456 *trinity-linux-arm64.zip
xyz789  some-other-file.zip
`
	cases := map[string]string{
		"trinity-linux-x86_64.zip": "abc123", // text mode (two spaces)
		"trinity-linux-arm64.zip":  "def456", // binary mode (asterisk)
		"some-other-file.zip":      "xyz789",
		"missing.zip":              "",
	}
	for asset, want := range cases {
		got := lookupChecksum(manifest, asset)
		if got != want {
			t.Errorf("lookupChecksum(%q): got %q, want %q", asset, got, want)
		}
	}
}
