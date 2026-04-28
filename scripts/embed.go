// Package scripts exposes shell scripts under this directory as
// embedded byte slices, so the trinity binary can run them without
// shelling out to a checkout or a remote URL.
//
// Source-of-truth lives in the .sh files alongside this Go file —
// devs edit those, `go build` re-embeds. The same scripts remain
// available at https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/
// for the curl|bash bootstrap path (install.sh fetches itself this way).
package scripts

import _ "embed"

//go:embed bootstrap-nginx.sh
var BootstrapNginx []byte
