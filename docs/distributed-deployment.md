# Distributed Tracking Deployment Guide

Trinity Tracker runs in three deployment modes, selected by the
top-level `tracker:` block in `/etc/trinity/config.yml`.

The default front door (`scripts/install.sh` → `trinity init`) only
configures **collector-only** installs. That's deliberate: Trinity
is designed as a network of collectors reporting to a shared hub
(`trinity.run`), and standing up a second hub fragments the network
in ways that are easy to do by accident and hard to consolidate later.

## Standing up your own hub

If you genuinely need to run your own hub — for a private network,
or as a development environment for the tracker itself — install the
binary by hand and pass `--allow-hub` to unlock the hub-bearing modes:

```
sudo trinity init --allow-hub
```

The wizard then offers three choices (still defaulting to collector;
you have to pick a hub mode actively):

1. Hub + local collector (single machine)
2. Hub only (central UI; remote collectors report in)
3. Collector only

The example configs below are what each mode writes to
`/etc/trinity/config.yml`.

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
credentials that make `!claim`-ed / `!link`-ed accounts actually
useful. The hub rejects `match_start` for a server without the cvar
enabled, so nothing lands in `match_player_stats` for that match.

The hub records the most recently observed `handshake_required` value
per server in `servers.handshake_required`. The schema default is 0,
so a brand-new server stays hidden from the UI until a `match_start`
with `HandshakeRequired=true` flips it on. The collector publishes
`match_start` for every match regardless of `g_trinityHandshake`, so
both 0→1 and 1→0 transitions track live traffic — flipping the cvar
off and waiting for the next match end downgrades the server in the
hub UI without manual intervention. The only gap is a server that
flips the cvar and then never runs another match: in that case the
operator can run `UPDATE servers SET handshake_required = 0 WHERE id = ?`
and restart the hub, but in normal operation that's never necessary.

The column drives three places, all on the hub:

- `handleGetServers` returns only servers with
  `handshake_required=1`, so the cards UI doesn't list unproven
  servers.
- `Router.Broadcast` consults the column (cached in memory) before
  fanning out a live event to WebSocket clients, so the activity feed
  doesn't surface frags / chat / joins / awards for non-enforcing
  servers.
- `ListPollableServers` filters on it, so the UDP poller doesn't
  waste packets querying servers below the bar.

Match-scoped events (`match_end`, `match_settings_update`,
`match_crashed`, `demo_finalized`, `trinity_handshake`) are implicitly
gated by their UUID lookups (`GetMatchByUUID`, open-session
resolution) returning nil when the originating `match_start` was
rejected. Sessions and presence still flow internally for
non-enforcing servers — they're presence-only and never feed
`match_player_stats`, and the UI never surfaces them because the
server card itself is hidden.

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
    public_url: "https://q3.example.com"
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

No `server.http_port`, no `database.path` — the collector has no
local UI and no SQLite file. State lives under `data_dir`:

- `publish_watermark.json` — last NATS-acked `{seq, ts}` for replay.
- `buffer.jsonl` + `buffer.head.json` — disk spill of events queued
  during NATS outages (see Outage handling below).

Source identity is the admin-chosen `source_id` in the YAML — the same
name the hub provisioned and the `.creds` file is scoped to. There is
no separate UUID file to keep in sync.

### hub_host and the NATS endpoint

`hub_host` is a bare hostname. It serves two roles:

- Shown verbatim in the `!claim` chat reply so players know which
  web UI to visit.
- In collector-only mode, supplies the default NATS endpoint
  (`nats://<hub_host>:4222`) when `tracker.nats.url` is unset.

In hub+collector and hub-only modes the NATS URL defaults to
`nats://localhost:4222` regardless of `hub_host`.

## Provisioning remote collectors

Collectors are pre-provisioned: a `sources` row plus a `.creds`
bundle scoped to it must exist before the collector can publish
anything. NATS auth is the trust boundary — an unprovisioned
collector is rejected at the broker, never reaches the hub's ingest
path, and cannot publish registrations, events, or live status.

There are two ways a source comes into being.

### Self-service (preferred)

Owners drive their own onboarding from the hub web UI; admins approve
in one click. This is the path the wizard's prereq screen points at,
and the path documented in
[collector-setup.md §1](./collector-setup.md#1-request-a-source-on-the-hub).

1. **Operator**: sign up / log in on the hub. Click **Add Servers**
   in the header. Pick a source name (3-32 chars, alnum +
   `_`/`-`); optionally fill in a "what is this for?" note. Submit.
   Header flips to **Request Pending**.

   *API equivalent:* `POST /api/sources/request` with body
   `{"name":"<source>","purpose":"..."}` (returns `201 Created`).

2. **Hub admin**: opens **Admin → Sources**. The Pending Requests
   section pinned to the top lists every awaiting submission with
   the requesting user's username and timestamp. Three actions per
   row: **Approve**, **Rename** (admin discretion — only legal while
   pending, since the source name is the NATS subject scope and
   can't change once creds are minted), and **Reject** (requires a
   reason; the requester sees it as a banner above their next
   request).

   *API equivalents:* `POST /api/admin/sources/{source}/approve`,
   `POST /api/admin/sources/{source}/rename` with `{"name":"..."}`,
   `POST /api/admin/sources/{source}/reject` with `{"reason":"..."}`.

3. **Operator**: header button flips to **My Servers** within ~30 s.
   Click it, then **Download .creds** on the row. Run the installer
   on the collector host with that file in hand.

   *API equivalent:* `GET /api/sources/mine/{source}/creds` streams
   the file, scoped to the caller's owned active sources.

One source per physical host is the recommended convention — each
gets its own credential and independent rotate/leave controls. Only
one *pending* request per user at a time (anti-flood); already-active
sources don't block adding more.

### Admin-direct (out-of-band onboarding)

If the operator doesn't have a hub account, or the admin prefers to
mint a source without a request:

1. **Hub admin**: in **Admin → Sources**, fill the **Add a new
   source** form (Source name) and click **Create source**. The hub
   inserts a `sources` row, mints an NKey + signed user JWT scoped to
   `trinity.{events,register,live,rpc.*}.<source>.>`, and persists
   the `.creds` file on disk. Click **Download .creds** on the new
   row to pull it.

   *API equivalents:* `POST /api/admin/sources` with body
   `{"source":"remote-1"}` (returns `201 Created`), then
   `GET /api/admin/sources/{source}/creds`.

2. **Hub admin**: hand the `.creds` file + source name to the remote
   operator out-of-band (encrypted email, signal, etc.).

Sources created this way have `owner_user_id IS NULL`, so they don't
show up in any user's My Servers drawer — only admin tools can
manage them.

### Running the wizard

Either path leaves the operator with a source name + `.creds` file.
Run `sudo ./scripts/install.sh` (collector-only by default), supply
the source name and `.creds` path when the wizard prompts, and the
installer writes `tracker.collector.source_id`, copies the creds to
`/etc/trinity/source.creds`, installs the engine, and enables the
systemd units. The collector connects, publishes registration (which
populates the server roster on the hub), and events flow
immediately.

### Rotating credentials

- **Owner self-rotate (self-service sources):** in **My Servers**
  click **Rotate creds** on the source. Rate-limited to 5 rotations
  per source per 24 h.

  *API:* `POST /api/sources/mine/{source}/rotate-creds`.
- **Admin rotate (any source):** in **Admin → Sources** click
  **Rotate creds** next to the source. Use this for admin-direct
  sources (no owner) or when re-issuing to an operator out-of-band.

  *API:* `POST /api/admin/sources/{source}/rotate-creds`.

Rotation mints a fresh user NKey, writes the new `.creds` (the
source name is unchanged), and adds the old pubkey to the TRINITY
account's revocation list + calls `server.UpdateAccountClaims`, so
any collector still connected with the old creds is dropped
immediately.

### Taking a source offline

There's no row-level delete — historical matches/sessions keep their
`(source, key)` reference, so the row stays around and the UI dims
inactive content rather than dropping it. Two ways to disable a
source:

- **Owner self-leave (self-service sources):** in **My Servers**
  click **Leave network**. Status flips to `left`, NATS creds are
  revoked, the source is marked blocked in-memory. The owner can
  rejoin by submitting a new request with the same name (auto-
  approved, since the row is theirs).

  *API:* `POST /api/sources/mine/{source}/leave`.
- **Admin deactivate (any source, punitive):** in **Admin → Sources**
  click **Deactivate**. Status flips to `revoked`; only an admin
  can re-enable.

  *API:* `POST /api/admin/sources/{source}/deactivate`.

Both paths revoke the NKey at the broker and flip the in-memory
cache, so any in-flight message from that source is refused.

### Downloading current creds

- **Owner:** `GET /api/sources/mine/{source}/creds` streams the
  current `.creds` body. Handy if you lost the file but don't need
  to invalidate the old one.
- **Admin (any source):** `GET /api/admin/sources/{source}/creds`.

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
  events, or emit live status until its source row is `active` on
  the hub. Self-service requests start in `pending` (no creds
  minted, NATS auth would fail anyway); admin-direct creates land
  straight in `active`. Either way, the hub's ingest path
  double-checks the source against the `sources` table, so even a
  misrouted message from a stale-but-still-valid NKey is dropped.
- **Source lifecycle audit** — every state change (request, approve,
  reject, rename, rotate, leave, deactivate) writes a row to
  `source_audit` with the actor's user ID. Useful for
  reconstructing how a given source got into its current state.
- **Handshake gate** — matches are only persisted when the source
  server has `g_trinityHandshake` enabled (collector reports, hub
  double-checks at `handleMatchStart`). This keeps vanilla ioquake3
  clients out of the stats pool.

## Schema notes

Distributed tracking added the following to `servers`:

- `source`, `local_id`
- `remote_address`
- `is_remote INTEGER NOT NULL DEFAULT 0`
- `last_heartbeat_at`
- `demo_base_url`
- `source_version`
- `handshake_required INTEGER NOT NULL DEFAULT 0` — gate that drives
  the hub-side rejection of session/live events for non-enforcing
  servers (see "g_trinityHandshake" above).

Plus three tables:

- `sources` — hub record of provisioned collectors. The hub refuses
  anything from a `source` name that doesn't have a row here. Self-
  service onboarding adds `owner_user_id`, `status` (pending /
  active / rejected / left / revoked), `requested_purpose`,
  `rejection_reason`, `status_changed_at`. The legacy `active`
  column is kept in sync (`active=1` ↔ `status='active'`) so the
  ingest gate keeps working.
- `source_progress` — per-source `consumed_seq` for idempotent
  replay.
- `source_audit` — append-only log of lifecycle events
  (request / approve / reject / rotate / leave / etc.) with the
  actor's user ID.

`schema.sql` carries the full target shape for fresh installs. For
existing deployments, the operator runs the matching ALTERs by hand
before deploying the new binary — e.g. for the handshake gate:

```sql
ALTER TABLE servers ADD COLUMN handshake_required INTEGER NOT NULL DEFAULT 0;
```

(the default of 0 means every existing server starts in the
"unenforcing" state until its next `match_start` arrives; if you'd
rather grandfather currently-enforcing servers, run
`UPDATE servers SET handshake_required = 1 WHERE …` after the ALTER).

The self-service onboarding columns + `source_audit` table arrived
together; the corresponding ALTER block is in the matching commit
message and follows the same "operator runs it pre-deploy" pattern.

The `last_consumed_ts` column on `source_progress` (advanced as
forward-progress telemetry; envelope dedup is seq-only):

```sql
ALTER TABLE source_progress ADD COLUMN last_consumed_ts TIMESTAMP NOT NULL DEFAULT '1970-01-01T00:00:00Z';
```

The fresh-install case (collector watermark wiped, hub still holds
the prior instance's `consumed_seq`) is handled by seeding the new
publisher's initial seq from the hub on startup — local mode reads
`source_progress` directly; remote collectors call the
`SourceProgress` RPC.
