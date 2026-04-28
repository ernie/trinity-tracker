# Setting up a Trinity collector

End state: matches you play on a Quake 3 server you run show up on a
Trinity hub's leaderboard (`trinity.run` or your own), players get
welcome / `!claim` chat messages, and recorded demos are watchable
through the hub's web player. For architecture / lifecycle / security
background, see [distributed-deployment.md](./distributed-deployment.md).

The stack:

| Component | What it does |
|-----------|--------------|
| **trinity-engine** | Forked ioquake3 server binary (`trinity.ded`) + the trinity mod QVMs (bundled in the same release zip). Vanilla ioquake3 won't work — the tracker keys on log lines only the trinity mod and engine emit. |
| **trinity-tracker** | Daemon (`trinity`) that tails the q3 server log and publishes facts to the hub. This repo. |
| **nginx + certbot** | Required. Serves recorded demos + levelshots over HTTPS so the hub UI can load them (the hub 302s users to your `public_url`, and an HTTPS page can't fetch content from a non-HTTPS host), and runs a `:27970` fast-download vhost so q3 clients can pull missing custom-map pk3s when they connect. See §6. |

---

## Prerequisites

- **Linux host**. Debian/Ubuntu primary; the installer probes for
  `apt`/`dnf`/`pacman`.
- **A retail copy of `pak0.pk3`** for baseq3 (and a separate one for
  missionpack if you want gametypes from Team Arena — One Flag CTF,
  Overload, Harvester). The trinity-engine release zip bundles the
  engine + the trinity mod + the publicly-distributable game patches,
  but the retail Quake 3 asset pak isn't redistributable. After the
  installer finishes you'll drop these into:
  - `/usr/lib/quake3/baseq3/pak0.pk3`
  - `/usr/lib/quake3/missionpack/pak0.pk3` *(only if you picked any gametypes from Team Arena)*

  > The free Quake 3 demo (evaluation version) is **not supported** —
  > retail only.
- **Outbound TCP/4222** to your hub (default `trinity.run`). NATS
  over TLS; the collector validates the hub's cert against system CA
  roots, so a strict-egress firewall just needs plain TCP/4222 (no
  TLS-inspection proxy).
- **Inbound UDP** on whatever port(s) you bind your q3 server(s) to
  (typical: 27960, 27961, …).
- **Inbound TCP/80, TCP/443, TCP/27970** for nginx — see next bullet.
- **A public hostname pointing at this box, with HTTPS.** Required
  for joining the network: §6 sets up nginx + a Let's Encrypt cert
  for the hub-side demo / levelshot URLs and a `:27970` fast-download
  vhost for in-game pk3 distribution. You don't need either of these
  running before the wizard, but you do need the DNS in place and
  the ports open so the cert issuance + fast-download work after.
- **A hub account and an approved source** — see §1. You'll end up
  with a source name (e.g. `mygamesite-jfk`) and a `.creds` file
  scoped to it.

---

## 1. Request a source on the hub

Before you install, you need a source row on the hub plus a `.creds`
file scoped to it. The hub UI handles both:

1. Sign up / log in at the hub (`https://trinity.run` for the
   canonical network; whichever URL your hub admin gave you
   otherwise).
2. Click **Add Servers** in the header. Fill in a source name —
   3-32 chars, alnum + `_` + `-`. A `name-airport-code` convention
   (e.g. `mygamesite-jfk`, `mygamesite-fra`) is a good idea if you
   plan to run hosts in multiple regions; you submit one request per
   physical machine and each gets its own credentials. Optional
   "what is this for?" goes in the second box.
3. Submit. The header button flips to **Request Pending**. An admin
   reviews and approves; you can keep the tab open or come back
   later. The button polls every 30 s, so it'll switch to **My
   Servers** within ~30 s of approval.
4. Click **My Servers**, then **Download .creds** on the new source.
   The file is scoped to that source name and is the secret that
   authenticates your collector to the hub's NATS broker — treat it
   like an SSH key.

If your hub doesn't have a public web UI (or the admin prefers
out-of-band onboarding), ask them directly. For `trinity.run` the
[Team Beef Discord](https://discord.gg/tuDB2YNc7h) works.

---

## 2. Run the installer

`scripts/install.sh` installs the OS packages Trinity needs, drops the
`trinity` binary on the host, then hands off to a collector-only
wizard that walks you through the rest.

```bash
git clone https://github.com/ernie/trinity-tracker
cd trinity-tracker
sudo ./scripts/install.sh
```

When the wizard prompts:

- **Hub hostname**: e.g. `trinity.run` (or your own hub).
- **Public URL**: the publicly-reachable HTTPS URL for this host
  (e.g. `https://q3.example.com`). The bare hostname is what the
  hub stores as the q3 server's address; the full URL is the demo
  download base. Required.
- **Source ID**: the source name from §1 (shown in the hub's **My
  Servers** drawer, or whatever the admin gave you out-of-band).
  Must match the `.creds` file's subject.
- **Path to .creds file**: where the file lives on this host. The
  installer copies it to `/etc/trinity/source.creds` (mode 0640
  root:quake).
- **Servers**: for each q3 server you want to run, pick a gametype.
  Trinity supports the four stock Quake 3 gametypes (FFA, Tournament,
  Team Deathmatch, Capture The Flag) plus the three from Team Arena
  (One Flag CTF, Overload, Harvester) — the Team Arena ones deploy
  to `missionpack/` and need a separate `pak0.pk3`. The wizard
  generates the per-server bind port, an RCON password (or accepts
  your own), and a starter `<key>.cfg` from the gametype template.

After confirming, the installer:
- creates the `quake` system user (if missing) and the standard dirs;
- writes `/etc/trinity/config.yml`;
- copies your `.creds` file to `/etc/trinity/source.creds`;
- downloads the trinity-engine release into `/usr/lib/quake3/`,
  symlinks `trinity.ded` to the arch binary, and points the engine's
  per-mod `logs` symlinks at `/var/log/quake3/`;
- installs systemd units (`trinity.service`, `quake3-server@.service`,
  `quake3-servers.target`) and `/etc/logrotate.d/quake3`;
- writes `/usr/lib/quake3/baseq3/trinity.cfg` (Trinity-required cvars
  + recommended sv_tv* settings + the matching rcon_password) and an
  `autoexec.cfg` that execs it (only if no autoexec.cfg exists yet);
- writes `<key>.env` for each server's bind port and a starter
  `<key>.cfg` from the gametype template;
- enables `trinity.service`, `quake3-servers.target`, and each
  `quake3-server@<key>.service`.

Re-running `trinity init` later refuses to overwrite an existing
`/etc/trinity/config.yml`. To redo, delete the file first.

---

## 3. Drop your retail `pak0.pk3` into place

The engine refuses to load most maps without the retail pak0.pk3
(the base models, sounds, and original maps). Copy it from your
licensed Q3 install:

```bash
sudo install -m 0644 -o quake -g quake \
    /path/to/your/Quake3/baseq3/pak0.pk3 \
    /usr/lib/quake3/baseq3/pak0.pk3

# Repeat for missionpack/ if you picked any gametypes from Team Arena:
sudo install -m 0644 -o quake -g quake \
    /path/to/your/Quake3/missionpack/pak0.pk3 \
    /usr/lib/quake3/missionpack/pak0.pk3
```

---

## 4. Tune your per-server cfgs (optional)

The wizard wrote a starter `<key>.cfg` for each server you added,
based on the gametype template. They're fine as-is for getting
started, but you'll probably want to edit them to set sv_hostname,
fraglimits, map cycles, etc.

Example:

```bash
sudo -u quake vi /usr/lib/quake3/baseq3/ffa.cfg
```

The required Trinity cvars (`g_logSync`, `g_trinityHandshake`, your
RCON password, sv_tv*, fast-download URL) live in
`/usr/lib/quake3/baseq3/trinity.cfg` (and `missionpack/trinity.cfg`
if you have any servers running gametypes from Team Arena). Both are exec'd by the auto-generated
`autoexec.cfg`. Change them only if you know what you're doing.

---

## 5. Start everything and verify

```bash
sudo systemctl start trinity quake3-servers.target
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

On the hub side, your source should show up green in the admin
sources view within ~30s of a heartbeat.

**End-to-end smoke test**:

1. Connect to your q3 server in-game.
2. You should see a welcome message in chat (`Welcome back, ernie^7!`
   if your GUID is already linked, or a generic greeting if it
   isn't). If you don't see anything, check that the RCON password in
   `trinity.cfg` matches the one in the wizard-written
   `/etc/trinity/config.yml` — the wizard sets them in sync, but if
   you've hand-edited one but not the other they'll silently diverge.
3. Play a match through to its natural end (frag/time limit). Reload
   the hub's matches view — your match should appear within a few
   seconds of the map ending.

---

## 6. Stand up nginx + the fast-download vhost

This step is **required for collectors joining the network**. Two
public-facing surfaces have to be served from your host:

- `/demos/`, `/assets/levelshots/`, `/demopk3s/` over HTTPS — the hub
  302s viewers to your `public_url` for matches recorded on your
  host, and an HTTPS hub page can't load mixed content from a
  non-HTTPS origin. No HTTPS, no demo playback.
- A plain-HTTP `:27970` fast-download vhost — Quake 3 clients pull
  missing custom-map pk3s from here when they connect to your
  servers. Without it, the engine falls back to UDP downloads
  through the q3 server itself; functional, but slow enough on any
  nontrivial pk3 to frustrate players into disconnecting.

`scripts/bootstrap-nginx.sh` wires both up:

```bash
sudo PUBLIC_URL=https://q3.example.com \
     ADMIN_EMAIL=you@example.com \
     ./scripts/bootstrap-nginx.sh
```

The script installs nginx + certbot, runs a webroot-challenge cert
issuance, then writes an HTTPS vhost serving `/demos/`,
`/assets/levelshots/`, and `/demopk3s/` (all CORS-enabled for the
hub's WASM player), plus a `:27970` fast-download vhost over
`/usr/lib/quake3/` with `pak0.pk3` blocked.

Once nginx is up, populate the static asset directories from your pk3s:

```bash
sudo -u quake trinity levelshots /usr/lib/quake3
sudo -u quake trinity demobake   /usr/lib/quake3
```

`levelshots` extracts a JPG per map; `demobake` builds the baseline +
per-map pk3s so the web demo player can stream the right map data.

---

## 7. Adding more servers later

Use `trinity server add`:

```bash
# Interactive (gametype menu, port, RCON, …)
sudo trinity server add

# Or non-interactively from a script:
sudo trinity server add 1v1 --gametype tournament --port 27961
```

This writes the env file and starter `<key>.cfg`, appends the entry
to `/etc/trinity/config.yml`, and enables the systemd unit. Restart
trinity (`sudo systemctl restart trinity`) and start the new server
(`sudo systemctl start quake3-server@<key>`).

`trinity server remove <key>` disables the systemd unit, archives
the env file as `<key>.env.removed-<timestamp>`, and removes the
config entry. The starter `<key>.cfg` and the log file are left alone
— they're operator content.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Error sending print to client N on server K: RCON not configured for this server` in `journalctl -fu trinity` | `rcon_password` empty in `/etc/trinity/config.yml`, or doesn't match `rconpassword` in `trinity.cfg` | Set both to the same value, restart trinity. |
| `natsbus.RPCClient: ... auth violation` or "permissions violation" at startup | Wrong `.creds` file, your source was deactivated on the hub, or your creds were rotated | Open the hub's **My Servers** drawer to confirm the source is still active; if the status is anything but green, that's your answer. To re-fetch creds, click **Rotate creds** (or **Download .creds** if you just lost the file but don't need to invalidate the old one). |
| Source stuck "stale" in the hub admin UI | `trinity.service` not running, or NATS port (TCP/4222) blocked outbound | `sudo systemctl status trinity`; `nc -vz <hub_host> 4222`. |
| Repeated `unknown event: TrinityHandshake:` in q3 log | Trinity mod not loaded — release zip wasn't extracted into `baseq3/`, or you set `fs_game` to a non-trinity mod | Confirm `/usr/lib/quake3/baseq3/trinity-baseq3.pk3` exists; restart the q3 server. |
| No `DemoSaved:` lines ever appear in the q3 log | Engine isn't the trinity build (vanilla ioquake3, etc.), or `sv_tvAuto 0` | Confirm `/usr/lib/quake3/trinity.ded` is the symlink the installer set up; verify cvars in `trinity.cfg`. |
| Match cards on the hub show no play button | Demo wasn't finalized — the recording was discarded (player count below threshold, server crash, manual abort). Expected. | Nothing to fix. The hub uses `matches.demo_available` to decide whether to render the play button. |
| Q3 server logs are growing to gigabytes | `/etc/logrotate.d/quake3` got removed or never installed | Re-run `sudo trinity init` after removing `/etc/trinity/config.yml`, or copy the snippet from `scripts/logrotate.quake3`. |
| Demo playback in the hub UI hangs / 404s on a custom map | nginx not reachable from the hub, or `trinity demobake` never ran for that map's pk3 | See §6; make sure the map's pk3 is still in `/usr/lib/quake3/baseq3/`, then re-run `trinity demobake`. |
