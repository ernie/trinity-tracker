# Trinity

Real-time statistics tracking system for Quake 3 Arena servers.

## Installation

### From Prebuilt Release

Download the latest release for your platform from the [Releases](https://github.com/ernie/trinity-tools/releases) page:

| Platform | Architecture | File |
| -------- | ------------ | ---- |
| Linux (64-bit) | x86_64 | `trinity-vX.X.X-linux-amd64.tar.gz` |
| Linux (64-bit ARM) | aarch64 (Raspberry Pi 4/5 64-bit) | `trinity-vX.X.X-linux-arm64.tar.gz` |
| Linux (32-bit ARM) | armv7 (Raspberry Pi 2/3/4 32-bit) | `trinity-vX.X.X-linux-arm.tar.gz` |

```bash
# Download and extract (replace with your architecture and version)
wget https://github.com/ernie/trinity-tools/releases/download/v1.0.0/trinity-v1.0.0-linux-arm64.tar.gz
tar -xzf trinity-v1.0.0-linux-arm64.tar.gz
cd trinity-v1.0.0-linux-arm64

# Create service user
sudo useradd -r -s /usr/sbin/nologin quake

# Create directories
sudo mkdir -p /var/lib/trinity /etc/trinity
sudo chown -R quake:quake /var/lib/trinity /etc/trinity

# Install binary
sudo install -m 755 trinity /usr/local/bin/

# Install web assets
sudo cp -r web /var/lib/trinity/
sudo chown -R quake:quake /var/lib/trinity/web

# Install config
sudo -u quake cp config.example.yml /etc/trinity/config.yml
sudo chmod 640 /etc/trinity/config.yml
# Edit /etc/trinity/config.yml with your settings

# Install and enable service
sudo cp trinity.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable trinity
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
  -config <path>    Path to config file (default: /etc/trinity/config.yml)
```

### CLI Commands

```bash
trinity status                   Show all servers status
trinity players                  Show current players across all servers
trinity players --humans         Show only human players
trinity matches                  Show recent matches
trinity leaderboard              Show top players
trinity user add <username>      Add a user (prompts for password)
       [--admin]                 Create as admin user
       [--player-id=N]           Link to player ID
trinity user remove <username>   Remove a user
trinity user list                List all users
trinity user reset <username>    Reset a user's password
trinity user admin <username>    Toggle admin status for a user
trinity levelshots <path>        Extract levelshots from pk3 file(s)
trinity version                  Show version
trinity help                     Show help
```

### Extracting Levelshots

The `levelshots` command extracts map preview images from Q3A pk3 files and places them in the web frontend's assets directory. These images are displayed as subtle backgrounds on server and match cards, and as hover previews on map names.

```bash
# Extract from a directory (recursively scans for pk3 files)
# -config option not needed if you use the default location
sudo -u quake trinity -config /etc/trinity/config.yml levelshots /usr/lib/quake3

# Extract from a single pk3 file
sudo -u quake trinity -config /etc/trinity/config.yml levelshots /path/to/map-mymap.pk3
```

The command:

- Recursively searches directories for `.pk3` files
- Extracts `levelshots/*.jpg` and `levelshots/*.tga` from each pk3
- Converts TGA files to JPG format
- Saves images to `<static_dir>/assets/levelshots/<mapname>.jpg`

Requires `static_dir` to be configured in your config file.

## Configuration

Create `config.yml`:

```yaml
server:
  listen_addr: "127.0.0.1" # Use "0.0.0.0" to listen on all interfaces
  http_port: 8080
  poll_interval: 5s
  static_dir: "/var/lib/trinity/web"

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
| `database.path`              | SQLite database file path                                          |
| `q3_servers[].name`          | Display name for the server                                        |
| `q3_servers[].address`       | UDP address for server queries                                     |
| `q3_servers[].log_path`      | Path to Q3 server log (optional, enables detailed event tracking)  |
| `q3_servers[].rcon_password` | RCON password (optional, enables remote console in web UI)         |

## Running

```bash
./bin/trinity serve -config config.yml
```

Open <http://localhost:8080> in your browser.

## Installation from Source (Ubuntu 24.04)

For prebuilt binaries, see [Installation](#installation) above.

```bash
# Create service user (if not using existing quake user)
sudo useradd -r -s /usr/sbin/nologin quake

# Create directories
sudo mkdir -p /var/lib/trinity/web
sudo chown -R quake:quake /var/lib/trinity
sudo mkdir -p /etc/trinity
sudo chown -R quake:quake /etc/trinity

# Build and install binary
make
sudo make install

# Copy web frontend
sudo cp -r web/dist/* /var/lib/trinity/web/
sudo chown -R quake:quake /var/lib/trinity/web

# Create config
sudo -u quake cp config.example.yml /etc/trinity/config.yml
sudo chmod 640 /etc/trinity/config.yml
# Edit /etc/trinity/config.yml with your settings

# Install and enable service
sudo cp trinity.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable trinity
sudo systemctl start trinity

# Check status
sudo systemctl status trinity
sudo journalctl -u trinity -f

# Create an admin user (required for RCON access in web UI)
sudo -u quake trinity user add admin --admin
```

## Systemd Service

The `trinity.service` file is included in the repository. It runs as the `quake` user and automatically uses `/etc/trinity/config.yml`. Copy it to `/etc/systemd/system/trinity.service` and modify as needed.

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

When using nginx for static files, Trinity won't serve static content directly, but keep `static_dir` configured so the `levelshots` command knows where to save extracted map images:

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
- `kill` - Frag event

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
set g_logSync 0
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

Example systemd service for the Q3 server:

```ini
[Unit]
Description=Quake 3 FFA
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=quake
WorkingDirectory=/usr/lib/quake3
ExecStart=/usr/bin/screen -DmS quake3-ffa -L -Logfile /var/log/quake3/ffa-console.log /usr/lib/quake3/quake3e.ded.aarch64 +set com_hunkmegs 256 +set net_port 27960 +exec ffa.cfg
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

This uses screen for interactive console access (`screen -r quake3-ffa`), with console output logged separately from the game event log.

## License

MIT
