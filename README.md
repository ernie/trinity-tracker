# Trinity

Real-time statistics tracking system for the [Trinity Quake 3 engine](https://github.com/ernie/trinity-engine) and [Trinity mod](https://github.com/ernie/trinity).

## Installation

### From Prebuilt Release

Download the latest release for your platform from the [Releases](https://github.com/ernie/trinity-tracker/releases) page:

| Platform           | Architecture                      | File                                |
| ------------------ | --------------------------------- | ----------------------------------- |
| Linux (64-bit)     | x86_64                            | `trinity-vX.X.X-linux-amd64.tar.gz` |
| Linux (64-bit ARM) | aarch64 (Raspberry Pi 4/5 64-bit) | `trinity-vX.X.X-linux-arm64.tar.gz` |
| Linux (32-bit ARM) | armv7 (Raspberry Pi 2/3/4 32-bit) | `trinity-vX.X.X-linux-arm.tar.gz`   |

```bash
# Download and extract (replace with your architecture and version)
wget https://github.com/ernie/trinity-tracker/releases/download/v1.0.0/trinity-v1.0.0-linux-arm64.tar.gz
tar -xzf trinity-v1.0.0-linux-arm64.tar.gz
cd trinity-v1.0.0-linux-arm64

# Install binary
sudo install -m 755 trinity /usr/local/bin/

# Bootstrap the system (creates user, dirs, config, systemd units)
sudo trinity init

# Add yourself to quake group for CLI access (log out/in to take effect)
sudo usermod -aG quake $USER

# Install web assets
sudo cp -r web/* /var/lib/trinity/web/
sudo chown -R quake:quake /var/lib/trinity/web

# Edit config with your settings
sudo -u quake vi /etc/trinity/config.yml

# Start trinity
sudo systemctl start trinity

# Verify
trinity version
sudo systemctl status trinity
```

### Building from Source

```bash
# Install frontend dependencies (first time only)
npm --prefix web install

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
trinity init [--no-systemd] [--user quake]  Bootstrap system (create user, dirs, config)
trinity serve                               Start the stats server
trinity server list                         Show configured game servers
trinity server add <name> [--port N] [flags]
                                            Add a game server instance
trinity server remove <name>                Remove a game server instance
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

Add, remove, and list game server instances:

```bash
# List configured servers (includes systemd status if available)
trinity server list

# Add a new server (auto-assigns next available port if --port omitted)
sudo trinity server add ctf --port 27962 --game missionpack

# Add with all options
sudo trinity server add tdm --port 27961 --game missionpack --display-name "Team DM" --rcon-password secret

# Remove a server (stops service, removes env file and config entry)
sudo trinity server remove ctf
```

`server add` flags:
- `--port` - server port (default: next available starting from 27960)
- `--game` - game directory, e.g. `missionpack` (default: `baseq3`)
- `--display-name` - display name (default: uppercase of name)
- `--rcon-password` - RCON password (optional)
- `--log-path` - log file path (default: `<quake3_dir>/<game>/logs/<name>.log`)

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

Create `config.yml`:

```yaml
server:
  listen_addr: "127.0.0.1" # Use "0.0.0.0" to listen on all interfaces
  http_port: 8080
  poll_interval: 5s
  static_dir: "/var/lib/trinity/web"
  quake3_dir: "/usr/lib/quake3" # For asset extraction commands
  service_user: "quake"         # Service user for privilege dropping
  use_systemd: true             # Set by `trinity init`, or override manually

database:
  path: "/var/lib/trinity/trinity.db"

q3_servers:
  - name: "FFA"
    address: "127.0.0.1:27960"
    log_path: "/var/log/quake3/ffa.log"
    rcon_password: "secret" # optional
  - name: "TDM"
    address: "127.0.0.1:27961"
    log_path: "/var/log/quake3/tdm.log"
  - name: "CTF"
    address: "127.0.0.1:27962"
    log_path: "/var/log/quake3/ctf.log"
```

| Field                        | Description                                                        |
| ---------------------------- | ------------------------------------------------------------------ |
| `server.listen_addr`         | Address to listen on (default: `127.0.0.1`, use `0.0.0.0` for all) |
| `server.http_port`           | HTTP server port                                                   |
| `server.poll_interval`       | UDP polling interval (e.g., `5s`, `10s`)                           |
| `server.static_dir`          | Path to built web frontend                                         |
| `server.quake3_dir`          | Path to Quake 3 install (default: `/usr/lib/quake3`)               |
| `server.service_user`        | Service user for privilege dropping (default: `quake`)             |
| `server.use_systemd`         | Enable systemd integration (auto-detected by `trinity init`)      |
| `database.path`              | SQLite database file path                                          |
| `q3_servers[].name`          | Display name for the server                                        |
| `q3_servers[].address`       | UDP address for server queries                                     |
| `q3_servers[].log_path`      | Path to Q3 server log (optional, enables detailed event tracking)  |
| `q3_servers[].rcon_password` | RCON password (optional, enables remote console in web UI)         |

## Running

```bash
./bin/trinity serve --config config.yml
```

Open <http://localhost:8080> in your browser.

## Installation from Source (Ubuntu 24.04)

For prebuilt binaries, see [Installation](#installation) above.

```bash
# Build and install binary
make
sudo make install

# Bootstrap the system (creates user, dirs, config, systemd units)
sudo trinity init

# Add yourself to quake group for CLI access (log out/in to take effect)
sudo usermod -aG quake $USER

# Copy web frontend
sudo cp -r web/dist/* /var/lib/trinity/web/
sudo chown -R quake:quake /var/lib/trinity/web

# Edit config with your settings
sudo -u quake vi /etc/trinity/config.yml

# Start trinity
sudo systemctl start trinity

# Check status
sudo systemctl status trinity
sudo journalctl -u trinity -f

# Create an admin user (required for RCON access in web UI)
sudo -u quake trinity user add admin --admin
```

## Systemd Services

The systemd unit files are embedded in the binary (source: `cmd/trinity/systemd/`) and installed automatically by `trinity init`. See [Systemd Setup](#systemd-setup) under Quake 3 Server Log Configuration for details.

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

The systemd units are embedded in the binary and installed by `trinity init`. The source files are in `cmd/trinity/systemd/`:

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
# Add the server (creates env file, updates config, enables systemd unit)
sudo trinity server add tdm --port 27961 --game missionpack

# Create the game config
vi /usr/lib/quake3/missionpack/tdm.cfg

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
