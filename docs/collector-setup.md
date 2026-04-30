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
| **nginx + certbot** | Required. Serves recorded demos + levelshots over HTTPS so the hub UI can load them (the hub 302s users to your `public_url`, and an HTTPS page can't fetch content from a non-HTTPS host), and runs a `dl.<hostname>` fast-download vhost (HTTP + HTTPS on the same `:80`/`:443`) so q3 clients can pull missing custom-map pk3s when they connect. See §6. |

---

## Prerequisites

- **Linux host**. Debian/Ubuntu primary; the installer probes for
  `apt`/`pacman`. Fedora/RHEL aren't supported — the
  `quake3-server@.service` unit's `screen` wrapper trips SELinux's
  `init_t` confinement and there's no clean upstream-policy fix. If
  you know SELinux you can run from a checkout and craft your own
  policy module, but the curl|bash path will refuse to proceed.
- **A retail copy of `pak0.pk3`** for baseq3 (and a separate one for
  missionpack if you want gametypes from Team Arena — One Flag CTF,
  Overload, Harvester). The trinity-engine release bundles the engine
  + the trinity mod, but the retail Quake 3 asset pak isn't
  redistributable, and the engine refuses to load most maps without
  it. The wizard handles placement during its pak step (§3); to skip
  the path prompt entirely, copy your retail pak0(s) into the
  directory you'll run the installer from, renamed:
  - `q3-pak0.pk3` for baseq3
  - `mp-pak0.pk3` for missionpack (only if you'll run Team Arena)

  > The free Quake 3 demo (evaluation version) is **not supported** —
  > retail only.
- **Outbound TCP/4222** to your hub (default `trinity.run`). NATS
  over TLS; the collector validates the hub's cert against system CA
  roots, so a strict-egress firewall just needs plain TCP/4222 (no
  TLS-inspection proxy).
- **Inbound UDP** on whatever port(s) you bind your q3 server(s) to
  (typical: 27960, 27961, …).
- **Inbound TCP/80, TCP/443** for nginx — see next bullet.
- **A public hostname pointing at this box, plus a `dl.<hostname>`
  A/AAAA record on the same host.** Required for joining the
  network: §6 sets up nginx + a Let's Encrypt SAN cert covering both
  names, with the SPA / demo / levelshot URLs on `<hostname>` and an
  HTTP-+-HTTPS fast-download vhost on `dl.<hostname>` for in-game
  pk3 distribution. You don't need either of these running before
  the wizard, but you do need both DNS records in place and `:80`
  open so the cert issuance + fast-download work after.
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

The one-line installer fetches `install.sh`, installs the OS packages
Trinity needs, downloads the prebuilt `trinity` binary for this host's
architecture, then hands off to a collector-only wizard that walks you
through the rest.

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash
```

(If you'd rather work from a checkout, `git clone` the repo and run
`sudo ./scripts/install.sh` from inside it — same wizard either way.)

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
  generates the per-server bind port and an RCON password (or
  accepts your own). All servers of the same gametype share one
  `<stem>.cfg` (e.g. all TDM servers exec `tdm.cfg`); per-server
  customization is done by editing the `.env` file's `+exec`.

After confirming, the installer:
- creates the `quake` system user (if missing) and the standard dirs;
- writes `/etc/trinity/config.yml`;
- copies your `.creds` file to `/etc/trinity/source.creds`;
- downloads the trinity-engine release into `/usr/lib/quake3/` and
  points the engine's per-mod `logs` symlinks at `/var/log/quake3/`;
- installs systemd units (`trinity.service`, `quake3-server@.service`,
  `quake3-servers.target`) and `/etc/logrotate.d/quake3`;
- writes `/usr/lib/quake3/baseq3/trinity.cfg` (Trinity-required cvars
  + recommended sv_tv* settings + the matching rcon_password) and an
  `autoexec.cfg` that execs it (only if no autoexec.cfg exists yet);
- writes `<key>.env` for each server (bind port + `+exec <stem>.cfg`)
  and one shared `<stem>.cfg` + `rotation.<stem>` per gametype/mod;
- enables `trinity.service`, `quake3-servers.target`, and each
  `quake3-server@<key>.service`.

Re-running `trinity init` later refuses to overwrite an existing
`/etc/trinity/config.yml`. To redo, delete the file first.

---

## 3. Pak files (retail `pak0` + 1.32 patch data)

After the wizard finishes the configuration steps, it walks you
through pak placement: the retail `pak0.pk3`(s) you supply, plus the
1.32 point-release patch data redistributed by ioquake3.org under id
Software's EULA.

**Retail `pak0.pk3`.** If you copied a retail pak0 into the directory
you ran the installer from as `q3-pak0.pk3` (and `mp-pak0.pk3` for
Team Arena), the wizard finds them by name and installs them without
prompting. Otherwise it asks for a path — anywhere on the filesystem
is fine, any filename is fine, the wizard renames to `pak0.pk3` on
copy. Hit Enter on a blank prompt to skip; you can place the file
manually at `/usr/lib/quake3/baseq3/pak0.pk3` (and
`/usr/lib/quake3/missionpack/pak0.pk3`) before starting the server.

**Patch data.** If any of `pak1.pk3`–`pak8.pk3` are missing in
`baseq3/` (or `pak1.pk3`–`pak3.pk3` in `missionpack/` for TA
installs), the wizard offers to fetch the canonical bundle from
`https://files.ioquake3.org/quake3-latest-pk3s.zip` (~26 MB). It
displays the id Software EULA via `more` and prompts for explicit
agreement before downloading. Decline and the wizard prints the URL
and moves on — manual install is just an unzip into `baseq3/` and
`missionpack/`.

**Auto-start.** Once the required paks are all present, the wizard
offers to `systemctl start trinity.service quake3-servers.target` for
you. If anything is missing, it lists the unfinished file paths and
prints the start command for when you've placed them manually.

---

## 4. Tune your gametype cfgs (optional)

The wizard wrote one `<stem>.cfg` per gametype/mod you used (e.g.
`baseq3/ffa.cfg`, `baseq3/tdm.cfg`, `missionpack/ctf-ta.cfg`). All
servers of that gametype share the file, so editing it changes every
instance. They're fine as-is for getting started; tweak fraglimits,
map cycles, sv_hostname, etc. as you like.

Example:

```bash
sudo -u quake vi /usr/lib/quake3/baseq3/ffa.cfg
```

To customize a single instance instead of the whole gametype, copy
the shared cfg to a per-instance file (e.g. `tdm-pro.cfg`) and point
the server's `.env` `+exec` at the copy:

```bash
sudo cp /usr/lib/quake3/baseq3/tdm.cfg /usr/lib/quake3/baseq3/tdm-pro.cfg
sudo vi /etc/trinity/tdm-2.env   # change "+exec tdm.cfg" → "+exec tdm-pro.cfg"
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
- A `dl.<hostname>` fast-download vhost (HTTP on `:80`, HTTPS on
  `:443`) — Quake 3 clients pull missing custom-map pk3s from here
  when they connect to your servers. Without it, the engine falls
  back to UDP downloads through the q3 server itself; functional,
  but slow enough on any nontrivial pk3 to frustrate players into
  disconnecting.

**`trinity init` handles this automatically.** During the apply phase
the wizard installs nginx + certbot, opens ports 80/443/tcp and
27960-28000/udp on UFW or firewalld, runs `certbot --nginx` to get a
Let's Encrypt SAN cert covering both `<hostname>` and
`dl.<hostname>`, then writes an HTTPS vhost serving `/demos/`,
`/assets/levelshots/`, and `/demopk3s/` (all CORS-enabled for the
hub's WASM player) plus a `dl.<hostname>` fast-download vhost over
`/usr/lib/quake3/` with `pak0.pk3` blocked.

Pre-flight: both `<hostname>` and `dl.<hostname>` must resolve to
this box before running `trinity init`, or the cert fetch will time
out at the ACME HTTP-01 validation step for whichever name is
missing. (Cloud-side firewalls — Vultr, Hetzner, etc. — also need
ports 80/443/tcp and 27960-28000/udp opened in their dashboard; the
wizard only handles the host-local firewall.)

If you ever need to re-run the nginx setup (rotate the cert manually,
change the public hostname, etc.) the same script lives at
`scripts/bootstrap-nginx.sh` and can be invoked directly:

```bash
sudo scripts/bootstrap-nginx.sh \
  --mode=collector \
  --hostname=q3.example.com \
  --email=you@example.com
```

After the wizard completes, populate the static asset directories from your pk3s:

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

This writes the env file (and the shared `<stem>.cfg` +
`rotation.<stem>` if no other server of that gametype is already
using them), appends the entry to `/etc/trinity/config.yml`, and
enables the systemd unit. Restart trinity
(`sudo systemctl restart trinity`) and start the new server
(`sudo systemctl start quake3-server@<key>`).

`trinity server remove <key>` disables the systemd unit, archives
the env file as `<key>.env.removed-<timestamp>`, and removes the
config entry. The shared `<stem>.cfg`, the rotation file, and the
log file are left alone — they're operator content.

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
