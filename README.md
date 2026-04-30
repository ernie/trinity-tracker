# Trinity

Real-time statistics tracking system for the [Trinity Quake 3 engine](https://github.com/ernie/trinity-engine) and [Trinity mod](https://github.com/ernie/trinity).

The free Quake 3 demo (evaluation version) is not supported — retail only.

> **New install?** See [Installation](#installation) below for the
> one-line `curl | sudo bash`. It drops the `trinity` binary on the
> box, then hands off to `trinity init` — an interactive wizard that
> joins this Trinity server to a hub (defaults to `trinity.run`).
> Step-by-step walkthrough in
> [docs/collector-setup.md](./docs/collector-setup.md). Running your
> own hub is an expert path covered in
> [docs/distributed-deployment.md](./docs/distributed-deployment.md).

## Installation

### One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash
```

That fetches the script, installs OS deps, downloads the prebuilt
`trinity` binary + web frontend for your architecture from the latest
GitHub release, and hands off to `trinity init` — an interactive
wizard that joins this host to a hub as a collector. (Running your
own hub is a separate, expert path — see
[docs/distributed-deployment.md](./docs/distributed-deployment.md).)

If you'd rather work from a checkout:

```bash
git clone https://github.com/ernie/trinity-tracker
cd trinity-tracker
sudo ./scripts/install.sh                 # prebuilt release (default)
sudo ./scripts/install.sh --from-source   # build from this checkout
```

The wizard refuses to run if `/etc/trinity/config.yml` already
exists. To redo a setup, delete the file and re-run.

### Upgrading

Same one-liner with `--upgrade` swaps the binary on an existing
install and restarts `trinity.service` — no wizard prompts:

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash -s -- --upgrade
```

Hub installs also get the new web bundle overlaid onto `static_dir`.
Collector-only installs just swap the binary.

Prebuilt release supports:

| Platform           | Architecture                      |
| ------------------ | --------------------------------- |
| Linux (64-bit)     | x86_64                            |
| Linux (64-bit ARM) | aarch64 (Raspberry Pi 4/5 64-bit) |
| Linux (32-bit ARM) | armv7 (Raspberry Pi 2/3/4 32-bit) |

After the wizard finishes:

- Place retail `pak0.pk3` at `/usr/lib/quake3/baseq3/pak0.pk3` (and
  `missionpack/pak0.pk3` if you picked any gametypes from Team Arena).
- Generate the levelshot images and demo-playback pk3s the hub serves:
  `sudo -u quake trinity levelshots && sudo -u quake trinity demobake`.
- Start: `sudo systemctl start trinity quake3-servers.target`.

The wizard installs nginx + obtains a Let's Encrypt SAN cert
covering both `<hostname>` and `dl.<hostname>` (the latter hosts
the q3 fast-download vhost on `:80`/`:443`) and opens the firewall
ports it needs (UFW/firewalld). Both DNS records must already
point at this box before you run the wizard, or the cert fetch
will fail.

### Building from Source

```bash
# Install frontend dependencies (first time only) — requires bun >= 1.2
bun install --cwd web

# Build everything
make
```

## Usage

Trinity provides a single binary with subcommands for both the server and CLI operations.

### Starting the Server

```bash
trinity serve [options]

Options:
  --config <path>    Path to config file (default: /etc/trinity/config.yml)
```

### CLI Commands

```bash
trinity init [--no-systemd] [--dry-run]     Interactive install wizard (collector-only by default)
trinity serve                               Start the stats server
trinity server list                         Show configured game servers
trinity server add [<key>] [--gametype X] [--port N] [flags]
                                            Add a game server instance (interactive on a TTY)
trinity server remove <key>                 Remove a game server instance
trinity status                              Show all servers status
trinity players [--humans]                  Show current players across all servers
trinity matches [--recent N]                Show recent matches (default: 20)
trinity leaderboard [--top N]               Show top players (default: 20)
trinity user add [--admin] [--player-id N] <username>
                                            Add a user (prompts for password)
trinity user remove <username>              Remove a user
trinity user list                           List all users
trinity user reset <username>               Reset a user's password
trinity user admin <username>               Toggle admin status for a user
trinity levelshots [path]                   Extract levelshots from pk3 file(s)
trinity portraits [path]                    Extract player portraits from pk3 file(s)
trinity medals [path]                       Extract medal icons from pk3 file(s)
trinity skills [path]                       Extract skill icons from pk3 file(s)
trinity assets [path]                       Extract all assets (levelshots, portraits, medals, skills)
trinity version                             Show version
trinity help                                Show help
```

### Server Management

Add, remove, and list game server instances. The wizard's per-server
prompts are reused by `server add` so the UX matches whether you set
the host up fresh or add a server later.

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
# config entry; leaves <key>.cfg and the log file alone).
sudo trinity server remove ctf
```

`server add` flags:

- `--gametype` - one of `ffa`, `tournament`, `tdm`, `ctf`, `oneflag`, `overload`, `harvester` (default: `ffa`).
  Gametypes from Team Arena (`oneflag`/`overload`/`harvester`) deploy to `missionpack/`; the others to `baseq3/`.
- `--port` - server port (default: next available starting from 27960)
- `--rcon-password` - RCON password (default: generated 24-char base64)
- `--log-path` - log file path (default: `/var/log/quake3/<key>.log`)

Adding a server writes:
- `/etc/trinity/<key>.env` - bind port (+ `fs_game missionpack` for gametypes from Team Arena)
- `<quake3_dir>/<modfolder>/<key>.cfg` - starter cfg from the gametype template
- An entry in `/etc/trinity/config.yml`
- Enables `quake3-server@<key>.service` (if systemd present)

### Extracting Assets

Trinity can extract various game assets from Q3A pk3 files for use in the web frontend. All extraction commands read pk3 files in Quake 3's load order (baseq3 pak0-8, then missionpack pak0-3, then remaining pk3s alphabetically) so that later files properly override earlier ones.

```bash
# Extract all assets (recommended)
sudo -u quake trinity assets

# Or extract specific asset types
sudo -u quake trinity levelshots    # Map preview images
sudo -u quake trinity portraits     # Player model icons
sudo -u quake trinity medals        # Award medal icons
sudo -u quake trinity skills        # Bot skill level icons

# Override the source directory (default: quake3_dir from config)
sudo -u quake trinity assets /path/to/quake3
```

| Command      | Source Path                                        | Output Path                           | Format      |
| ------------ | -------------------------------------------------- | ------------------------------------- | ----------- |
| `levelshots` | `levelshots/*.tga\|jpg`                            | `assets/levelshots/<map>.jpg`         | JPG         |
| `portraits`  | `models/players/<model>/icon_*.tga`                | `assets/portraits/<model>/icon_*.png` | PNG 128x128 |
| `medals`     | `menu/medals/medal_*.tga`, `ui/assets/medal_*.tga` | `assets/medals/medal_*.png`           | PNG 128x128 |
| `skills`     | `menu/art/skill[1-5].tga`                          | `assets/skills/skill[1-5].png`        | PNG 128x128 |

Portraits, medals, and skills are upscaled to 128x128 using Catmull-Rom (bicubic) interpolation and saved as PNG to preserve alpha transparency.

Requires `static_dir` to be configured. The source path defaults to `quake3_dir` from config but can be overridden on the command line.

For higher quality source assets, consider installing:

- [High Quality Quake](https://www.moddb.com/mods/high-quality-quake) for baseq3
- [HQQ Team Arena](https://www.moddb.com/games/quake-iii-team-arena/addons/hqq-high-quality-quake-team-arena-test) for missionpack

## Configuration

`trinity init` writes `/etc/trinity/config.yml` for you. Hand-edit it
to add servers, change ports, etc. Example for a hub + local
collector single-machine install:

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

For hub-only or collector-only installs, the YAML grows a `tracker:`
block that selects the role(s). See
[docs/distributed-deployment.md](./docs/distributed-deployment.md)
for full examples.

| Field                        | Description                                                        |
| ---------------------------- | ------------------------------------------------------------------ |
| `server.listen_addr`         | Address to listen on (default: `127.0.0.1`, use `0.0.0.0` for all) |
| `server.http_port`           | HTTP server port                                                   |
| `server.poll_interval`       | UDP polling interval (e.g., `5s`, `10s`)                           |
| `server.static_dir`          | Path to built web frontend (hub modes only)                        |
| `server.quake3_dir`          | Path to Quake 3 install (default: `/usr/lib/quake3`)               |
| `server.service_user`        | Service user for privilege dropping (default: `quake`)             |
| `server.use_systemd`         | Enable systemd integration (auto-detected by `trinity init`)       |
| `database.path`              | SQLite database file path (hub modes only)                         |
| `q3_servers[].key`           | Stable identifier (alnum/underscore/hyphen, max 64 chars)          |
| `q3_servers[].address`       | UDP address for server queries (`host:port`)                       |
| `q3_servers[].log_path`      | Path to Q3 server log (the collector tails this)                   |
| `q3_servers[].rcon_password` | RCON password (must match `rconpassword` in the q3 server cfg)     |

## Running

```bash
./bin/trinity serve --config config.yml
```

Open <http://localhost:8080> in your browser.

## Installation from Source (Ubuntu 24.04)

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

# Start trinity (and the q3 servers if you configured any)
sudo systemctl start trinity quake3-servers.target

# Check status
sudo systemctl status trinity
sudo journalctl -u trinity -f

# Create an admin user (required for RCON access in web UI)
sudo -u quake trinity user add admin --admin
```

## Systemd Services

The systemd unit files are embedded in the binary (source:
`cmd/trinity/setup/systemd/`) and installed by `trinity init`. The
wizard installs only the units the chosen mode needs:
- `trinity.service` always
- `quake3-server@.service` and `quake3-servers.target` when at least
  one q3 server is configured (collector or combined modes)

Operators using a non-systemd init system can pass `--no-systemd` to
`trinity init` to skip unit installation; trinity still writes
`/etc/trinity/config.yml` and the per-server side files.

## Nginx Configuration

For production, serve static files from nginx and proxy API/WebSocket requests to the Go backend.

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

When using nginx for static files, Trinity won't serve static content directly, but keep `static_dir` configured so the asset extraction commands know where to save images:

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

Get current server status including players, map, and scores.

**Response:**

```json
{
  "server_id": 1,
  "name": "TDM",
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

**Query Parameters:**

- `limit` - Number of matches to return (default: 20)

### `GET /api/stats/leaderboard`

Get player leaderboard sorted by K/D ratio.

**Query Parameters:**

- `limit` - Number of players to return (default: 20)

### `GET /ws`

WebSocket endpoint for real-time updates.

**Events:**

- `server_update` - Server status changed
- `player_join` - Player connected
- `player_leave` - Player disconnected
- `match_start` - New match started
- `match_end` - Match ended
- `frag` - Frag event

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

## Quake 3 Server Log Configuration

To enable detailed event tracking, use the `g_log` cvar to write game events to a log file. This requires a modified game QVM that outputs ISO 8601 timestamps (see [baseq3a](https://github.com/ernie/baseq3a) or [missionpackplus](https://github.com/ernie/missionpackplus)).

Add to your server config:

```
set g_log "games.log"
set g_logSync 1 // Flush immediately (otherwise, stats will lag)
```

This produces timestamped log output like:

```
2026-01-12T19:45:00   0:00.0 InitGame: \capturelimit\5\g_gametype\4...
2026-01-12T19:45:01   0:01.2 ClientConnect: 0
2026-01-12T19:45:01   0:01.3 ClientUserinfoChanged: 0 n\Player\t\1\g\ABC123...
2026-01-12T19:45:02   0:02.4 ClientBegin: 0
2026-01-12T19:45:15   0:15.5 Kill: 2 0 7: Bot killed Player by MOD_ROCKET_SPLASH
```

The log file will be written relative to `fs_homepath`/`fs_game` (e.g., `~/.q3a/baseq3/games.log` or `~/.q3a/missionpack/games.log`). Point `log_path` in your trinity config to this file, or create a symlink to a preferred location.

### Systemd Setup

The systemd units are embedded in the binary and installed by `trinity init`. The source files are in `cmd/trinity/setup/systemd/`:

- `trinity.service` - the Trinity tracker service
- `quake3-servers.target` - groups all game server instances
- `quake3-server@.service` - template unit for game server instances

The target ensures proper shutdown ordering: when the system stops, game servers receive a `quit` command (via `ExecStop`) and shut down cleanly before Trinity, so match data is never lost.

**Manage all servers at once** via the target:

```bash
sudo systemctl start quake3-servers.target    # Start all enabled instances
sudo systemctl stop quake3-servers.target     # Clean shutdown (sends quit)
sudo systemctl restart quake3-servers.target  # Restart all
sudo systemctl status 'quake3-server@*'       # Status of all instances
```

**Attach to a server console** (detach with `Ctrl-A D`):

```bash
sudo -u quake screen -r quake3-ffa
```

**Adding a new server instance:**

```bash
# Interactive: prompts for gametype, port, RCON, etc., writes the
# starter <key>.cfg from a gametype template and enables the systemd unit.
sudo trinity server add

# Or non-interactively:
sudo trinity server add tdm --gametype tdm --port 27961

# (Optional) Tweak the starter cfg
vi /usr/lib/quake3/baseq3/tdm.cfg

# Restart trinity to pick up the config change, then start the game server
sudo systemctl restart trinity
sudo systemctl start quake3-server@tdm
```

The new instance automatically joins the `quake3-servers.target` group and will be included in target operations. If using the shell aliases below, log in again to pick up the new `q3tdm` alias.

**Handy shell aliases** (zsh):

```bash
# Auto-create q3<name> aliases for attaching to server consoles
for f in /etc/trinity/*.env(N); do
  alias q3${f:t:r}="sudo -u quake screen -r quake3-${f:t:r}"
done

# Manage all servers
alias q3status="sudo systemctl status 'quake3-server@*'"
alias q3stop="sudo systemctl stop quake3-servers.target"
alias q3start="sudo systemctl start quake3-servers.target"
alias q3restart="sudo systemctl restart quake3-servers.target"
```

## License

MIT
