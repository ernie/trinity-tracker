# Distributed Tracking Deployment Guide

Trinity Tracker can run in four deployment modes, selected entirely by
the top-level `tracker:` block in `/etc/trinity/config.yml`.

## Modes

| Mode | `tracker:` config | Process does |
|------|-------------------|--------------|
| Standalone | absent | Parses local logs, serves unified UI, no NATS. Default and unchanged. |
| Hub + local collector | `tracker.hub` and `tracker.collector` both set | One process, both roles. Embedded NATS server. Matches a single-host community deployment. |
| Hub-only | `tracker.hub` set, `tracker.collector` absent | Serves UI, subscribes to NATS, polls remote game servers. Does no log parsing. |
| Collector-only | `tracker.collector` set, `tracker.hub` absent | Tails logs, publishes events to a remote hub's NATS. (Deferred — the current binary fatal-errors at startup; unblock in a future patch.) |

## Example configs

### Standalone (unchanged from M1)

```yaml
server:
  listen_addr: "127.0.0.1"
  http_port: 8080
database:
  path: /var/lib/trinity/trinity.db
q3_servers:
  - name: ffa
    address: 127.0.0.1:27960
    log_path: /var/log/quake3/ffa.log
    rcon_password: ...
```

### Hub + local collector

```yaml
server:
  listen_addr: "0.0.0.0"
  http_port: 8080
database:
  path: /var/lib/trinity/trinity.db
q3_servers:
  - name: ffa
    address: 127.0.0.1:27960
    log_path: /var/log/quake3/ffa.log
    rcon_password: ...

tracker:
  nats:
    url: "nats://0.0.0.0:4222"     # embedded server bind
  hub:
    dedup_window: "30m"
    retention: "10d"
    approval_required: true
  collector:
    source_id: "chicago-ffa"       # this host's logical source name
    data_dir: "/var/lib/trinity"
    heartbeat_interval: "30s"
    demo_base_url: "https://trinity.chicago.example.com"
    hub_host: "trinity.run"
```

The embedded NATS server listens on 4222 for future remote collectors.
Data files the collector persists under `data_dir`:

- `source_uuid` — UUIDv4 generated on first run, stable forever
- `publish_watermark.json` — last NATS-acked `{seq, ts}` pair for replay

### hub_host and the NATS endpoint

`hub_host` is a bare hostname (no scheme, no port). It serves two
roles:

- Shown verbatim in the `!claim` chat reply so players know which
  web UI to visit.
- When `tracker.nats.url` is unset in collector-only mode, the NATS
  endpoint defaults to `nats://<hub_host>:4222`. For split
  topologies (NATS on a private interface, web UI on a public one),
  set `tracker.nats.url` explicitly.

In hub+collector and hub-only modes the NATS URL defaults to
`nats://localhost:4222` regardless of `hub_host`, since the embedded
server is the connection target.

### Hub-only (multi-host community)

```yaml
tracker:
  nats:
    url: "nats://0.0.0.0:4222"
  hub:
    dedup_window: "30m"
    retention: "10d"
    approval_required: true
```

Remote collectors elsewhere publish to this host's 4222. The hub polls
each approved remote server's `remote_address` via UDP for live status.

## Operational flow

### First boot of a remote collector

1. Operator edits the collector-host config and starts the binary.
2. The collector generates a UUID into `<data_dir>/source_uuid` and
   starts publishing registration heartbeats to
   `trinity.register.<source_id>`.
3. The hub sees the unknown source_uuid and upserts it into
   `pending_sources`. Any fact events from this source go to an
   in-memory DLQ (10 000 event cap, drop-oldest on overflow).
4. The hub operator reviews pending sources and approves via:

   ```bash
   curl -X POST \
     -H 'Authorization: Bearer $JWT' \
     -H 'Content-Type: application/json' \
     -d '{"demo_base_url":"https://remote.example"}' \
     https://hub.example.com/api/admin/sources/<source_uuid>/approve
   ```

5. The hub creates `is_remote=1` servers rows from the registration's
   roster, drains the DLQ through the writer, and subsequent events
   flow straight into the DB.

Reject is `POST /api/admin/sources/<source_uuid>/reject`. Pending list
is `GET /api/admin/sources/pending`. All three sit behind `requireAdmin`
JWT auth.

### Restart safety

- The collector's `publish_watermark.json` records the last (seq, ts)
  NATS confirmed. On restart the collector replays logs silently up
  to `last_ts` (rebuilding in-memory state) and resumes publishing
  from `last_seq + 1`. First-run (no watermark) starts publishing from
  "now" rather than bulk-publishing history.
- The hub persists `source_progress.consumed_seq` per source and
  skip-acks any envelope whose Seq is at or below it. Combined with
  the JetStream 30-minute duplicate window, restarts on either side
  do not double-process events.

### NATS disconnects on the collector

The collector's publish path sits behind a `BufferedPublisher`:

- Ring buffer cap: 10 000 events (in memory, ~5 MB).
- Overflow drops the *oldest* queued event and bumps the
  `Dropped()` counter; recent history is worth more than ancient gaps.
- The drain goroutine retries failed publishes every 250 ms until
  NATS is back, so transient disconnects are transparent to the log
  parser.

A durable disk-spill file (100 MB JSONL, rotate-oldest) is not yet
implemented; if the ring runs dry in the field, the counter makes it
observable.

## Security

- Cross-source authorship is prevented by the NATS subject scheme:
  each collector publishes under its own `<source_id>` segment and
  the hub records events keyed by `source_uuid`. Spoofing a known
  source_uuid requires access to that collector's generated uuid.
- NKey authentication is plumbed at the collector connection via
  `tracker.nats.credentials_file`. The hub-side enforcement
  (operator-scoped accounts, per-subject permissions, admin-UI
  cred generation/rotation) is the pending half of the NKey work —
  operators can configure accounts manually today.

## Not yet implemented

- Collector-only mode against a remote hub (process boot-guard still
  fatal-errors; structural refactor of the non-RPC writer calls is
  the blocker).
- Disk spill for the collector's publish buffer.
- Automated cred generation / rotation in the admin UI.
- Collector-side UDP polling refactor — the local collector still
  polls its own servers; remote servers are polled by the hub.
- React admin UI for source approval — the JSON endpoints are
  in place; a dashboard sits on top.
