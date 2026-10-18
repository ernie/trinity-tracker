# Distributed tracking deployment guide

Trinity Tracker runs in three deployment modes, chosen by the
top-level `tracker:` block in `/etc/trinity/config.yml`.

The standard installer (`scripts/install.sh` → `trinity init`) sets up
**collector-only** installs. Trinity is a network of collectors that
report to one shared hub (`trinity.run`), and a second hub splits the
network in ways that are easy to do by accident and hard to merge back
later.

## Standing up your own hub

If you need your own hub, for a private network or for developing the
tracker itself, install the binary by hand and pass `--allow-hub` to
unlock the hub modes:

```
sudo trinity init --allow-hub
```

The wizard then offers three choices. It still defaults to collector,
so you have to pick a hub mode yourself:

1. Hub + local collector (single machine)
2. Hub only (central UI; remote collectors report in)
3. Collector only

The example configs below are what each mode writes to
`/etc/trinity/config.yml`.

## Modes

| Mode | `tracker:` config | Process does |
|------|-------------------|--------------|
| Hub + local collector | absent, or `tracker.hub` and `tracker.collector` both set | The single-machine default. Parses local logs, serves the UI and runs an embedded NATS server. A private install binds `server.listen_addr` and the NATS URL to `127.0.0.1`. |
| Hub-only | `tracker.hub` set, `tracker.collector` absent | Serves the UI, subscribes to NATS and UDP-polls game servers for live status. Parses no logs. Accepts remote collectors. |
| Collector-only | `tracker.collector` set, `tracker.hub` absent | Tails logs and publishes events over NATS to a remote hub. Has no local database, UI or embedded NATS. |

Leaving out the `tracker:` block entirely gives the hub + local
collector default. Every mode uses NATS; on a private install the
embedded server costs nothing.

Game servers must run with `g_trinityHandshake 1` for their match
stats to be saved. Trinity servers usually don't run in pure mode, and
the config templates ship `sv_pure 0`. Pure mode and
`g_trinityHandshake` do different jobs: pure mode pins the asset set,
and the handshake identifies the client. It confirms each client is a
real Trinity client and carries the login credentials that give
`!claim` and `!link` accounts their value. The hub rejects
`match_start` from a server without the cvar, so nothing lands in
`match_player_stats` for that match.

The hub stores the last `handshake_required` value it saw for each
server in `servers.handshake_required`. The schema default is 0, so a
new server stays hidden from the UI until a `match_start` with
`HandshakeRequired=true` turns it on. The collector publishes
`match_start` for every match whatever `g_trinityHandshake` is, so the
column follows live traffic both ways (0→1 and 1→0): turn the cvar off
and the next match hides the server in the hub UI with nothing else to
do. The one gap is a server that changes the cvar and never runs
another match. For that case, run
`UPDATE servers SET handshake_required = 0 WHERE id = ?` and restart
the hub.

The column drives three places, all on the hub:

- `handleGetServers` returns only servers with
  `handshake_required=1`, so the server cards list only proven
  servers.
- `Router.Broadcast` checks the column (cached in memory) before it
  sends a live event to WebSocket clients, so the activity feed shows
  no frags, chat, joins or awards from servers without the handshake.
- `ListPollableServers` filters on it, so the UDP poller skips those
  servers.

Match-scoped events (`match_end`, `match_settings_update`,
`match_crashed`, `demo_finalized`, `trinity_handshake`, `player_vr`)
are dropped on their own: their UUID lookups (`GetMatchByUUID`,
open-session resolution) find nothing when the hub rejected the
`match_start`. Sessions and presence still flow internally for these
servers. They only track presence, never feed `match_player_stats`,
and never show in the UI, because the server card itself is hidden.

## Example configs

### Hub + local collector (default)

```yaml
server:
  listen_addr: "127.0.0.1"          # bind externally only if you want a public hub
  http_port: 8080
database:
  path: /var/lib/trinity/trinity.db
q3_servers:
  - key: ffa
    address: 127.0.0.1:27960
    log_path: /var/log/quake3/ffa.log
    rcon_password: ...
```

The `tracker:` block is optional. Without it, the mode is hub + local
collector with `source_id: local`, `data_dir` next to the database, and
NATS bound to `localhost:4222`. To override those:

```yaml
tracker:
  nats:
    url: "nats://0.0.0.0:4222"      # bind externally to accept remote collectors
  hub:
    dedup_window: "30m"
    retention: "10d"
  collector:
    source_id: "remote-1"           # the source name, as shown in My servers
    data_dir: "/var/lib/trinity"
    heartbeat_interval: "30s"
    public_url: "https://q3.example.com"
    hub_host: "trinity.example.com"
```

The local collector connects over in-process NATS with hub-internal
credentials created on first boot. It needs no `credentials_file`,
and the hub's own source needs no admin setup.

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

Remote collectors publish to this host's port 4222. The hub UDP-polls
every approved game server's `remote_address` for live status.

### Collector-only

```yaml
q3_servers:
  - key: ffa
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
    public_url: "https://q3.example.com"
    hub_host: "trinity.example.com"
```

There's no `server.http_port` or `database.path`: the collector has no
local UI and no SQLite file. Its state lives under `data_dir`:

- `publish_watermark.json`: the last `{seq, ts}` NATS acknowledged, for
  replay.
- `buffer.jsonl` and `buffer.head.json`: events written to disk during
  NATS outages (see Outage handling below).

The collector's identity is the `source_id` in the YAML, the same name
the hub approved and the `.creds` file is scoped to. There's no
separate UUID file to keep in sync.

### hub_host and the NATS endpoint

`hub_host` is a bare hostname with two jobs:

- It appears as is in the `!claim` chat reply, so players know which
  website to visit.
- In collector-only mode, it gives the default NATS endpoint
  (`nats://<hub_host>:4222`) when `tracker.nats.url` is unset.

In hub + collector and hub-only modes, the NATS URL defaults to
`nats://localhost:4222` whatever `hub_host` is.

## Setting up remote collectors

A collector needs a `sources` row and a `.creds` bundle scoped to it
before it can publish anything. NATS auth is the trust boundary: the
broker rejects a collector without credentials before anything
reaches the hub's ingest path, so it can't publish registrations,
events or live status.

A source comes into being one of two ways.

### Self-service

Owners start from the hub's website and an admin approves the
request. The wizard's prerequisite screen points here, and
[collector-setup.md §1](./collector-setup.md#1-request-a-source-on-the-hub)
walks through it.

1. **Operator:** sign up or log in on the hub and click **Add servers**
   at the top right (or **My servers**, then **Add another source**).
   Enter a **Source name** of 3–16 characters (letters, numbers, `_`
   and `-`), optionally answer **What is this for?**, and submit. The
   button changes to **Request pending**.

   *API equivalent:* `POST /api/sources/request` with body
   `{"name":"<source>","purpose":"..."}` (returns `201 Created`).

2. **Hub admin:** open **Admin → Sources**. Pending requests are listed
   at the top with each requester's username and when they asked.
   Each row has three actions: **Approve**, **Rename** and **Reject**.
   Rename works only while the request is pending, because the source
   name scopes the NATS subjects and can't change once credentials
   exist. Reject needs a reason, which the requester sees in **My
   servers**.

   *API equivalents:* `POST /api/admin/sources/{source}/approve`,
   `POST /api/admin/sources/{source}/rename` with `{"name":"..."}`,
   `POST /api/admin/sources/{source}/reject` with `{"reason":"..."}`.

3. **Operator:** within 30 seconds the button changes to **My
   servers**. Click it, then **Download .creds** on the source, and run
   the installer on the collector host with that file.

   *API equivalent:* `GET /api/sources/mine/{source}/creds` streams
   the file, for the caller's own active sources only.

Use one source per physical host, so each has its own credentials and
its own rotate and leave controls. A user can have one pending request
at a time; active sources don't count against that.

### Created by an admin

An admin can also create a source without a request:

1. **Hub admin:** in **Admin → Sources**, fill in the **Add a new
   source** form and click **Create source**. Every source has an
   owner, a user on the hub; the owner defaults to you, and you can
   pick another user to create it for them. The hub adds the `sources`
   row, creates an NKey and a signed user JWT scoped to
   `trinity.{events,register,live,rpc.*}.<source>.>`, and saves the
   `.creds` file on disk. Click **Download .creds** on the new row to
   get it.

   *API equivalents:* `POST /api/admin/sources` with body
   `{"source":"remote-1","owner_user_id":2}` (returns `201 Created`),
   then `GET /api/admin/sources/{source}/creds`.

2. **Hub admin:** if the owner runs the collector, they can download
   the file from **My servers** themselves. Otherwise, give the
   operator the `.creds` file and the source name privately, over an
   encrypted channel.

The owner sees the source in **My servers** and can download and
rotate its credentials and take it off the network. An admin can move
a source to another owner with **Transfer** on its row.

### Running the wizard

Either way, the operator ends up with a source name and a `.creds`
file. Run `sudo ./scripts/install.sh` (collector-only by default) and
give the source name and `.creds` path when the wizard asks. The
installer writes `tracker.collector.source_id`, copies the credentials
to `/etc/trinity/source.creds`, installs the engine and enables the
systemd units. The collector connects and publishes its registration,
which fills in the server list on the hub, and events start flowing.

### Rotating credentials

- **Owner:** in **My servers**, click **Rotate creds** on the source.
  Each source can be rotated 5 times in 24 hours.

  *API:* `POST /api/sources/mine/{source}/rotate-creds`.
- **Admin (any source):** in **Admin → Sources**, click **Rotate
  creds** next to the source, for example to give an operator new
  credentials directly.

  *API:* `POST /api/admin/sources/{source}/rotate-creds`.

Rotating creates a new user NKey and writes the new `.creds` (the
source name stays the same). It also adds the old public key to the
TRINITY account's revocation list and pushes the updated account
claims to the running server, so a collector still using the old
credentials is dropped right away.

### Taking a source offline

Sources are never deleted: past matches and sessions keep their
`(source, key)` reference, so the row stays and the UI dims inactive
content. A source goes offline one of two ways:

- **Owner leaves:** in **My servers**, click **Leave network**. The
  status changes to `left`, the NATS credentials are revoked, and the
  hub blocks the source in memory. The owner can rejoin by requesting
  the same name again, which is approved automatically because the
  row is theirs.

  *API:* `POST /api/sources/mine/{source}/leave`.
- **Admin deactivates (any source):** in **Admin → Sources**, click
  **Deactivate**. The status changes to `revoked`, and the owner can't
  rejoin on their own. **Reactivate** makes the source active again
  and issues new credentials, which the owner or an admin downloads
  from the source.

  *API:* `POST /api/admin/sources/{source}/deactivate` and
  `POST /api/admin/sources/{source}/reactivate`.

Leaving and deactivating both revoke the NKey at the broker and update
the in-memory cache, so the hub refuses any message still in flight
from that source.

### Downloading current creds

- **Owner:** `GET /api/sources/mine/{source}/creds` returns the current
  `.creds` file. Use it if you lost the file and don't need to cancel
  the old one.
- **Admin (any source):** `GET /api/admin/sources/{source}/creds`.

Downloading leaves the NKey as it is. Rotate the credentials to cancel
the old file.

## Restart safety

### Collector restart

- `publish_watermark.json` records the last `(seq, ts)` NATS
  confirmed. On restart, the collector replays logs silently up to
  `last_ts` to rebuild its in-memory state, without publishing, and
  resumes publishing from `last_seq + 1`. A first run (no watermark)
  starts from now, with no backfill of history.
- `buffer.jsonl` opens at the offset in `buffer.head.json`, and events
  saved during an earlier outage drain before new log events.

### Hub restart

- `source_progress.consumed_seq` is stored per source, and the hub
  acknowledges and skips any envelope whose `seq` ≤ that value.
- Together with the JetStream 30-minute duplicate window, this keeps a
  restart on either side from processing an event twice.

## Outage handling (collector side)

The collector publishes through a `BufferedPublisher`:

- **In-memory ring:** 10 000 events (about 5 MB resident). It absorbs
  sub-second reconnects without touching disk.
- **Disk spill:** once the ring is 80% full (or the spill file already
  holds pending events), new events go to `<data_dir>/buffer.jsonl`,
  with the next unread byte offset in `buffer.head.json`. The drain
  goroutine sends spill first, then ring, then live events, so the hub
  sees events in publish order.
- **Spill rotation:** if the unread part of the file nears the 100 MB
  cap, the queue compacts in place (rewriting from the head onward),
  and if it's still over the cap, it drops the oldest entries one line
  at a time. Each drop increments the publisher's `Dropped()` counter.
  Moving events from the ring to the spill file doesn't count as a
  drop.

Outages of several minutes are fine: 100 MB of JSONL holds a few
hundred thousand events (sizes vary by event type), far more than any
match produces.

## Live dashboards

The hub runs one UDP poller that queries every approved game server's
`remote_address`. It fills in each PlayerStatus row by resolving
`(serverID, clientNum) → guid` through the hub's presence tracker (fed
by `player_join` and `player_leave` events), then
`guid → player_id / verified / admin` through the player_guids and
users tables.

Live events (frags, flags, chat, awards and so on) from remote
collectors travel on `trinity.live.<source>` over core NATS, without
JetStream or durability: a live event matters only at the moment it
happens. The local collector's live events go through the in-process
`manager.Events()` channel straight to the WebSocket hub, with no NATS
round trip.

## TrinityVision live streams

Live spectating has its own pipeline, separate from the live event bus
above and from `.tvd` recording. Three components must agree on the
wire format (`TVL1`); see the lockstep note at the end.

- **Engine (writer).** With `sv_tvLive 1`, `trinity.ded` listens on
  loopback TCP at the same port number as its UDP game port and writes
  the match as a TVL1 stream: a header for each map session, then
  length-prefixed segments anchored by keyframes
  (`sv_tvLiveKeyframeMsec`, default 1000). A map change keeps the
  socket open: the session ends with a `TVLe` marker, and the next one
  starts with a new header on the same connection.
- **Collector (relay).** On InitGame, the collector opens a lasting
  tap to `127.0.0.1:<game port>` (repeat opens are harmless, and it
  redials after a drop mid-match). Segments go into a per-server
  buffer that starts over for each map session, so viewers keep one
  URL per server across map rotations. The relay (`live_relay_addr`,
  usually `:8081`) holds viewers back on purpose: `live_delay` is the
  target viewer delay (default 5s), and the relay subtracts the
  keyframe interval it measures from the stream, so a new viewer always
  starts on a keyframe behind the live edge.
- **Hub (router).** The hub tracks each server's `is_live` in memory
  and routes `/tv/<source>/<key>/stream` to the owning collector's
  relay. The same path without the /stream suffix is the player page,
  served by the site. Browser viewer counts feed the LIVE pill and go
  back into the game through `trinity_watch` rcon commands (the
  in-game Vadrigar presence).
- **Browser (reader).** The web engine's `cl_tv` reads the stream
  through a byte-feed ring. It treats the `TVLe`→`TVL1` boundary as an
  in-place map change, keeping one connection across the rotation and
  showing a levelshot during the load.

> **Lockstep constraint:** a TVL1 format change touches the
> `trinity.ded` writer, the Go relay (parser), and the browser loader
> and web reader. Deploy all three together, or live taps fall out of
> sync.

### Console following

`trinity console --follow` is a separate tap with the same two checks
as RCON: the hub authorizes the viewer, and the collector checks the
role again against its own per-server config. The engine offers a
console tap when `sv_conTap 1` (on a loopback port the kernel assigns,
advertised as `sv_conport` in serverinfo). The collector keeps a
scrollback ring and publishes lines on core NATS only while a viewer's
lease is live. The lease expiring is the off switch, so a hub that
disappears can't leave a stream running forever. The hub sends lines
to viewers over SSE.

## Security

- **Per-collector NKeys** can publish only under their own
  `source_id` segment. The broker, not the application, rejects
  subjects for other sources, so a compromised collector can't fake
  another source's events.
- **Operator JWT hierarchy:** the operator, SYS and TRINITY accounts
  are generated under `<hub_data_dir>/auth/` on first boot. The
  operator signing key signs all account JWTs, and the TRINITY signing
  key signs user JWTs. The hub's own in-process clients use a
  hub-internal user under TRINITY with full publish and subscribe
  rights.
- **Approval gate:** a collector can't register, publish events or
  send live status until its source row is `active` on the hub.
  Self-service requests start as `pending`, with no credentials yet,
  so NATS auth would fail anyway; sources an admin creates start as
  `active`. Either way, the hub's ingest path checks the source
  against the `sources` table again, so it drops even a stray message
  from an NKey that is stale but still valid.
- **Source lifecycle audit:** every change (request, approve, reject,
  rename, download, rotate, leave, rejoin, deactivate, reactivate)
  writes a row to `source_audit` with the acting user's ID, so you can
  trace how a source reached its current state.
- **Handshake gate:** matches are saved only when the server has
  `g_trinityHandshake` enabled (the collector reports it, and the hub
  checks again at `handleMatchStart`). This keeps plain ioquake3
  clients out of the stats.

## Schema notes

For distributed tracking, `servers` has these columns:

- `source`, `local_id`
- `last_heartbeat_at`
- `demo_base_url`
- `source_version`
- `active INTEGER NOT NULL DEFAULT 1`
- `handshake_required INTEGER NOT NULL DEFAULT 0`: the gate behind the
  hub's rejection of session and live events from servers without the
  handshake (see "g_trinityHandshake" above).
- `admin_delegation_enabled INTEGER NOT NULL DEFAULT 0`

Whether a source is remote lives on its `sources` row
(`sources.is_remote`).

And three tables:

- `sources`: the hub's record of approved collectors. The hub refuses
  anything from a `source` name without a row here. It also holds
  `owner_user_id`, `status` (pending, active, rejected, left or
  revoked), `requested_purpose`, `rejection_reason` and
  `status_changed_at`. The `active` column mirrors the status
  (`active=1` ↔ `status='active'`) for the ingest gate.
- `source_progress`: each source's `consumed_seq`, for idempotent
  replay.
- `source_audit`: an append-only log of lifecycle events (request,
  approve, reject, rotate, leave and so on) with the acting user's ID.

`schema.sql` creates the current schema on a fresh database. There's no
migration runner, so changes to an existing database are made by hand
for now.

When a collector's watermark is wiped but the hub still holds the old
`consumed_seq`, the new publisher takes its starting seq from the hub
when it starts: local mode reads `source_progress` directly, and
remote collectors call the `SourceProgress` RPC.
