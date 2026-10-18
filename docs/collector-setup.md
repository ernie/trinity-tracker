# Setting up a Trinity collector

When you're done, matches on a Quake III Arena server you run show up on
a Trinity hub's leaderboard (`trinity.run` or your own), players get
welcome and `!claim` chat messages, and recorded demos play in the
hub's web player. For architecture, lifecycle and security background,
see [distributed-deployment.md](./distributed-deployment.md).

The stack:

| Component | What it does |
|-----------|--------------|
| **trinity-engine** | The Trinity engine, a Quake3e fork: the dedicated server binary (`trinity.ded`) and the Trinity mod's QVMs, shipped in one release zip. Plain ioquake3 won't work, because the tracker reads log lines that only the Trinity engine and mod write. |
| **trinity-tracker** | The `trinity` daemon (this repo). It tails the game server's log and publishes facts to the hub. |
| **nginx + certbot** | Required. Serves recorded demos and levelshots over HTTPS so the hub's pages can load them: the hub redirects viewers to your `public_url`, and an HTTPS page can't fetch from a plain-HTTP host. Also runs a `dl.<hostname>` fast-download vhost, over HTTP and HTTPS on the same `:80`/`:443`, so players can fetch custom-map pk3s they're missing when they connect. See §6. |

---

## Prerequisites

- **A Linux host.** Debian and Ubuntu are the main targets. The
  installer looks for `apt` or `pacman` and stops if it finds neither.
  Fedora and RHEL aren't supported: nothing installs the packages
  there, and SELinux is untested. If you know those platforms, you can
  install the base packages by hand and run from a checkout, but the
  curl|bash path won't get you there.
- **A retail `pak0.pk3`** for baseq3, and a second one for missionpack
  if you want the Team Arena game types (One Flag CTF, Overload,
  Harvester). The Trinity engine release includes the engine and the
  Trinity mod, but the retail Quake III Arena asset pak can't be
  redistributed, and the engine won't run without it. The wizard
  places it during its pak step (§3). To skip the path prompt, copy
  your retail pak0 files into the directory you'll run the installer
  from, renamed:
  - `q3-pak0.pk3` for baseq3
  - `mp-pak0.pk3` for missionpack (only if you'll run Team Arena game types)

  > The free Quake III Arena demo isn't supported. You need the retail
  > game.
- **Outbound TCP/4222** to your hub (`trinity.run` by default). This is
  NATS over TLS, and the collector checks the hub's certificate against
  the system CA roots, so a strict egress firewall only needs to allow
  plain TCP/4222, with no TLS-inspecting proxy in the way.
- **Inbound UDP** on the ports your game servers bind (usually 27960,
  27961 and up).
- **Inbound TCP/80 and TCP/443** for nginx, as the next item explains.
- **A public hostname pointing at this box, plus a `dl.<hostname>` A
  or AAAA record for the same box.** Both are required to join the
  network. §6 sets up nginx and one Let's Encrypt certificate covering
  both names: the site, demo and levelshot URLs live on `<hostname>`,
  and `dl.<hostname>` serves pk3 downloads to players over HTTP and
  HTTPS. Neither needs to be running before the wizard, but both DNS
  records must exist and `:80` must be open so the certificate request
  and fast downloads work afterward.
- **A hub account and an approved source** (§1). You'll come away with
  a source name, such as `mygamesite-jfk`, and a `.creds` file for it.

---

## 1. Request a source on the hub

Before you install, you need an approved source and its `.creds` file.
Both come from the hub's website:

1. Sign up or log in at the hub: `https://trinity.run` for the main
   network, or the URL your hub's admin gave you.
2. Click **Add servers** at the top right. If you already have a
   source, click **My servers**, then **Add another source**. Enter a
   **Source name** of 3–16 characters: letters, numbers, `_` and `-`.
   One request covers one machine, and each machine gets its own
   credentials, so if you run hosts in several regions, a name plus an
   airport code works well (`mygamesite-jfk`, `mygamesite-fra`).
   **What is this for?** is optional.
3. Submit. The button changes to **Request pending** until an admin
   reviews the request. You can leave the page open or come back
   later: it checks every 30 seconds and changes to **My servers** once
   the request is approved.
4. Click **My servers**, then **Download .creds** on the new source.
   The file is the secret that lets your collector connect to the
   hub's NATS broker as that source, so guard it like an SSH key.

If your hub has no public website, or its admin prefers to set you up
directly, ask them. For `trinity.run`, ask in the
[Trinity Discord](https://discord.gg/uJ5EPAXKfs).

---

## 2. Run the installer

The one-line installer fetches `install.sh`, installs the OS packages
Trinity needs, downloads the prebuilt `trinity` binary for this host's
architecture, and starts a collector-only wizard for the rest.

```bash
curl -fsSL https://raw.githubusercontent.com/ernie/trinity-tracker/main/scripts/install.sh \
    | sudo bash
```

To work from a checkout instead, `git clone` the repo and run
`sudo ./scripts/install.sh` from inside it. It's the same wizard.

The wizard asks for:

- **Hub hostname:** such as `trinity.run`, or your own hub.
- **Public URL:** this host's public HTTPS URL, such as
  `https://q3.example.com`. The hub stores the bare hostname as your
  game servers' address and uses the full URL as the base for demo
  downloads. Required.
- **Source ID:** the source name from §1, as shown in **My servers** on
  the hub (or the name your hub's admin gave you). It must match the
  subject in the `.creds` file. If the `.creds` file you downloaded is
  in the directory you run the installer from, the wizard offers it and
  takes the source ID from its file name.
- **Path to your .creds file:** where the file is on this host. The
  installer copies it to `/etc/trinity/source.creds` (mode 0640,
  root:quake).
- **Servers:** a game type for each game server you want to run.
  Trinity runs the four Quake III Arena game types (Free For All,
  Tournament, Team Deathmatch, Capture the Flag) and the three Team
  Arena ones (One Flag CTF, Overload, Harvester). The Team Arena game
  types install to `missionpack/` and need the Team Arena `pak0.pk3`.
  The wizard picks each server's port and an RCON password, or takes
  yours. Every server of one game type shares one `<stem>.cfg` (all
  TDM servers run `tdm.cfg`, for example); to customize one server,
  change the `+exec` in its `.env` file (§4).

After you confirm, the installer:
- creates the `quake` system user if it's missing, and the standard
  directories;
- writes `/etc/trinity/config.yml`;
- copies your `.creds` file to `/etc/trinity/source.creds`;
- downloads the Trinity engine release into `/usr/lib/quake3/` and
  points the engine's per-mod `logs` symlinks at `/var/log/quake3/`;
- installs the systemd units (`trinity.service`, `quake3-server@.service`,
  `quake3-servers.target`) and `/etc/logrotate.d/quake3`;
- writes `/usr/lib/quake3/baseq3/trinity.cfg`, with the cvars Trinity
  requires (collector wiring, console tap, TrinityVision recording and
  live broadcast) and recommended settings such as the pair that
  discards bot-only recordings, plus an `autoexec.cfg` that runs it, if
  no autoexec.cfg exists yet;
- writes `<key>.env` for each server (its port and `+exec <stem>.cfg`)
  and one shared `<stem>.cfg` and `rotation.<stem>` per game type and
  mod;
- enables `trinity.service`, `quake3-servers.target` and each
  `quake3-server@<key>.service`.

Running `trinity init` again won't overwrite an existing
`/etc/trinity/config.yml`. To start over, delete that file first.

---

## 3. Pak files (retail `pak0` and 1.32 patch data)

When the configuration steps are done, the wizard places the pak
files: the retail `pak0.pk3` files you supply, and the 1.32
point-release patch data, which id Software distributes under its EULA.

**Retail `pak0.pk3`.** If you copied your retail pak0 into the
directory you ran the installer from as `q3-pak0.pk3` (and
`mp-pak0.pk3` for Team Arena), the wizard finds and installs them
without asking. Otherwise it asks for a path. Any location and any file
name work; the wizard copies the file in as `pak0.pk3`. Press Enter at
an empty prompt to skip, then copy the file to
`/usr/lib/quake3/baseq3/pak0.pk3` (and
`/usr/lib/quake3/missionpack/pak0.pk3`) yourself before starting the
servers.

**Patch data.** If any of `pak1.pk3`–`pak8.pk3` are missing from
`baseq3/` (or `pak1.pk3`–`pak3.pk3` from `missionpack/` on a Team Arena
install), the wizard offers to fetch `quake3-1.32-pk3s.zip` (about
26 MB). It shows the id Software EULA with `more` and asks you to agree
before downloading. If you decline, it prints how to finish by hand
and moves on.

The wizard looks for the zip in this order:

1. **The current directory.** Put `quake3-1.32-pk3s.zip` in the
   directory you ran the installer from. The first hub on a network
   gets the zip this way, since there's no other hub to fetch it from.
   A hub install also copies the zip into
   `/var/lib/trinity/web/downloads/` so later collectors can fetch it
   from this hub.
2. **The hub's static directory.** `<static_dir>/downloads/quake3-1.32-pk3s.zip`,
   which the hub serves over HTTPS. This is where a hub finds it when
   you run the wizard there again.
3. **A remote hub.** `https://<hub>/downloads/quake3-1.32-pk3s.zip`,
   which is where collectors get it.

The zip should have `baseq3/` and `missionpack/` directories at its top
level, with the pak files inside.

The hub also publishes the EULA at `https://<hub>/quake3-eula`, where
visitors scroll to the end and agree before the zip download unlocks.

The wizard never overwrites files you placed; it only fills in missing
ones.

**Auto-start.** Once every required pak is in place, the wizard offers
to run `systemctl start trinity.service quake3-servers.target` for you.
If anything is missing, it lists the missing files and prints the start
command to run once you've placed them.

---

## 4. Tune your game type cfgs (optional)

The wizard wrote one `<stem>.cfg` for each game type and mod you chose
(such as `baseq3/ffa.cfg`, `baseq3/tdm.cfg`, `missionpack/ctf-ta.cfg`).
Every server of that game type shares the file, so an edit changes all
of them. They work as they are; change frag limits, the starting map,
sv_hostname and so on as you like.

Example:

```bash
sudo -u quake vi /usr/lib/quake3/baseq3/ffa.cfg
```

To customize one server instead of the whole game type, copy the
shared cfg to a new file (such as `tdm-pro.cfg`) and point that
server's `.env` `+exec` at the copy:

```bash
sudo cp /usr/lib/quake3/baseq3/tdm.cfg /usr/lib/quake3/baseq3/tdm-pro.cfg
sudo vi /etc/trinity/tdm-2.env   # change "+exec tdm.cfg" → "+exec tdm-pro.cfg"
```

The cvars Trinity requires (`g_logSync`, `g_trinityHandshake`,
`sv_conTap`, your RCON password, sv_tv*, the fast-download URL) are in
`/usr/lib/quake3/baseq3/trinity.cfg`, and in `missionpack/trinity.cfg`
if any of your servers run Team Arena game types. The generated
`autoexec.cfg` runs them. Change them only if you know what you're
doing.

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

On the hub, your source shows as healthy in **My servers** within about
30 seconds of its first heartbeat.

**End-to-end check:**

1. Connect to your game server.
2. You should see a welcome message in chat (`Welcome back, ernie^7!`
   if your GUID is already linked, or a greeting that asks you to type
   !claim if it isn't). If nothing appears, check that the RCON
   password in `trinity.cfg` matches the one in
   `/etc/trinity/config.yml`. The wizard writes the same password to
   both; if you change one by hand, change the other too.
3. Play a match to its end (frag or time limit). Reload the hub's
   matches page: your match appears a few seconds after the map ends.

---

## 6. Set up nginx and the fast-download vhost

This step is **required to join the network**. Your host serves two
things publicly:

- `/demos/`, `/assets/levelshots/` and `/demopk3s/` over HTTPS. The hub
  redirects viewers to your `public_url` for matches recorded on your
  host, and an HTTPS page on the hub can't load content from a
  plain-HTTP origin. Without HTTPS, demos don't play.
- A `dl.<hostname>` fast-download vhost (HTTP on `:80`, HTTPS on
  `:443`). Players' clients fetch missing custom-map pk3s from it when
  they connect to your servers. Without it, the engine falls back to
  UDP downloads through the game server itself, which work but are
  slow enough on any sizable pk3 that players give up and disconnect.

**`trinity init` does this for you.** While applying, the wizard
installs nginx and certbot, opens ports 80/443/tcp and 27960-28000/udp
in UFW or firewalld, runs `certbot --nginx` to get one Let's Encrypt
certificate covering both `<hostname>` and `dl.<hostname>`, and writes
an HTTPS vhost serving `/demos/`, `/assets/levelshots/` and
`/demopk3s/` (all with CORS for the hub's web player), plus the
`dl.<hostname>` fast-download vhost over `/usr/lib/quake3/`, with
`pak0.pk3` blocked.

Before you run `trinity init`, both `<hostname>` and `dl.<hostname>`
must resolve to this box, or the certificate request times out on
whichever name is missing. Cloud providers' firewalls (Vultr, Hetzner
and others) also need ports 80/443/tcp and 27960-28000/udp opened in
their dashboards; the wizard only configures the host's own firewall.

To run the nginx setup again (to renew the certificate by hand, change
the public hostname, and so on), run the script the wizard uses,
`scripts/bootstrap-nginx.sh`, directly:

```bash
sudo scripts/bootstrap-nginx.sh \
  --mode=collector \
  --hostname=q3.example.com \
  --email=you@example.com
```

After the wizard finishes, fill the static asset directories from your
pk3s:

```bash
sudo -u quake trinity levelshots /usr/lib/quake3
sudo -u quake trinity demobake   /usr/lib/quake3
```

`levelshots` extracts a JPG for each map; `demobake` builds the
baseline and per-map pk3s that the web demo player streams.

---

## 7. Adding more servers later

Use `trinity server add`:

```bash
# Interactive (gametype menu, port, RCON, …)
sudo trinity server add

# Or non-interactively from a script:
sudo trinity server add 1v1 --gametype tournament --port 27961
```

This writes the env file (and the shared `<stem>.cfg` and
`rotation.<stem>`, unless another server of that game type already uses
them), adds the entry to `/etc/trinity/config.yml`, and enables the
systemd unit. Restart trinity (`sudo systemctl restart trinity`) and
start the new server (`sudo systemctl start quake3-server@<key>`).

`trinity server remove <key>` disables the systemd unit, renames the
env file to `<key>.env.removed-<timestamp>`, and removes the config
entry. It leaves the shared `<stem>.cfg`, the rotation file and the log
file alone, since those are yours.

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `Error sending print to client N on server K: RCON not configured for this server` in `journalctl -fu trinity` | `rcon_password` is empty in `/etc/trinity/config.yml`, or doesn't match `rconpassword` in `trinity.cfg` | Set both to the same value and restart trinity. |
| `natsbus.RPCClient: ... auth violation` or "permissions violation" at startup | The wrong `.creds` file, your source was deactivated on the hub, or your credentials were rotated | Open **My servers** on the hub and check that the source is still active and healthy. To get working credentials, click **Rotate creds**; if you only lost the file and don't need to cancel the old one, click **Download .creds**. |
| Source stuck at "stale" in **My servers** | `trinity.service` isn't running, or outbound TCP/4222 (NATS) is blocked | `sudo systemctl status trinity`; `nc -vz <hub_host> 4222`. |
| Repeated `unknown event: TrinityHandshake:` in the game server log | The Trinity mod isn't loaded: the release zip wasn't extracted into `baseq3/`, or `fs_game` points at another mod | Check that `/usr/lib/quake3/baseq3/pak8t.pk3` exists, then restart the game server. |
| No `DemoSaved:` lines ever appear in the game server log | The engine isn't the Trinity build (plain ioquake3, for example), or `sv_tvAuto 0` | Check that `/usr/lib/quake3/trinity.ded` is the symlink the installer set up, and check the cvars in `trinity.cfg`. |
| Match cards on the hub have no play button | The recording was discarded (too few players, a server crash, a manual abort), so the demo was never finished. This is expected. | Nothing to fix. The hub shows the play button only when `matches.demo_available` is set. |
| Game server logs grow to gigabytes | `/etc/logrotate.d/quake3` was removed or never installed | Delete `/etc/trinity/config.yml` and run `sudo trinity init` again, or copy the snippet from `scripts/logrotate.quake3`. |
| Demo playback on the hub hangs or 404s on a custom map | The hub can't reach your nginx, or `trinity demobake` never ran for that map's pk3 | See §6. Make sure the map's pk3 is still in `/usr/lib/quake3/baseq3/`, then run `trinity demobake` again. |
