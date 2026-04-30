package setup

import _ "embed"

// quake3EULA is the id Software Limited Use Software License Agreement
// as published at https://ioquake3.org/extras/patch-data/. It governs
// redistribution of the 1.32 point-release pk3 patch data the wizard
// downloads from the hub.
//
// The text lives in quake3-eula.txt so it can be a single source of
// truth: the Go binary embeds it for the wizard's `more`-pager step,
// and a symlink at web/public/quake3-eula.txt lets Vite serve the same
// bytes to the hub web UI's EULA page.
//
//go:embed quake3-eula.txt
var quake3EULA string
