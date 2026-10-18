// Package scripts embeds the shell scripts in this directory so the
// trinity binary can run them without a checkout. The .sh files are
// the source; install.sh is also fetched raw from GitHub for the
// curl|bash bootstrap.
package scripts

import _ "embed"

//go:embed bootstrap-nginx.sh
var BootstrapNginx []byte
