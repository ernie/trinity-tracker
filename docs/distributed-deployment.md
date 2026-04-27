# Distributed Tracking Deployment Guide

Trinity Tracker runs in three deployment modes, selected by the
top-level `tracker:` block in `/etc/trinity/config.yml`.

## Modes

| Mode | `tracker:` config | Process does |
|------|-------------------|--------------|
| Hub + local collector | absent, or `tracker.hub` and `tracker.collector` both set | Single-machine default. Parses local logs, serves the UI, runs an embedded NATS server. A private install just binds `server.listen_addr` and the NATS URL to `127.0.0.1`. |
| Hub-only | `tracker.hub` set, `tracker.collector` absent | Serves the UI, subscribes to NATS, UDP-polls game servers for live status. No log parsing. Accepts remote collectors. |
| Collector-only | `tracker.collector` set, `tracker.hub` absent | Tails logs, publishes events over NATS to a remote hub. No local DB, no UI, no embedded NATS. |

Omitting the `tracker:` block entirely is equivalent to the
hub+local-collector default — there is no "standalone no-NATS" mode
anymore, because the embedded NATS server is free on a private
install and dropping it wouldn't simplify anything.

Q3 servers must run with `g_trinityHandshake 1` for their match stats
to be persisted. Trinity servers typically don't run in pure mode —
VR clients load the game module as a dll/so rather than a QVM, so
`sv_pure` isn't available to keep vanilla ioquake3 clients out;
`g_trinityHandshake` is the practical substitute. The handshake
confirms each client is a real Trinity client and carries the auth
credentials that make `!claim` / `!link` / greeted stats actually
usable. Collectors log a visible warning whenever they see `InitGame`
on a server without the cvar enabled, and skip publishing match_start
for those matches. The hub refuses `match_start` with
`handshake_required=false` as a safety net, so nothing lands in
`match_player_stats` without this. Sessions and live events still
flow — you'll see who's on the server in real time; stats just won't
persist.

## Example configs

### Hub + local collector (default)

```yaml
server:
  listen_addr: "127.0.0.1"          # bind externally only if you want a public hub
  http_port: 8080
database:
  path: /var/lib/trinity/trinity.db
q3_servers:
  - name: ffa
    address: 127.0.0.1:27960
    log_path: /var/log/quake3/ffa.log
    rcon_password: ...
```

The `tracker:` block can be omitted — absent, it defaults to
hub+local-collector with `source_id: local`, `data_dir` alongside the
DB, and NATS bound to `localhost:4222`. To override:

```yaml
tracker:
  nats:
    url: "nats://0.0.0.0:4222"      # bind externally to accept remote collectors
  hub:
    dedup_window: "30m"
    retention: "10d"
  collector:
    source_id: "remote-1"           # admin-chosen name surfaced in the UI
    data_dir: "/var/lib/trinity"
    heartbeat_interval: "30s"
    demo_base_url: "https://demos.example.com"
    hub_host: "trinity.example.com"
```

The local collector connects via in-process NATS using hub-internal
credentials minted on first boot — no explicit `credentials_file`
needed, and no admin provisioning step for the hub's own source.

### Hub-only

```yaml
server:
  listen_addr: "0.0.0.0"
  http_port: 8080
database:
  path: /var/lib/trinity/trinity.db

tracker:
  nats:
    url: "nats://0.0.0.0:4222"
  hub:
    dedup_window: "30m"
    retention: "10d"
```

Remote collectors publish to this host's port 4222. The hub
UDP-polls every approved game server's `remote_address` for live
status.

### Collector-only

```yaml
q3_servers:
  - name: ffa
    address: 127.0.0.1:27960
    log_path: /var/log/quake3/ffa.log
    rcon_password: ...

tracker:
  nats:
    url: "nats://trinity.example.com:4222"     # hub's NATS endpoint
    credentials_file: "/etc/trinity/remote.creds"
  collector:
    source_id: "remote-1"                      # must match what the hub admin chose
    data_dir: "/var/lib/trinity"
    heartbeat_interval: "30s"
    demo_base_url: "https://demos.example.com"
    hub_host: "trinity.example.com"
```

No `server.http_port`, no `database.path` — the collector has no
local UI and no SQLite file. State lives under `data_dir`:

- `source_uuid` — UUIDv4 generated on first run, stable forever.
  Must match the UUID the hub issued when the source was created.
- `publish_watermark.json` — last NATS-acked `{seq, ts}` for replay.
- `buffer.jsonl` + `buffer.head.json` — disk spill of events queued
  during NATS outages (see Outage handling below).

### hub_host and the NATS endpoint

`hub_host` is a bare hostname. It serves two roles:

- Shown verbatim in the `!claim` chat reply so players know which
  web UI to visit.
- In collector-only mode, supplies the default NATS endpoint
  (`nats://<hub_host>:4222`) when `tracker.nats.url` is unset.

In hub+collector and hub-only modes the NATS URL defaults to
`nats://localhost:4222` regardless of `hub_host`.

## Provisioning remote collectors

Collectors are pre-provisioned: the hub admin creates the source and
mints credentials first, hands them to the remote operator, and only
then can the collector connect. NATS auth is the trust boundary — an
unprovisioned collector is rejected at the broker, never reaches the
hub's ingest path, and cannot publish registrations, events, or live
status.

### Adding a new remote collector

1. **Hub admin**: open **Admin → Sources**, fill in **Source name**
   (e.g. `remote-1` — becomes the NATS subject scope) and click
   **Create source**. The hub inserts a `sources` row and mints an
   NKey + signed user JWT scoped to
   `trinity.{events,register,live,rpc.*}.<source_name>.>`. Click
   **Download creds** on the new row to pull the `.creds` file.

   *API equivalent:* `POST /api/admin/sources` with body
   `{"source": "remote-1"}` (returns `201 Created`), then
   `GET /api/admin/sources/<source>/creds`.

2. **Hub admin**: hand the `.creds` file and the source name to the
   remote operator out-of-band (email, encrypted channel, whatever).
   Also communicate the generated `source_uuid` — the remote
   operator's `data_dir/source_uuid` file must contain exactly this
   string. (If that file already exists from a prior run, delete it
   so it regenerates — otherwise the hub will reject registrations
   from a UUID it doesn't know.)

3. **Remote operator**: drop the `.creds` file at
   `tracker.nats.credentials_file`, set
   `tracker.collector.source_id` to the provisioned name, populate
   `data_dir/source_uuid` with the hub's UUID, and start the
   collector. It connects, publishes registration (which populates
   the server roster on the hub), and events flow immediately.

### Rotating credentials

- **Web UI:** Admin → Sources → **Rotate creds** next to the source.
  Confirms, downloads the replacement `.creds`.
- **API:** `POST /api/admin/sources/<source_uuid>/rotate-creds`
  (admin-scoped JWT).

Rotation mints a fresh user NKey, writes the new `.creds` (the
source UUID is unchanged), and adds the old pubkey to the TRINITY
account's revocation list + calls `server.UpdateAccountClaims`, so
any collector still connected with the old creds is dropped
immediately.

### Deleting a source

- **Web UI:** Admin → Sources → **Delete**.
- **API:** `DELETE /api/admin/sources/<source_uuid>`.

Removes the `sources` row, removes any `is_remote=1` servers rows
tagged with that source, revokes the NKey at the broker, and flips
the in-memory cache so any in-flight message from that source is
refused.

### Downloading current creds

`GET /api/admin/sources/<source_uuid>/creds` streams the current
`.creds` body. Handy for re-issuing to an operator who lost theirs.
The underlying NKey is unchanged — use rotate-creds if you need to
invalidate the old file.

## Restart safety

### Collector restart

- `publish_watermark.json` records the last `(seq, ts)` NATS
  confirmed. On restart the collector replays logs silently up to
  `last_ts` (rebuilding in-memory state, no publishes) and resumes
  publishing from `last_seq + 1`. First-run (no watermark) starts
  from "now" — no bulk historical backfill.
- `buffer.jsonl` is opened at the offset recorded in
  `buffer.head.json`; any spilled events from a prior outage drain
  before new log events.

### Hub restart

- `source_progress.consumed_seq` persists per source; the hub
  skip-acks any envelope whose `seq` ≤ stored value.
- Paired with the JetStream 30-minute duplicate window, restarts on
  either side do not double-process events.

## Outage handling (collector side)

The collector's publish path is a `BufferedPublisher`:

- **In-memory ring** — 10 000 events (~5 MB resident). Absorbs
  sub-second reconnect blips without touching disk.
- **Disk spill** — once the ring reaches 80% capacity (or the spill
  file already has pending data), new events append to
  `<data_dir>/buffer.jsonl` with the next-unread byte offset tracked
  in `buffer.head.json`. The drain goroutine prefers spill → ring →
  live so the hub sees monotonic publish order.
- **Spill rotation** — if the live-slice ever approaches the 100 MB
  cap, the queue compacts in place (rewrites from head onward) and,
  if still over cap, drops oldest entries one line at a time. Each
  drop bumps the publisher's `Dropped()` counter. Ring→spill
  transfers are not counted — they're preservation, not loss.

Multi-minute outages are survivable: 100 MB of JSONL is ~3–500k
events (payload sizes vary by event type), well beyond any realistic
match backlog.

## Live dashboards

The hub runs a single UDP poller that hits every provisioned game
server's `remote_address`. PlayerStatus rows are enriched by
resolving `(serverID, clientNum) → guid` through the hub-side
presence tracker (fed by `player_join` / `player_leave` events),
then `guid → player_id / verified / admin` via the player_guids +
users tables.

Live events (frag, flag, say, award, etc.) from remote collectors
travel on `trinity.live.<source>` — core NATS, no JetStream, no
durability: live events are ephemeral by definition and show up on
the dashboard the next time a player does the thing. The local
collector's live events flow through the in-process
`manager.Events()` channel directly to the WebSocket hub, avoiding
an unnecessary NATS round-trip.

## Security

- **Per-collector NKeys** scoped to publish only under their own
  `source_id` segment. Cross-source subjects are rejected by the
  broker, not the application, so a compromised collector can't
  spoof another source's events.
- **Operator JWT hierarchy** — operator + SYS + TRINITY accounts
  are generated under `<hub_data_dir>/auth/` on first boot. The
  operator signing key signs all account JWTs; the TRINITY signing
  key signs user JWTs. The hub's own in-process clients use a
  hub-internal user under TRINITY with full pub/sub.
- **Pre-provisioning gate** — a collector cannot register, publish
  events, or emit live status until the hub admin has created its
  source and issued creds. The hub's ingest path double-checks the
  source against the `sources` table, so even a misrouted message
  from a stale-but-still-valid NKey is dropped.
- **Handshake gate** — matches are only persisted when the source
  server has `g_trinityHandshake` enabled (collector reports, hub
  double-checks at `handleMatchStart`). This keeps vanilla ioquake3
  clients out of the stats pool.

## Schema notes

Distributed tracking added the following to `servers`:

- `source`, `source_uuid`, `local_id`
- `remote_address`
- `is_remote INTEGER NOT NULL DEFAULT 0`
- `last_heartbeat_at`
- `demo_base_url`
- `source_version`

Plus two new tables:

- `sources` — hub record of provisioned collectors. The hub refuses
  anything from a `source_uuid` that doesn't have a row here.
- `source_progress` — per-source `consumed_seq` for idempotent
  replay.

`schema.sql` carries the full target shape for fresh installs; see
project conventions for applying ALTERs against a deployed DB.
