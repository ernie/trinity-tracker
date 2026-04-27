# Setting up a Trinity server + collector

End state: your matches show up on a Trinity hub's leaderboard
(`trinity.run` or your own), players get welcome / `!claim` messages
in chat, and your recorded demos are watchable through the hub's web
player. For architecture / lifecycle / security background, see
[distributed-deployment.md](./distributed-deployment.md).

The stack:

| Component | What it does |
|-----------|--------------|
| **trinity-engine** | Forked ioquake3 server binary (`trinity.ded`) + the trinity mod QVMs (bundled in the same release zip). Vanilla ioquake3 won't work — the tracker keys on log lines only the trinity mod and engine emit. |
| **trinity-tracker** | Daemon (`trinity`) that tails the q3 server log and publishes facts to the hub. This repo. |
| **nginx + certbot** | Optional for stat publishing, but effectively required if you want demos recorded on your host to play back through the hub UI (the hub 302s users to your `public_url`, and a hub HTTPS page can't load content from a non-HTTPS host). |

---

## Prerequisites

- **Linux host**. Debian/Ubuntu primary; other distros should work with
  minor `apt` → `dnf`/`pacman` substitutions.
- **Go ≥1.22** for the trinity-tracker build. `mise` / `asdf` / a manual
  install all work — the installer detects `go` on your `PATH` before
  elevating to root.
- Standard userland: `curl`, `unzip`, `make`, basic toolchain. The
  installer apt-installs anything missing.
- **A retail copy of `pak0.pk3`** for baseq3 (and a separate one for
  missionpack if you want Team Arena maps). The trinity-engine release
  zip bundles the engine + the trinity mod + the publicly-distributable
  game patches, but the retail Quake 3 asset pak isn't redistributable.
  You supply it from your own legitimately-acquired install. After running
  the installer you'll drop these into:
  - `/usr/lib/quake3/baseq3/pak0.pk3`
  - `/usr/lib/quake3/missionpack/pak0.pk3` *(optional)*

  > The free Quake 3 demo (evaluation version) is **not supported** —
  > retail only.
- **Outbound TCP/4222** to your hub (default `trinity.run`). This is
  where the collector publishes events and receives RPC replies. The
  hub serves NATS over TLS on this port; the collector validates the
  hub's cert against system CA roots, so a strict-egress firewall just
  needs to allow plain TCP/4222 (no TLS-inspection proxy).
- **Inbound UDP** on whatever port(s) you bind your q3 server(s) to
  (typical: 27960, 27961, …).

---

## 1. Coordinate with the hub admin to provision your source

Before you install anything, the hub admin needs to add a row for you in
the hub's `sources` table and mint a `.creds` file you'll install on your
host. The admin UI does this in one click. For the canonical hub at
`trinity.run`, ping **NilClass** in the
[Team Beef Discord](https://discord.gg/tuDB2YNc7h) with a proposed source
name (short, alphanumeric/hyphen/underscore, e.g. `mygamesite`).

You'll receive a single file like `mygamesite.creds`. Treat it as a
secret — it's an NKey + JWT bundle that authenticates your collector to
the hub's NATS broker.

---

## 2. Run the installer

Clone the trinity-tracker repo and run `scripts/install-collector.sh`.
It handles the operator-independent parts (deps, the `quake` user, the
[trinity-engine release zip](https://github.com/ernie/trinity-engine/releases/latest),
the trinity binary, the `.creds`, and the systemd units) and asks for
sudo consent before doing so. Everything operator-owned (pak0, q3 cfgs,
the trinity config, nginx) is covered in the sections below. Run the
script with no env vars set to see its full usage.

```bash
git clone https://github.com/ernie/trinity-tracker
cd trinity-tracker

sudo SOURCE_NAME=mygamesite \
     PUBLIC_URL=https://q3.example.com \
     CREDS_FILE=/path/to/mygamesite.creds \
     ./scripts/install-collector.sh
```

The script does **not** start any service — your config isn't ready yet.
Set `ENGINE_VERSION=v0.9.16` to pin a specific engine release, or
`HUB_HOST=...` to report to a hub other than `trinity.run`.

---

## 3. Drop your retail `pak0.pk3` into place

The engine refuses to load most maps without the retail pak0.pk3 (which
contains the base models, sounds, and the original maps). Copy it from
wherever you keep your licensed Q3 install:

```bash
sudo install -m 0644 -o quake -g quake \
    /path/to/your/Quake3/baseq3/pak0.pk3 \
    /usr/lib/quake3/baseq3/pak0.pk3

# Repeat for missionpack/ if you want Team Arena gametypes:
sudo install -m 0644 -o quake -g quake \
    /path/to/your/Quake3/missionpack/pak0.pk3 \
    /usr/lib/quake3/missionpack/pak0.pk3
```

---

## 4. RCON: required for player messaging

**Read this even if you skim everything else.**

The collector publishes events one-way to the hub. Replies that need to
reach players in-game — welcome lines on join, the random code returned by
`!claim`, the help text from `!help` — are delivered via RCON from the
collector to your q3 server.

Two places have to agree on the RCON password:

1. The q3 server cfg (`set rconpassword "..."` — see the cvars in step 5
   below).
2. The trinity collector's config (`q3_servers[].rcon_password` — see
   step 6).

If you skip it, stats still flow to the hub but **players see no welcome
or claim messages**, and `journalctl -fu trinity` shows lines like:

```
Error sending print to client N on server K: RCON not configured for this server
```

Pick a long random string and put it in both places.

---

## 5. Configure your q3 cvars

Trinity-required cvars live in **`/usr/lib/quake3/baseq3/autoexec.cfg`**
(loaded automatically for every server using the baseq3 mod). If you
also run missionpack-based servers, mirror them in
`/usr/lib/quake3/missionpack/autoexec.cfg`. Per-server settings (gametype,
fraglimit, mapname, mod-specific tunables) live in
`/usr/lib/quake3/baseq3/<key>.cfg`, loaded by the systemd template unit's
`+exec %i.cfg`.

Recommended `autoexec.cfg`:

```
set g_logSync          1                       // REQUIRED: flush per write so events arrive promptly
set g_trinityHandshake 1                       // REQUIRED: without this, the hub ignores everything from this server (no stats, no sessions, no live events)
set rconpassword       "<a long random string>"   // REQUIRED — see §4 above

// Strongly recommended — without these the hub has no demos to play back
// for matches recorded on your servers.
set sv_tvAuto          1                       // auto-record demos
set sv_tvpath          "demos"                 // default

// Strongly recommended — discard recordings that never had a real human
// player on the server. Defaults are 0/0 (keep every recording), which
// fills your disk with bot-only matches.
set sv_tvAutoMinPlayers     1
set sv_tvAutoMinPlayersSecs 60

// Strongly recommended — bound sudden-death overtime to a few minutes.
// Default is 0 (unbounded sudden death), which can produce huge demo
// files if a tied match drags on.
set g_overtimelimit         2

// Strongly recommended — let clients download .tvd recordings and
// missing pk3s straight from your nginx fast-dl vhost (set up by
// scripts/bootstrap-nginx.sh on port 27970). Without these, players who
// don't already have a map have no way to fetch it from your server.
set sv_tvDownload           1
set sv_dlURL                "http://<your-public-hostname>:27970"
```

The systemd template unit also passes `+set g_log "logs/%i.log"` on the
command line (where `%i` is the server key from
`quake3-server@<key>.service`), so each server logs to its own file in
`/var/log/quake3/`.

**Why `g_logSync 1` is required**: without it the q3 server buffers log
writes, and the collector tails stale content. Player joins, kills,
match-end events all land at the hub seconds-to-minutes late depending on
how busy the server is. Always set it.

### Per-server cfg

Create one `<key>.cfg` per q3 server you want to run. Example
`/usr/lib/quake3/baseq3/ffa.cfg`:

```
set sv_hostname "MyGameSite — FFA"
set g_gametype  0                  // 0 = FFA
set fraglimit   30
set timelimit   15
map q3dm17
```

### Per-server env file

The systemd template unit reads `/etc/trinity/<key>.env` to get the bind
port. Create one per server:

```bash
sudo tee /etc/trinity/ffa.env > /dev/null <<'EOF'
SERVER_OPTS="+set net_port 27960"
EOF

sudo tee /etc/trinity/1v1.env > /dev/null <<'EOF'
SERVER_OPTS="+set net_port 27961"
EOF
```

For missionpack-based servers add `+set fs_game missionpack`:

```
SERVER_OPTS="+set net_port 27962 +set fs_game missionpack"
```

---

## 6. Configure `/etc/trinity/config.yml`

Copy the example and edit:

```bash
sudo install -m 0640 -o root -g quake \
    /etc/trinity/config.yml.example /etc/trinity/config.yml
sudo nano /etc/trinity/config.yml
```

Fields you must change:

- **`tracker.collector.source_id`** — must match the source name the hub
  admin provisioned for you (and the subject of your `.creds` file).
- **`tracker.collector.public_url`** — the publicly-reachable HTTPS URL
  for this host. The bare hostname is what the hub stores as the q3
  server's address; the full URL is the demo download base.
- **`q3_servers[*]`** — one entry per server you set up in step 5:
  - `key` — short, stable identifier. **Identity is `(source, key)`**:
    if you rename the key you create a brand new server in the hub's
    eyes (and split your match history). The address is mutable — port
    changes and host moves keep the same row.
  - `address` — `<your-public-hostname>:<port>` matching what's in the
    `.env` file. Use the public hostname here, not 127.0.0.1.
  - `log_path` — `/var/log/quake3/<key>.log` to match what the systemd
    unit's `+set g_log "logs/<key>.log"` writes.
  - `rcon_password` — same value as `rconpassword` in your `autoexec.cfg`.

---

## 7. Start everything and verify

Enable and start the q3 servers first, then trinity:

```bash
sudo systemctl enable --now quake3-server@ffa.service
sudo systemctl enable --now quake3-server@1v1.service
sudo systemctl enable --now trinity.service
```

The order matters only on this first run — the trinity unit's
`Before=quake3-servers.target` makes systemd start trinity first on
subsequent reboots, and the collector's tailer handles a not-yet-
existing log file by waiting for it.

Watch the trinity log:

```bash
sudo journalctl -fu trinity
```

You should see lines like:

```
Trinity vX.Y.Z starting...
Monitoring N servers
Collector publishing as source=mygamesite (last_seq=0)
Replaying log for ffa from <timestamp>
Collector heartbeating every 30s
```

Then on the hub side, your source should show up green in the admin
sources view within ~30s of a heartbeat.

**End-to-end smoke test**:

1. Connect to your q3 server in-game.
2. You should see a welcome message in chat (e.g.
   `Welcome back, ernie^7!` if your GUID is already linked, or a generic
   greeting if it isn't). If you don't see anything → re-check §4 (RCON).
3. Play a match through to its natural end (frag/time limit). Reload
   the hub's matches view — your match should appear within a few
   seconds of the map ending.

---

## 8. Serve recorded demos cross-host via nginx

This step is technically optional — stat publishing works without it —
but if you took the §5 recommendation to enable `sv_tvAuto`, it's
effectively required for those recordings to be watchable. The hub
serves demo files at `/demos/<uuid>.tvd`; for matches recorded on your
host it 302s to your `public_url`, and the hub's HTTPS page won't load
content from a non-HTTPS collector host.

```bash
sudo PUBLIC_URL=https://q3.example.com \
     ADMIN_EMAIL=you@example.com \
     ./scripts/bootstrap-nginx.sh
```

The script installs nginx + certbot, runs a webroot-challenge cert
issuance, then writes an HTTPS vhost serving `/demos/`,
`/assets/levelshots/`, and `/demopk3s/` (all CORS-enabled for the hub's
WASM player), plus a `:27970` fast-download vhost over `/usr/lib/quake3/`
with `pak0.pk3` blocked.

Once nginx is up, populate the static asset directories from your pk3s:

```bash
sudo -u quake trinity levelshots /usr/lib/quake3
sudo -u quake trinity demobake   /usr/lib/quake3
```

`levelshots` extracts a JPG per map; `demobake` builds the baseline +
per-map pk3s so the web demo player can stream the right map data.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Error sending print to client N on server K: RCON not configured for this server` in `journalctl -fu trinity` | `rcon_password` empty in `/etc/trinity/config.yml`, or doesn't match `rconpassword` in `autoexec.cfg` | Set both, restart trinity. |
| `natsbus.RPCClient: ... auth violation` or "permissions violation" at startup | Wrong `.creds` file, or your source was deactivated on the hub | Confirm the hub still lists your source as active; ask the admin for fresh creds if rotated. |
| Source stuck "stale" in the hub admin UI | `trinity.service` not running, or NATS port (TCP/4222) blocked outbound | `sudo systemctl status trinity`; `nc -vz <hub_host> 4222`. |
| Repeated `unknown event: TrinityHandshake:` in q3 log | Trinity mod not loaded — release zip wasn't extracted into `baseq3/`, or you set `fs_game` to a non-trinity mod | Confirm `/usr/lib/quake3/baseq3/trinity-baseq3.pk3` exists; restart the q3 server. |
| No `DemoSaved:` lines ever appear in the q3 log | Engine isn't the trinity build (vanilla ioquake3, etc.), or `sv_tvAuto 0` | Confirm `/usr/lib/quake3/trinity.ded` is the symlink the installer set up; verify cvars. |
| Match cards on the hub show no play button | Demo wasn't finalized — the recording was discarded (player count below threshold, server crash, manual abort). Expected. | Nothing to fix. The hub uses `matches.demo_available` to decide whether to render the play button. |
| Q3 server logs are growing to gigabytes | `/etc/logrotate.d/quake3` got removed or never installed | Reinstall it from `scripts/logrotate.quake3` (the installer drops it by default). |
| Demo playback in the hub UI hangs / 404s on a custom map | nginx not reachable from the hub, or `trinity demobake` never ran for that map's pk3 | See §8; make sure the map's pk3 is still in `/usr/lib/quake3/baseq3/`, then re-run `trinity demobake`. |
