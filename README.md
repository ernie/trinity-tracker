# Trinity

Statistics tracker for the [Trinity engine](https://github.com/ernie/trinity-engine), a modernized Quake III Arena, and the [Trinity mod](https://github.com/ernie/trinity). Matches are recorded and replay in the browser through the engine compiled to WebAssembly, and matches in progress can be watched live the same way (TrinityVision).

The tracker needs the retail game. The free Quake III Arena demo isn't supported.

> **New install?** [Installation](#installation) below has the
> one-line `curl | sudo bash`. It puts the `trinity` binary on the
> box and then starts `trinity init`, an interactive wizard that
> joins this host to a hub (`trinity.run` by default).
> [docs/collector-setup.md](./docs/collector-setup.md) walks through
> it step by step. Running your own hub is an expert path, covered in
> [docs/distributed-deployment.md](./docs/distributed-deployment.md).

## Installation

### One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash
```

The script installs the OS dependencies, downloads the prebuilt
`trinity` binary and web frontend for your architecture from the
latest GitHub release, and starts `trinity init`, an interactive
wizard that joins this host to a hub as a collector. Running your own
hub is a separate, expert path; see
[docs/distributed-deployment.md](./docs/distributed-deployment.md).

To work from a checkout instead:

```bash
git clone https://github.com/ernie/trinity-tracker
cd trinity-tracker
sudo ./scripts/install.sh                 # prebuilt release (default)
sudo ./scripts/install.sh --from-source   # build from this checkout
```

The wizard won't run if `/etc/trinity/config.yml` already exists. To
set up again, delete that file and run the wizard again.

### Upgrading

The same one-liner with `--upgrade` replaces the binary on an existing
install and restarts `trinity.service`, with no wizard prompts:

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash -s -- --upgrade
```

On a hub, the upgrade also copies the new web bundle into `static_dir`.
A collector-only install just gets the new binary.

Prebuilt releases support:

| Platform           | Architecture                      |
| ------------------ | --------------------------------- |
| Linux (64-bit)     | x86_64                            |
| Linux (64-bit ARM) | aarch64 (Raspberry Pi 4/5 64-bit) |
| Linux (32-bit ARM) | armv7 (Raspberry Pi 2/3/4 32-bit) |

The wizard copies your retail `pak0.pk3` files into place, generates
the map assets the hub serves, and offers to start the services. If it
skipped any of that, finish by hand:

- Put the retail `pak0.pk3` at `/usr/lib/quake3/baseq3/pak0.pk3`, and
  the Team Arena one at `missionpack/pak0.pk3` if you picked any Team
  Arena game types.
- Generate the map-derived assets the hub serves (levelshots, map long
  names and demo-playback pk3s): `sudo trinity mapsync`.
- Start the services: `sudo systemctl start trinity quake3-servers.target`.

Run `sudo trinity mapsync` again whenever you add or change maps.

The wizard installs nginx, gets a Let's Encrypt certificate covering
both `<hostname>` and `dl.<hostname>` (the second one serves fast
downloads on `:80`/`:443`), and opens the firewall ports it needs with
UFW or firewalld. Both DNS records must already point at this box when
you run the wizard, or the certificate request fails.

### Building from source

```bash
# Install frontend dependencies (first time only; needs bun 1.2 or newer)
bun install --cwd web

# Build everything
make
```

### Deploying from source

For maintainers working from a source checkout instead of the
prebuilt-release upgrade above, the Makefile has three deploy targets:

```bash
make deploy            # build, then deploy both frontend and backend
make deploy-frontend   # build, then rsync web/dist/ to the live web root
make deploy-backend    # build, then install the binary and restart service
```

Run plain `make deploy`, not `sudo make deploy`. The build needs `go`
and `bun` on your PATH (usually through mise or similar), and sudo
drops that PATH, so the build fails with `go: not found`. The targets
`sudo` only the file moves and the service restart, so you'll be asked
for your password for those.

You can override these environment variables: `DEPLOY_USER` (default
`quake`), `DEPLOY_WEB` (default `/var/lib/trinity/web`), `SERVICE`
(default `trinity`) and `BINDIR` (default `/usr/local/bin`).

#### What `deploy-frontend` does

```
sudo rsync -a --chown=$DEPLOY_USER:$DEPLOY_USER web/dist/ $DEPLOY_WEB/
# then, for each main-*.{js,css} in the live assets dir that isn't in
# web/dist/assets/, sudo rm it (purge stale content-hashed bundles).
```

- It doesn't use `--delete`. The web root also holds assets that
  `trinity assets` extracts at runtime (`levelshots/`, `portraits/`,
  `skills/`, `maps.json`, and the extracted PNGs inside `medals/`,
  `flags/` and `icons/`), and none of them are in `web/dist/`, so
  `--delete` would wipe them. Only the hashed bundles need purging,
  and the recipe removes those in a second pass.
- `--chown` gives new files to the service user, so there's no
  `chown -R` to run afterward.
- rsync writes to a temporary file and then renames it, so a failure
  partway through can't take the site down the way `rm` followed by
  `cp` could.

If you rename or remove a bundled asset that isn't content-hashed (such
as a file under `web/public/assets/`), the old copy stays in the web
root until you delete it by hand, since it's no longer in `web/dist/`.

#### What `deploy-backend` does

```
sudo install -m 755 bin/trinity $BINDIR/
sudo systemctl restart $SERVICE
```

The restart always happens, because the service only runs the new
binary after a restart. To put the restart off, run the install line
by hand instead of `make deploy-backend`.

## Usage

Trinity is one binary, with subcommands for the server and for
command-line tasks.

### Starting the server

```bash
trinity serve [options]

Options:
  --config <path>    Path to config file (default: /etc/trinity/config.yml)
```

### CLI commands

```bash
trinity init [--no-systemd] [--dry-run]     Interactive install wizard (collector-only by default)
trinity update [--check] [--dry-run]        Update tracker binary, web bundle, engine, and mod from GitHub releases
trinity serve                               Start the stats server
trinity server list                         Show configured game servers
trinity server add [<key>] [--gametype X] [--port N] [flags]
                                            Add a game server instance (interactive on a TTY)
trinity server remove <key>                 Remove a game server instance
trinity status                              Run health checks, plus live game-server status on a hub
trinity players [--humans]                  Show current players across all servers
trinity matches [--recent N]                Show recent matches (default: 20)
trinity leaderboard [--top N]               Show top players (default: 20)
trinity discord-digest [--period P] [--dry-run]
                                            Post a leaderboard digest to a Discord webhook
trinity user add [--admin] [--player-id N] <username>
                                            Add a user (prompts for password)
trinity user remove <username>              Remove a user
trinity user list                           List all users
trinity user reset <username>               Reset a user's password
trinity user admin <username>               Toggle admin status for a user
trinity user disable <username>             Disable an account (soft-delete; keeps audit history)
trinity user enable <username>              Re-enable a disabled account
trinity login [--url <hub>]                 Log in and store a personal access token
trinity logout                              Revoke and discard the stored token
trinity console                             List servers you can control
trinity console <key> [command...]          Send rcon (interactive console without command)
trinity console --follow <key>              Stream console output read-only
trinity levelshots [path]                   Extract levelshots from pk3 file(s)
trinity portraits [path]                    Extract player portraits from pk3 file(s)
trinity heads [path]                        Extract 3D head bundles from pk3 file(s)
trinity medals [path]                       Extract medal icons from pk3 file(s)
trinity skills [path]                       Extract skill icons from pk3 file(s)
trinity objectives [path]                   Extract objective-state icons (CTF flags, Harvester skulls) from pk3 file(s)
trinity arenas [path]                       Extract map longnames into maps.json
trinity assets [path]                       Extract all assets (portraits, heads, medals, skills, objectives, levelshots)
trinity mapsync [path]                      Regenerate every map-derived artifact: levelshots, maps.json, demo pk3s
trinity demobake [path]                     Build baseline pk3, map pk3s, and manifest for web demo playback
trinity unpackmaps <pk3>... [--output dir] [--prefix p] [--game baseq3|missionpack] [--quake3-dir d] [--dry-run]
                                            Split map pack(s) into standalone per-map pk3s, with their .aas files
trinity maps [--mode <mode>] [path]         Scan pk3s and report which game modes each map supports
trinity version                             Show version
trinity maps [--mode <mode>] [path]         Scan pk3s and report which game modes each map supports
trinity version                             Show version
trinity help                                Show help
```

### Server management

Add, remove and list game server instances. `server add` asks the same
per-server questions as the wizard, so a server added later is set up
the same way as one added during setup.

```bash
# List configured servers (includes systemd status if available)
trinity server list

# Add a new server interactively (gametype menu, port suggestion, RCON
# generation). On a TTY with no key arg, drops into the wizard prompts.
sudo trinity server add

# Or non-interactively from a script:
sudo trinity server add ctf --gametype ctf --port 27962
sudo trinity server add 1v1 --gametype tournament --rcon-password secret

# Remove a server (stops/disables service, archives env file, removes
# config entry; leaves the game type's cfg and the log file alone).
sudo trinity server remove ctf
```

`server add` flags:

- `--gametype`: one of `ffa`, `tournament`, `tdm`, `ctf`, `oneflag`, `overload`, `harvester` (default: `ffa`).
  The Team Arena game types (`oneflag`, `overload`, `harvester`) install under `missionpack/`; the others under `baseq3/`.
- `--ta`: run a `tdm` or `ctf` server under `missionpack/`, with Team Arena weapons and maps.
- `--port`: server port (default: the next free port from 27960)
- `--rcon-password`: RCON password (default: a generated 24-character base64 string)
- `--log-path`: log file path (default: `/var/log/quake3/<key>.log`)

Adding a server writes:

- `/etc/trinity/<key>.env`: the port, `fs_game missionpack` for Team Arena game types, and the game type's cfg to exec
- `<quake3_dir>/<modfolder>/<stem>.cfg` and `rotation.<stem>`: the game type's starter cfg and map rotation, shared by every server of that game type and written only if they don't exist yet (`<stem>` names the game type, such as `ctf`, `1v1` or `tdm-ta`)
- An entry in `/etc/trinity/config.yml`

It also enables `quake3-server@<key>.service` when systemd is present.

### Extracting assets

Trinity can extract game assets from Quake III Arena pk3 files for the
web frontend. The extraction commands read `baseq3` and then
`missionpack`. Within each, `pak0` through `pak9` come first, in order,
then the other pk3s alphabetically, so later files override earlier
ones.

The extraction commands write into the service-owned web tree, so when
run as root (`sudo`) they switch to the configured service user
(default `quake`) before writing. A plain `sudo trinity assets` leaves
its output owned by the service account, not root. An explicit
`sudo -u quake` still works.

```bash
# Extract all assets (recommended)
sudo trinity assets

# Or extract specific asset types
sudo trinity levelshots    # Map preview images
sudo trinity portraits     # Player model icons
sudo trinity medals        # Award medal icons
sudo trinity skills        # Bot skill level icons

# Regenerate everything that depends on the installed map set
sudo trinity mapsync       # levelshots + maps.json + demo pk3s

# Override the source directory (default: quake3_dir from config)
sudo trinity assets /path/to/quake3
```

| Command      | Source Path                                        | Output Path                           | Format      |
| ------------ | -------------------------------------------------- | ------------------------------------- | ----------- |
| `levelshots` | `levelshots/*.tga\|jpg`                            | `assets/levelshots/<map>.jpg`         | JPG         |
| `portraits`  | `models/players/<model>/icon_*.tga`                | `assets/portraits/<model>/icon_*.png` | PNG 128x128 |
| `medals`     | `menu/medals/medal_*.tga`, `ui/assets/medal_*.tga` | `assets/medals/medal_*.png`           | PNG 128x128 |
| `skills`     | `menu/art/skill[1-5].tga`                          | `assets/skills/skill[1-5].png`        | PNG 128x128 |

Portraits, medals and skills are upscaled to 128x128 with Catmull-Rom
(bicubic) interpolation and saved as PNG to keep their alpha
transparency.

These commands need `static_dir` configured. The source path defaults
to `quake3_dir` from the config, and you can override it on the
command line.

## Configuration

`trinity init` writes `/etc/trinity/config.yml` for you. Edit it by
hand to add servers, change ports and so on. An example for a hub and
local collector on one machine:

```yaml
server:
  listen_addr: "127.0.0.1" # Use "0.0.0.0" to listen on all interfaces
  http_port: 8080
  static_dir: "/var/lib/trinity/web"
  quake3_dir: "/usr/lib/quake3"
  service_user: "quake"
  use_systemd: true

database:
  path: "/var/lib/trinity/trinity.db"

q3_servers:
  - key: "ffa"
    address: "127.0.0.1:27960"
    log_path: "/var/log/quake3/ffa.log"
    rcon_password: "..."
  - key: "1v1"
    address: "127.0.0.1:27961"
    log_path: "/var/log/quake3/1v1.log"
    rcon_password: "..."
```

Hub-only and collector-only installs add a `tracker:` block that
selects the roles. See
[docs/distributed-deployment.md](./docs/distributed-deployment.md)
for full examples.

| Field                        | Description                                                         |
| ---------------------------- | ------------------------------------------------------------------- |
| `server.listen_addr`         | Address to listen on (default: `127.0.0.1`; `0.0.0.0` for all)      |
| `server.http_port`           | HTTP server port                                                    |
| `server.poll_interval`       | UDP polling interval, such as `5s` or `10s` (default: 5 seconds)    |
| `server.static_dir`          | Path to the built web frontend (hub modes only)                     |
| `server.quake3_dir`          | Path to the Quake III install (default: `/usr/lib/quake3`)          |
| `server.service_user`        | Service user to drop privileges to (default: `quake`)               |
| `server.use_systemd`         | Enable systemd integration (detected by `trinity init`)             |
| `database.path`              | SQLite database file path (hub modes only)                          |
| `q3_servers[].key`           | Stable identifier: lowercase letters, digits, `_` and `-`, up to 64 |
| `q3_servers[].address`       | UDP address for server queries (`host:port`)                        |
| `q3_servers[].log_path`      | Path to the game server's log, which the collector reads            |
| `q3_servers[].rcon_password` | RCON password (must match `rconpassword` in the server's cfg)       |

## Running

```bash
./bin/trinity serve --config config.yml
```

Open <http://localhost:8080> in your browser.

## Installing from source (Ubuntu 24.04)

For prebuilt binaries, see [Installation](#installation) above. To
build from a checkout instead:

```bash
# Build + run install.sh in --from-source mode. This handles deps,
# stages the freshly-built web frontend in /tmp, installs the binary,
# and exec's the wizard. The wizard copies the staged web assets into
# /var/lib/trinity/web/ as part of `init` and then removes the temp.
make
sudo ./scripts/install.sh --from-source

# Add yourself to the quake group for CLI access (log out/in to take effect)
sudo usermod -aG quake $USER

# Start trinity (and the game servers, if you configured any)
sudo systemctl start trinity quake3-servers.target

# Check status
sudo systemctl status trinity
sudo journalctl -u trinity -f

# Create an admin user (required for RCON access in web UI)
sudo -u quake trinity user add admin --admin
```

## Systemd services

The systemd unit files are built into the binary (source:
`cmd/trinity/setup/systemd/`), and `trinity init` installs only the
ones the chosen mode needs:

- `trinity.service`, always
- `quake3-server@.service` and `quake3-servers.target` when the host
  runs at least one game server (collector and combined modes)
- `trinity-digest.service` and `trinity-digest.timer` when you set up
  the Discord digest

With another init system, pass `--no-systemd` to `trinity init` to skip
the units. It still writes `/etc/trinity/config.yml` and the
per-server files.

## Nginx configuration

In production, nginx serves the static files and proxies API and
WebSocket requests to the Go backend.

The vhosts the wizard installs (`cmd/trinity/setup/nginxtemplates/`)
are the reference. Beyond the basics below, they also serve `/q3/` pak
and demo files to the in-browser engine and proxy the `/tv/`
TrinityVision live-stream routes. If you write your own nginx config
from this example, browser demo playback and live viewing won't work
until you add those locations.

Example `/etc/nginx/sites-available/trinity`:

```nginx
server {
    listen 80;
    server_name stats.example.com;

    root /var/lib/trinity/web;
    index index.html;

    # Static files
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API proxy
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # WebSocket proxy
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```

When nginx serves the static files, Trinity doesn't serve them itself,
but keep `static_dir` set so the asset extraction commands know where
to write images:

```yaml
server:
  http_port: 8080
  poll_interval: 5s
  static_dir: "/var/lib/trinity/web"
```

Enable the site:

```bash
sudo ln -s /etc/nginx/sites-available/trinity /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## API

### `GET /api/servers`

List all configured servers.

### `GET /api/servers/{id}/status`

Current server status, including players, map and scores.

**Response:**

```json
{
  "server_id": 1,
  "key": "tdm",
  "map": "q3dm17",
  "game_type": "tdm",
  "players": [...],
  "team_scores": {"red": 21, "blue": 15},
  "online": true
}
```

### `GET /api/players`

List all known players.

### `GET /api/matches`

List recent matches.

**Query parameters:**

- `limit`: number of matches to return (default 20, at most 100)

### `GET /api/stats/leaderboard`

The player leaderboard for one stat.

**Query parameters:**

- `category`: the stat to rank by (default `frags`)
- `period`: `all`, `day`, `week`, `month` or `year` (default `all`)
- `game_type`: limit the ranking to one game type
- `limit`: number of players to return (default 50, at most 100)

### `GET /ws`

WebSocket endpoint for real-time updates.

**Events include:**

- `server_update`: server status changed
- `player_join`: player connected
- `player_leave`: player disconnected
- `match_start`: new match started
- `match_end`: match ended
- `frag`: frag event

**Example message:**

```json
{
  "event": "player_join",
  "server_id": 1,
  "timestamp": "2026-01-12T19:45:00Z",
  "data": {
    "player": {
      "name": "^1Player",
      "clean_name": "Player",
      "team": 1
    }
  }
}
```

### `GET /health`

Health check endpoint. Returns `ok` with status 200.

## RCON

The web UI has an RCON terminal for servers the signed-in user may
manage. Two rules decide who that is:

- The source's owner, the user who registered the source or was
  assigned it, can RCON any server on that source, admin or not.
- An admin (`is_admin`) can RCON a remote server only when its
  operator has set `allow_hub_admin_rcon: true` for that server in the
  collector's config. The heartbeat mirrors that setting to the hub as
  `admin_delegation_enabled`. On a combined hub and collector, a local
  admin can RCON the local servers without it.

### CLI console

The CLI follows the same rules. `trinity login` signs in to the hub
and stores a personal access token at `~/.config/trinity/token.json`
(mode 0600); `trinity logout` revokes and deletes it. With a token in
place:

```bash
trinity console                  # list servers you can control
trinity console ffa              # interactive console (rcon-backed)
trinity console ffa status       # one-shot command
trinity console --follow ffa     # stream console output read-only (pipeable)
```

Live console output (interactive and `--follow` alike) comes from the
engine's console tap: the game server runs with `sv_conTap 1`, the
collector reads the tap, and the hub relays it over SSE. Commands go
over plain rcon and work either way.

### Encrypted rcon-autoset

When an authorized user joins a managed server with a Trinity client
that's logged in (from Account on the main menu, or with
`cl_trinityUser` and `cl_trinityToken`), the hub sends the client an
encrypted copy of the server's rcon password during the Trinity
handshake. The engine decrypts it and sets `rconPassword` for the
session (a transient `set`, not an archived `seta`), so `\rcon status`
works without pasting the password in.

The crypto:

- Per-handshake key `K = SipHash128(token, "trinity-rconset-key-v1" || nonce)`,
  where `token` is the user's long-lived `cl_trinityToken`.
- Ciphertext = plaintext XOR keystream (SipHash counter mode).
- 64-bit SipHash MAC over `(header || ciphertext)`.
- The wire payload is the single argument to a `trinity_rconset`
  server command, handled in the engine's `cl_trinity_rconset.c`.

The user's long-lived `cl_trinityToken` never leaves the hub; only the
per-handshake key reaches the collector. Each autoset writes an audit
row under the `rcon.autoset` action, so autosets show up next to manual
RCON commands in `source_audit`.

## Game server logs

The tracker reads each game server's log. The Trinity mod starts every
log line with an ISO 8601 timestamp, which the tracker needs.

An install from `trinity init` sets this up: the game server unit sets
`g_log` to `logs/<key>.log`, the engine install links `baseq3/logs` and
`missionpack/logs` to `/var/log/quake3`, and `trinity.cfg` sets
`g_logSync 1` so lines are written right away. Each server's log ends
up at `/var/log/quake3/<key>.log`, the default `log_path`.

For a server you run some other way, add this to its config:

```
set g_log "games.log"
set g_logSync 1 // Flush immediately (otherwise, stats will lag)
```

That produces timestamped lines like:

```
2026-01-12T19:45:00   0:00.0 InitGame: \capturelimit\5\g_gametype\4...
2026-01-12T19:45:01   0:01.2 ClientConnect: 0
2026-01-12T19:45:01   0:01.3 ClientUserinfoChanged: 0 n\Player\t\1\g\ABC123...
2026-01-12T19:45:02   0:02.4 ClientBegin: 0
2026-01-12T19:45:15   0:15.5 Kill: 2 0 7: Bot killed Player by MOD_ROCKET_SPLASH
```

The log file is written relative to `fs_homepath`/`fs_game`. With the
systemd units (which set `fs_multiuser 0`), that's the install
directory, e.g. `/usr/lib/quake3/baseq3/games.log`; for a stock client
install with the per-user home path it's `~/.trinity/baseq3/games.log`.
Point `log_path` in your Trinity config at that file, or symlink it
somewhere you prefer.

### Systemd setup

The systemd units are built into the binary and installed by
`trinity init`. Their source files are in `cmd/trinity/setup/systemd/`:

- `trinity.service`: the Trinity tracker service
- `quake3-servers.target`: groups all game server instances
- `quake3-server@.service`: template unit for game server instances
- `trinity-digest.service` and `trinity-digest.timer`: post the
  Discord leaderboard digest on a schedule

Each game server unit starts after `trinity.service`, so when the
system shuts down, systemd stops the game servers first and the
tracker is still running to record how their matches end.

Manage all servers at once through the target:

```bash
sudo systemctl start quake3-servers.target    # Start all enabled instances
sudo systemctl stop quake3-servers.target     # Stop all instances
sudo systemctl restart quake3-servers.target  # Restart all
sudo systemctl status 'quake3-server@*'       # Status of all instances
```

Control a server or watch its console:

```bash
trinity console ffa                       # interactive console (rcon-backed; needs trinity login)
trinity console ffa status                # one-shot command
sudo journalctl -fu quake3-server@ffa     # follow raw console output
```

Add a server instance:

```bash
# Interactive: prompts for gametype, port, RCON, etc., writes the game
# type's starter cfg if it isn't there yet and enables the systemd unit.
sudo trinity server add

# Or non-interactively:
sudo trinity server add tdm --gametype tdm --port 27961

# (Optional) Tweak the starter cfg
vi /usr/lib/quake3/baseq3/tdm.cfg

# Restart trinity to pick up the config change, then start the game server
sudo systemctl restart trinity
sudo systemctl start quake3-server@tdm
```

The new instance joins the `quake3-servers.target` group automatically
and is included in target operations. If you use the shell aliases
below, log in again to get the new `q3tdm` alias.

Shell aliases (zsh):

```bash
# Auto-create q3<name> aliases for opening server consoles
for f in /etc/trinity/*.env(N); do
  alias q3${f:t:r}="trinity console ${f:t:r}"
done

# Manage all servers
alias q3status="sudo systemctl status 'quake3-server@*'"
alias q3stop="sudo systemctl stop quake3-servers.target"
alias q3start="sudo systemctl start quake3-servers.target"
alias q3restart="sudo systemctl restart quake3-servers.target"
```

## License

MIT
