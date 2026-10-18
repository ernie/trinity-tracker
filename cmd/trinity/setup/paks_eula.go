package setup

import _ "embed"

// quake3EULA is the id Software Limited Use Software License Agreement
// as published at https://ioquake3.org/extras/patch-data/. It governs
// redistribution of the 1.32 point-release pk3 patch data the wizard
// downloads from the hub.
//
// web/public/quake3-eula.txt is a symlink to the same file, so the
// site's EULA page shows the same text the wizard pages through.
//
//go:embed quake3-eula.txt
var quake3EULA string
