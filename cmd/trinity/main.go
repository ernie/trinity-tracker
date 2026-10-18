// trinity is the Trinity tracker: statistics, collector, hub and tools.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ernie/trinity-tracker/cmd/trinity/setup"
	"github.com/ernie/trinity-tracker/internal/api"
	"github.com/ernie/trinity-tracker/internal/assets"
	"github.com/ernie/trinity-tracker/internal/auth"
	"github.com/ernie/trinity-tracker/internal/collector"
	"github.com/ernie/trinity-tracker/internal/config"
	"github.com/ernie/trinity-tracker/internal/console"
	"github.com/ernie/trinity-tracker/internal/directory"
	"github.com/ernie/trinity-tracker/internal/discord"
	"github.com/ernie/trinity-tracker/internal/domain"
	"github.com/ernie/trinity-tracker/internal/hub"
	"github.com/ernie/trinity-tracker/internal/livestream"
	"github.com/ernie/trinity-tracker/internal/natsbus"
	"github.com/ernie/trinity-tracker/internal/notify"
	"github.com/ernie/trinity-tracker/internal/storage"
	"github.com/ftrvxmtrx/tga"
	"github.com/nats-io/nats.go"
	flag "github.com/spf13/pflag"
	"golang.org/x/image/draw"
	"golang.org/x/term"
)

var version = "dev"

const defaultConfigPath = "/etc/trinity/config.yml"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "update":
		cmdUpdate(os.Args[2:])
	case "_helper":
		cmdHelper(os.Args[2:])
	case "serve":
		cmdServe(os.Args[2:])
	case "server":
		cmdServer(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "players":
		cmdPlayers(os.Args[2:])
	case "matches":
		cmdMatches(os.Args[2:])
	case "leaderboard":
		cmdLeaderboard(os.Args[2:])
	case "discord-digest":
		cmdDiscordDigest(os.Args[2:])
	case "user":
		cmdUser(os.Args[2:])
	case "login":
		cmdLogin(os.Args[2:])
	case "logout":
		cmdLogout(os.Args[2:])
	case "console":
		cmdConsole(os.Args[2:])
	case "levelshots":
		cmdLevelshots(os.Args[2:])
	case "portraits":
		cmdPortraits(os.Args[2:])
	case "heads":
		cmdHeads(os.Args[2:])
	case "medals":
		cmdMedals(os.Args[2:])
	case "skills":
		cmdSkills(os.Args[2:])
	case "objectives":
		cmdObjectives(os.Args[2:])
	case "arenas":
		cmdArenas(os.Args[2:])
	case "assets":
		cmdAssets(os.Args[2:])
	case "mapsync":
		cmdMapsync(os.Args[2:])
	case "demobake":
		cmdDemobake(os.Args[2:])
	case "unpackmaps":
		cmdUnpackmaps(os.Args[2:])
	case "maps":
		cmdMaps(os.Args[2:])
	case "version":
		fmt.Printf("trinity %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: trinity <command> [options] [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init [--no-systemd] [--dry-run]     Interactive install wizard (collector-only by default)")
	fmt.Println("  update [--check] [--dry-run]        Update tracker binary, web bundle, engine, and mod from GitHub releases")
	fmt.Println("  serve                               Start the stats server")
	fmt.Println("  server list                         Show configured game servers")
	fmt.Println("  server add [<key>] [--gametype X] [--port N] [flags]")
	fmt.Println("                                      Add a game server instance (interactive on a TTY)")
	fmt.Println("  server remove <key>                 Remove a game server instance")
	fmt.Println("  status                              Run health checks, plus live game-server status on a hub")
	fmt.Println("  players [--humans]                  Show current players across all servers")
	fmt.Println("  matches [--recent N]                Show recent matches (default: 20)")
	fmt.Println("  leaderboard [--top N]               Show top players (default: 20)")
	fmt.Println("  discord-digest [--period P] [--dry-run]")
	fmt.Println("                                      Post a leaderboard digest to a Discord webhook")
	fmt.Println("  user add [--admin] [--player-id N] <username>")
	fmt.Println("                                      Add a user (prompts for password)")
	fmt.Println("  user remove <username>              Remove a user")
	fmt.Println("  user list                           List all users")
	fmt.Println("  user reset <username>               Reset a user's password")
	fmt.Println("  user admin <username>               Toggle admin status for a user")
	fmt.Println("  user disable <username>             Disable an account (soft-delete; keeps audit history)")
	fmt.Println("  user enable <username>              Re-enable a disabled account")
	fmt.Println("  login [--url <hub>]                 Log in and store a personal access token")
	fmt.Println("  logout                              Revoke and discard the stored token")
	fmt.Println("  console                             List servers you can control")
	fmt.Println("  console <key> [command...]          Send rcon (interactive console without command)")
	fmt.Println("  console --follow <key>              Stream console output read-only")
	fmt.Println("  levelshots [path]                   Extract levelshots from pk3 file(s)")
	fmt.Println("  portraits [path]                    Extract player portraits from pk3 file(s)")
	fmt.Println("  heads [path]                        Extract 3D head bundles from pk3 file(s)")
	fmt.Println("  medals [path]                       Extract medal icons from pk3 file(s)")
	fmt.Println("  skills [path]                       Extract skill icons from pk3 file(s)")
	fmt.Println("  objectives [path]                   Extract objective-state icons (CTF flags, Harvester skulls) from pk3 file(s)")
	fmt.Println("  arenas [path]                       Extract map longnames into maps.json")
	fmt.Println("  assets [path]                       Extract all assets (portraits, heads, medals, skills, objectives, levelshots)")
	fmt.Println("  mapsync [path]                      Regenerate every map-derived artifact: levelshots, maps.json, demo pk3s")
	fmt.Println("  demobake [path]                     Build baseline pk3, map pk3s, and manifest for web demo playback")
	fmt.Println("  unpackmaps <pk3>... [--output dir] [--prefix p] [--game baseq3|missionpack] [--quake3-dir d] [--dry-run]")
	fmt.Println("                                      Split map pack(s) into standalone per-map pk3s, with their .aas files")
	fmt.Println("  maps [--mode <mode>] [path]         Scan pk3s and report which game modes each map supports")
	fmt.Println("  version                             Show version")
	fmt.Println("  help                                Show this help")
	fmt.Println()
	fmt.Println("Global options:")
	fmt.Println("  --config <path>    Path to configuration file (default /etc/trinity/config.yml)")
	fmt.Println("  --url <url>        Base URL of the tracker (default: derived from config)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  sudo trinity init                          # interactive setup")
	fmt.Println("  sudo trinity server add                    # asks for game type, port and the rest")
	fmt.Println("  sudo trinity server add ffa --gametype ffa --port 27960")
	fmt.Println("  trinity serve --config /etc/trinity/config.yml")
	fmt.Println("  trinity players --humans")
	fmt.Println("  trinity matches --recent 50")
	fmt.Println("  trinity user add --admin myuser")
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfgPath := *configPath
	if cfgPath == "" {
		if _, err := os.Stat(defaultConfigPath); err == nil {
			cfgPath = defaultConfigPath
		} else {
			log.Fatalf("No config file found at %s. Use --config to specify a config file.", defaultConfigPath)
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Trinity %s starting...", version)
	log.Printf("Monitoring %d servers", len(cfg.Q3Servers))

	// config.Load defaults a missing tracker block to hub plus local collector.
	hasHub := cfg.Tracker.Hub != nil
	hasCollector := cfg.Tracker.Collector != nil

	var store *storage.Store
	if hasHub {
		s, err := storage.New(cfg.Database.Path)
		if err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		defer s.Close()
		store = s
		log.Printf("Database initialized at %s", cfg.Database.Path)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		writerOpts         []hub.Option
		ns                 *natsbus.Server
		subNC              *nats.Conn
		collectorNC        *nats.Conn
		subscriber         *natsbus.Subscriber
		rpcServer          *natsbus.RPCServer
		regSubscriber      *natsbus.RegistrationSubscriber
		registrar          *natsbus.Registrar
		remotePoller       *hub.RemotePoller
		bufferedPublisher  *natsbus.BufferedPublisher
		collectorRPC       *natsbus.RPCClient
		collectorSpill     *natsbus.SpillQueue
		livePublisher      *natsbus.LivePublisher
		liveSubscriber     *natsbus.LiveSubscriber
		collectorSource    string
		collectorWatermark *natsbus.WatermarkTracker
	)

	if hasHub {
		storeParent := filepath.Dir(cfg.Database.Path)
		var err error
		ns, err = natsbus.Start(cfg.Tracker, storeParent, store)
		if err != nil {
			log.Fatalf("Failed to start embedded NATS: %v", err)
		}
		log.Printf("Embedded NATS listening on %s (store=%s)", ns.ClientURL(), ns.StoreDir())
		subNC, err = ns.ConnectInternal(nats.Name("trinity-hub-subscriber"))
		if err != nil {
			log.Fatalf("Failed to connect hub subscriber NATS client: %v", err)
		}
		defer subNC.Close()
	}
	if hasCollector {
		opts := []nats.Option{
			nats.Name("trinity-collector"),
			// Retry forever: residential links drop for longer than the
			// default two-minute reconnect budget.
			nats.MaxReconnects(-1),
			nats.ReconnectWait(5 * time.Second),
			nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
				if err != nil {
					log.Printf("Collector NATS disconnected: %v", err)
				} else {
					log.Printf("Collector NATS disconnected")
				}
			}),
			nats.ClosedHandler(func(_ *nats.Conn) {
				log.Fatalf("Collector NATS connection permanently closed; exiting so systemd restarts us")
			}),
		}
		var connURL string
		switch {
		case ns != nil:
			// A co-located collector uses the hub's internal creds.
			opts = append(opts, nats.InProcessServer(ns.NATSServer()), nats.UserCredentials(ns.Auth().InternalCredsPath()))
		default:
			creds := cfg.Tracker.NATS.CredentialsFile
			if creds == "" {
				log.Fatalf("tracker.nats.credentials_file is required in collector-only mode")
			}
			opts = append(opts,
				nats.UserCredentials(creds),
				nats.CustomInboxPrefix(natsbus.InboxPrefixFor(cfg.Tracker.Collector.SourceID)),
			)
			log.Printf("Collector using credentials file: %s", creds)
			connURL = cfg.Tracker.NATS.URL
			if connURL == "" {
				log.Fatalf("tracker.nats.url or tracker.collector.hub_host must be set in collector-only mode")
			}
		}
		var err error
		collectorNC, err = nats.Connect(connURL, opts...)
		if err != nil {
			log.Fatalf("Failed to connect collector NATS client: %v", err)
		}
		defer collectorNC.Close()
		if connURL != "" {
			log.Printf("Collector connected to NATS hub at %s", connURL)
		}
		collectorSource = cfg.Tracker.Collector.SourceID
		wm, err := natsbus.NewWatermarkTracker(cfg.Tracker.Collector.DataDir)
		if err != nil {
			log.Fatalf("Failed to load publish watermark: %v", err)
		}
		collectorWatermark = wm
		rc, err := natsbus.NewRPCClient(collectorNC, collectorSource, 0)
		if err != nil {
			log.Fatalf("Failed to build RPC client: %v", err)
		}
		collectorRPC = rc
		// Start from the hub's consumed_seq when it's ahead of the local
		// watermark, so a reinstalled collector's events aren't dropped as
		// replays.
		initialSeq := wm.Current().LastSeq
		if hasHub && store != nil {
			if prog, perr := store.GetSourceProgress(ctx, collectorSource); perr == nil && prog.ConsumedSeq > initialSeq {
				log.Printf("Collector seeding seq from hub: local=%d hub=%d", initialSeq, prog.ConsumedSeq)
				initialSeq = prog.ConsumedSeq
			}
		} else {
			if reply, perr := rc.GetSourceProgress(ctx, collectorSource); perr == nil && reply.ConsumedSeq > initialSeq {
				log.Printf("Collector seeding seq from hub: local=%d hub=%d", initialSeq, reply.ConsumedSeq)
				initialSeq = reply.ConsumedSeq
			} else if perr != nil {
				log.Printf("Collector seq seed: hub.GetSourceProgress failed (%v); using local watermark seq=%d", perr, initialSeq)
			}
		}
		pub, err := natsbus.NewPublisherWithWatermark(collectorNC, collectorSource, initialSeq, wm)
		if err != nil {
			log.Fatalf("Failed to build fact-event publisher: %v", err)
		}
		spill, err := natsbus.NewSpillQueue(cfg.Tracker.Collector.DataDir)
		if err != nil {
			log.Fatalf("Failed to open publish spill queue: %v", err)
		}
		collectorSpill = spill
		bufPub := natsbus.NewBufferedPublisher(pub, natsbus.BufferedCapacity, spill)
		bufPub.Start(ctx)
		bufferedPublisher = bufPub
		log.Printf("Collector publishing as source=%s (last_seq=%d)", collectorSource, initialSeq)

		// A co-located hub already gets live events from the manager.
		if !hasHub {
			lp, err := natsbus.NewLivePublisher(collectorNC, collectorSource)
			if err != nil {
				log.Fatalf("Failed to build live-event publisher: %v", err)
			}
			livePublisher = lp
		}
	}

	collectorShutdown := func() {
		if registrar != nil {
			registrar.Stop()
		}
		if bufferedPublisher != nil {
			bufferedPublisher.Stop()
		}
		if collectorSpill != nil {
			if err := collectorSpill.Close(); err != nil {
				log.Printf("spill queue close on shutdown: %v", err)
			}
		}
		if collectorWatermark != nil {
			if err := collectorWatermark.Flush(); err != nil {
				log.Printf("watermark flush on shutdown: %v", err)
			}
		}
	}

	if hasHub {
		writerOpts = append(writerOpts, hub.WithPreStop(func() {
			collectorShutdown()
			if liveSubscriber != nil {
				liveSubscriber.Stop()
			}
			if remotePoller != nil {
				remotePoller.Stop()
			}
			if subscriber != nil {
				subscriber.Stop()
			}
			if regSubscriber != nil {
				regSubscriber.Stop()
			}
			if rpcServer != nil {
				rpcServer.Stop()
			}
			if ns != nil {
				ns.Stop()
			}
		}))
	}

	var writer *hub.Writer
	// The writer sets liveState from heartbeats; the poller reads it.
	var liveState *hub.LiveState
	if hasHub {
		// Greet gives admins rcon on the local source's servers without
		// admin_delegation_enabled.
		if hasCollector && collectorSource != "" {
			writerOpts = append(writerOpts, hub.WithLocalSource(collectorSource))
		}
		liveState = hub.NewLiveState()
		writerOpts = append(writerOpts, hub.WithLiveState(liveState))
		writer = hub.NewWriter(store, writerOpts...)
		writer.Start(ctx)
		defer writer.Stop()

		// A collector in the hub's process registers its own source.
		if hasCollector && collectorSource != "" {
			if err := store.UpsertLocalSource(ctx, collectorSource); err != nil {
				log.Fatalf("Failed to register local source: %v", err)
			}
			writer.MarkSourceApproved(collectorSource)
		}
	}

	if hasHub {
		var err error
		subscriber, err = natsbus.NewSubscriber(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to create fact-event subscriber: %v", err)
		}
		subscriber.Start(ctx)
		log.Printf("Hub subscribed to %s", natsbus.StreamEvents)

		rpcServer, err = natsbus.RegisterRPCHandlers(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to register RPC handlers: %v", err)
		}
		log.Printf("Hub RPC handlers registered (queue group %s)", natsbus.RPCQueueGroup)

		regSubscriber, err = natsbus.NewRegistrationSubscriber(subNC, writer)
		if err != nil {
			log.Fatalf("Failed to subscribe to registrations: %v", err)
		}
		log.Printf("Hub subscribed to %s*", natsbus.RegistrationSubjectPrefix)

	}

	if hasHub {
		remotePoller = hub.NewRemotePoller(store, collector.NewQ3Client(), cfg.Server.PollInterval, writer.Presence(), writer, ns)
		remotePoller.SetLiveState(liveState)
		remotePoller.Start(ctx)
		log.Printf("Hub polling every %v", cfg.Server.PollInterval)
	}

	// The activity notifier reads the poller's cached status, so it
	// comes after the poller.
	if hasHub && cfg.Discord != nil && cfg.Discord.Activity != nil && cfg.Discord.Activity.WebhookURL != "" {
		mapMeta := loadMapMeta(cfg.Server.StaticDir)
		activeDelay := time.Duration(cfg.Discord.Activity.ActiveDelaySeconds) * time.Second
		if cfg.Discord.Activity.ActiveDelaySeconds == 0 {
			activeDelay = time.Duration(config.DefaultActiveDelaySeconds) * time.Second
		}
		debounce := time.Duration(cfg.Discord.Activity.InactiveDebounceSeconds) * time.Second
		if cfg.Discord.Activity.InactiveDebounceSeconds == 0 {
			debounce = time.Duration(config.DefaultInactiveDebounceSeconds) * time.Second
		}
		publicURL := ""
		if cfg.Tracker.Collector != nil {
			publicURL = strings.TrimSuffix(cfg.Tracker.Collector.PublicURL, "/")
		}
		actNotifier := notify.NewNotifier(notify.ActivityConfig{
			WebhookURL:       cfg.Discord.Activity.WebhookURL,
			PublicURL:        publicURL,
			ActiveDelay:      activeDelay,
			InactiveDebounce: debounce,
		}, remotePoller, writer.Presence(), mapMeta)
		writer.SetActivityNotifier(actNotifier)
		log.Printf("Hub server-activity notifier active (active_delay=%s inactive_debounce=%s)", activeDelay, debounce)
	}

	if hasHub && cfg.Tracker.Hub.Directory != nil && cfg.Tracker.Hub.Directory.Enabled {
		d := cfg.Tracker.Hub.Directory
		dirSrv, err := directory.New(directory.Config{
			ListenAddr:         d.ListenAddr,
			Port:               d.Port,
			HeartbeatExpiry:    d.HeartbeatExpiry.D(),
			ChallengeTimeout:   d.ChallengeTimeout.D(),
			GateRefresh:        d.GateRefresh.D(),
			MaxServers:         d.MaxServers,
			Store:              store,
			Conns:              ns,
			RegistryStore:      store,
			PersistedFreshness: d.PersistedFreshness.D(),
		})
		if err != nil {
			log.Fatalf("Q3 directory server init: %v", err)
		}
		go func() {
			if err := dirSrv.Run(ctx); err != nil {
				log.Printf("Q3 directory server: %v", err)
			}
		}()
		defer dirSrv.Stop()
	}

	var (
		serverClient hub.ServerClient
		rpcClient    hub.RPCClient
		factPub      hub.FactPublisher
	)
	if hasCollector {
		serverClient = collectorRPC
		rpcClient = collectorRPC
		factPub = bufferedPublisher
	} else {
		serverClient = writer
		rpcClient = writer
		factPub = writer
	}
	liveReg := livestream.NewRegistry()
	manager := collector.NewServerManager(cfg, serverClient, rpcClient, factPub)
	if livePublisher != nil {
		manager.SetLivePublisher(livePublisher)
	}
	if cfg.Tracker.Collector.LiveRelayAddr != "" {
		manager.SetLiveRegistry(liveReg)
		// Tells each game server how many people are watching it live.
		watch := collector.NewWatchNotifier(liveReg.Viewers, func(key string, count int) {
			if _, err := manager.ExecuteRconByKey(key, fmt.Sprintf("trinity_watch %d", count)); err != nil {
				log.Printf("collector: trinity_watch %d to %s: %v", count, key, err)
			}
		})
		liveReg.SetWatchHooks(watch.ViewersChanged, watch.SessionStart)
	}
	consoleReg := console.NewRegistry()
	manager.SetConsoleRegistry(consoleReg)

	// Log lines older than a server's last published event only rebuild
	// state. The cutoff is per server so one server's events can't hide
	// another's from the same second.
	if collectorWatermark != nil {
		if wm := collectorWatermark.Current(); !wm.IsZero() {
			perServer := make(map[int64]time.Time, len(wm.PerServer))
			for id, sw := range wm.PerServer {
				perServer[id] = sw.LastTS
			}
			manager.SetReplayCutoffs(perServer)
		}
	}

	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Failed to start server manager: %v", err)
	}
	log.Printf("Server manager started, polling every %v", cfg.Server.PollInterval)

	// A reconnect usually means the hub restarted with empty presence,
	// so republish it; the hub treats a repeat as a no-op.
	if collectorNC != nil && hasCollector {
		collectorNC.SetReconnectHandler(func(*nats.Conn) {
			log.Printf("Collector NATS reconnected; re-bootstrapping presence")
			manager.BootstrapAll()
		})
	}

	var liveRelaySrv *http.Server

	// After the manager, so the first heartbeat carries the real roster.
	if hasCollector {
		var err error
		registrar, err = natsbus.NewRegistrar(
			collectorNC,
			collectorSource,
			version,
			cfg.Tracker.Collector.PublicURL,
			manager.Roster,
			cfg.Tracker.Collector.HeartbeatInterval.D(),
		)
		if err != nil {
			log.Fatalf("Failed to build registrar: %v", err)
		}
		registrar.Start(ctx)
		log.Printf("Collector heartbeating every %v", cfg.Tracker.Collector.HeartbeatInterval.D())

		// A live stream starting or stopping heartbeats at once, so is_live
		// updates quickly.
		manager.SetRegistrarKick(registrar.Kick)

		// The handler checks each server's allow_hub_admin_rcon again
		// before running a proxied command.
		rconHandler := collector.NewRconProxyHandler(manager)
		if rconServer, err := natsbus.RegisterRconHandler(collectorNC, collectorSource, rconHandler); err != nil {
			log.Fatalf("Failed to register RCON proxy handler: %v", err)
		} else {
			defer rconServer.Stop()
		}

		// Console lines are forwarded while the hub holds a lease, under
		// the same check as rcon.
		if fwd, err := natsbus.RegisterConsoleForwarder(collectorNC, collectorSource, consoleReg, rconHandler); err != nil {
			log.Fatalf("Failed to register console forwarder: %v", err)
		} else {
			defer fwd.Stop()
		}

		if addr := cfg.Tracker.Collector.LiveRelayAddr; addr != "" {
			relay := livestream.NewRelayServer(liveReg)
			liveRelaySrv = &http.Server{Addr: addr, Handler: relay}
			go func() {
				log.Printf("Live relay listening on %s (target viewer delay %v; per-match holdback derived from the keyframe interval)", addr, cfg.Tracker.Collector.LiveDelay.D())
				if err := liveRelaySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Printf("Live relay server error: %v", err)
				}
			}()
		}
	}

	if !hasHub {
		log.Printf("Running in collector-only mode; no HTTP server")
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		if liveRelaySrv != nil {
			// Live streams never end on their own, so close them outright.
			_ = liveRelaySrv.Close()
		}
		log.Println("Stopping server manager...")
		manager.Stop()
		collectorShutdown()
		cancel()
		log.Println("Shutdown complete")
		return
	}

	authService := auth.NewService(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)
	if cfg.Auth.JWTSecret == "" {
		log.Printf("Warning: No JWT secret configured. Auth tokens will use an empty secret.")
	}

	router := api.NewRouter(store, manager, writer, authService, cfg.Server.StaticDir, cfg.Server.Quake3Dir)
	router.SetVersion(version)
	if remotePoller != nil {
		router.SetPoller(remotePoller)
		remotePoller.SetSink(router)
	}
	if ns != nil {
		router.SetUserProvisioner(ns.Auth())
	}
	// Local-source rcon goes through the in-process manager; remote
	// sources go over NATS, where subNC can publish to any source.
	if hasCollector {
		router.SetLocalSource(collectorSource)
	}
	if subNC != nil {
		rconClient, err := natsbus.NewRconClient(subNC, 0)
		if err != nil {
			log.Fatalf("Failed to create RCON client: %v", err)
		}
		router.SetRconClient(rconClient)
		consoleClient, err := natsbus.NewConsoleClient(subNC)
		if err != nil {
			log.Fatalf("Failed to create console client: %v", err)
		}
		router.SetConsoleClient(consoleClient)
	}
	if hasCollector {
		router.SetConsoleRegistry(consoleReg)
	}
	router.StartWebSocketHub()
	log.Printf("Serving static files from %s", cfg.Server.StaticDir)

	// The in-process collector's live events already reach the
	// WebSocket hub, so the subscriber skips collectorSource.
	if hasHub {
		ls, err := natsbus.NewLiveSubscriber(subNC, store, writer, router, collectorSource)
		if err != nil {
			log.Fatalf("Failed to start live subscriber: %v", err)
		}
		liveSubscriber = ls
		log.Printf("Hub subscribed to %s*", natsbus.SubjectLivePrefix)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		log.Printf("Web UI available at http://%s", addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-serverErr:
		log.Fatalf("HTTP server error: %v", err)
	}

	log.Println("Shutting down HTTP server...")
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer httpCancel()
	if err := server.Shutdown(httpCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	if liveRelaySrv != nil {
		_ = liveRelaySrv.Close()
	}

	log.Println("Stopping server manager...")
	manager.Stop()

	cancel()
	log.Println("Shutdown complete")
}

var (
	baseURL = "http://localhost:8080"
	dbPath  string
)

// loadMapMeta maps short names to long names from the maps.json that
// `trinity arenas` writes. Any problem returns nil, and the notifier
// shows short names.
func loadMapMeta(staticDir string) map[string]string {
	if staticDir == "" {
		return nil
	}
	path := filepath.Join(staticDir, "assets", "maps.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw map[string]assets.ArenaMeta
	if err := json.Unmarshal(data, &raw); err != nil {
		log.Printf("loadMapMeta: parse %s: %v", path, err)
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if v.LongName != "" {
			out[k] = v.LongName
		}
	}
	return out
}

func loadCLIConfigFromFlags(configPath, url string) *config.Config {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't load config from %s: %v\n", configPath, err)
		dbPath = "/var/lib/trinity/trinity.db"
		if url != "" {
			baseURL = url
		}
		return nil
	}

	// Collector-only installs have no database; storage.New("") fails cleanly.
	if cfg.Database != nil {
		dbPath = cfg.Database.Path
	}
	if url != "" {
		baseURL = url
	} else {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	}
	return cfg
}

func openStoreForCLI(configPath, url string) *storage.Store {
	loadCLIConfigFromFlags(configPath, url)
	store, err := storage.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't open the database: %v\n", err)
		os.Exit(1)
	}
	return store
}

// cmdStatus checks the install's health and then shows each game
// server's live state. It exits 1 on any failed check so scripts can
// use it.
func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker (overrides config)")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	failures := 0
	// Pad before coloring; %-12s counts escape bytes.
	pass := func(label, detail string) {
		fmt.Printf("  %s %s %s\n", green("✓"), bold(fmt.Sprintf("%-12s", label)), detail)
	}
	fail := func(label, detail string) {
		fmt.Printf("  %s %s %s\n", red("✗"), bold(fmt.Sprintf("%-12s", label)), detail)
		failures++
	}

	fmt.Printf("Trinity %s\n\n", version)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fail("Config", fmt.Sprintf("%s: %v", *configPath, err))
		fmt.Println("\n1 check failed.")
		os.Exit(1)
	}
	pass("Config", fmt.Sprintf("%s loaded, %d game servers", *configPath, len(cfg.Q3Servers)))

	if out, err := exec.Command("systemctl", "is-active", "trinity").Output(); err == nil {
		state := strings.TrimSpace(string(out))
		if state == "active" {
			pass("Service", "trinity.service is active")
		} else {
			fail("Service", fmt.Sprintf("trinity.service is %s (run: sudo systemctl start trinity)", state))
		}
	}
	if cfg.Database != nil && cfg.Database.Path != "" {
		if info, err := os.Stat(cfg.Database.Path); err == nil && !info.IsDir() {
			pass("Database", cfg.Database.Path)
		} else {
			fail("Database", fmt.Sprintf("%s missing: the service hasn't started successfully yet", cfg.Database.Path))
		}
	}

	isCollector := cfg.Tracker != nil && cfg.Tracker.Collector != nil
	isHub := cfg.Tracker == nil || cfg.Tracker.Hub != nil

	if isCollector {
		creds := cfg.Tracker.NATS.CredentialsFile
		if creds != "" {
			if _, err := os.Stat(creds); err == nil {
				pass("Credentials", creds)
			} else {
				fail("Credentials", fmt.Sprintf("%s: %v", creds, err))
			}
		}
		if hub := cfg.Tracker.Collector.HubHost; hub != "" {
			u := "https://" + hub + "/health"
			if reachable(u) {
				pass("Hub", hub+" reachable")
			} else {
				fail("Hub", u+" not reachable. Check DNS, the firewall and the hub's status.")
			}
		}
		if pub := cfg.Tracker.Collector.PublicURL; pub != "" {
			if reachable(pub) {
				pass("Public URL", pub+" reachable")
			} else {
				fail("Public URL", pub+" not reachable. Is nginx down, or does DNS point somewhere else?")
			}
		}
	}

	httpReachable := false
	if isHub {
		listen := cfg.Server.ListenAddr
		if listen == "" || listen == "0.0.0.0" {
			listen = "127.0.0.1"
		}
		u := fmt.Sprintf("http://%s:%d/health", listen, cfg.Server.HTTPPort)
		if reachable(u) {
			pass("HTTP", u+" → 200")
			httpReachable = true
		} else {
			fail("HTTP", u+" not reachable. Is the service running but not listening?")
		}
	}

	fmt.Println()
	if failures > 0 {
		fmt.Printf("%s\n", red(fmt.Sprintf("%d check(s) failed.", failures)))
	} else {
		fmt.Println(green("All checks passed."))
	}

	if isHub && httpReachable {
		fmt.Println()
		printLiveServerTable(*configPath, *url)
	}

	if isCollector && !isHub && len(cfg.Q3Servers) > 0 {
		fmt.Println()
		printLocalServerTable(cfg)
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// printLiveServerTable doesn't count toward the health checks' exit code.
func printLiveServerTable(configPath, urlOverride string) {
	loadCLIConfigFromFlags(configPath, urlOverride)

	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "live status unavailable: %v\n", err)
		return
	}

	nameCol := column{header: "SERVER"}
	mapCol := column{header: "MAP"}
	playersCol := column{header: "PLAYERS", align: alignRight}
	humansCol := column{header: "HUMANS", align: alignRight}
	statusCol := column{header: "STATUS"}
	for _, srv := range servers {
		idF, _ := srv["id"].(float64)
		id := int64(idF)
		name := jsonString(srv, "key")
		if name == "" {
			name = fmt.Sprintf("server-%d", id)
		}

		var status map[string]interface{}
		if err := getJSON(fmt.Sprintf("/api/servers/%d/status", id), &status); err != nil {
			nameCol.cells = append(nameCol.cells, name)
			mapCol.cells = append(mapCol.cells, dim("-"))
			playersCol.cells = append(playersCol.cells, dim("-"))
			humansCol.cells = append(humansCol.cells, dim("-"))
			statusCol.cells = append(statusCol.cells, red("OFFLINE"))
			continue
		}

		mapName := jsonString(status, "map")
		if mapName == "" {
			mapName = "-"
		}
		players := 0
		if p, ok := status["players"].([]interface{}); ok {
			players = len(p)
		}
		humans, _ := status["human_count"].(float64)

		statusStr := green("ONLINE")
		if online, ok := status["online"].(bool); ok && !online {
			statusStr = red("OFFLINE")
		}

		nameCol.cells = append(nameCol.cells, name)
		mapCol.cells = append(mapCol.cells, mapName)
		playersCol.cells = append(playersCol.cells, fmt.Sprintf("%d", players))
		humansCol.cells = append(humansCol.cells, fmt.Sprintf("%d", int(humans)))
		statusCol.cells = append(statusCol.cells, statusStr)
	}
	renderTable(os.Stdout, []column{nameCol, mapCol, playersCol, humansCol, statusCol})
}

// printLocalServerTable polls each game server with getstatus, since a
// collector install has no local HTTP API.
func printLocalServerTable(cfg *config.Config) {
	type pollResult struct {
		status *domain.ServerStatus
		err    error
	}

	results := make([]pollResult, len(cfg.Q3Servers))
	var wg sync.WaitGroup
	client := collector.NewQ3Client()
	for i, srv := range cfg.Q3Servers {
		wg.Add(1)
		go func(i int, address string) {
			defer wg.Done()
			status, err := client.QueryStatus(address)
			results[i] = pollResult{status: status, err: err}
		}(i, srv.Address)
	}
	wg.Wait()

	nameCol := column{header: "SERVER"}
	mapCol := column{header: "MAP"}
	playersCol := column{header: "PLAYERS", align: alignRight}
	humansCol := column{header: "HUMANS", align: alignRight}
	statusCol := column{header: "STATUS"}
	for i, srv := range cfg.Q3Servers {
		name, mapName, players, humans, statusStr := localStatusCells(srv, results[i].status, results[i].err)
		nameCol.cells = append(nameCol.cells, name)
		mapCol.cells = append(mapCol.cells, mapName)
		playersCol.cells = append(playersCol.cells, players)
		humansCol.cells = append(humansCol.cells, humans)
		statusCol.cells = append(statusCol.cells, statusStr)
	}
	renderTable(os.Stdout, []column{nameCol, mapCol, playersCol, humansCol, statusCol})
}

// localStatusCells has no GUIDs to go on, so it counts a player with a
// ping of 0 as a bot. A human on a LAN with a true zero ping is miscounted.
func localStatusCells(srv config.Q3Server, status *domain.ServerStatus, err error) (name, mapName, players, humans, statusStr string) {
	name = srv.Key
	if err != nil || status == nil {
		return name, dim("-"), dim("-"), dim("-"), red("OFFLINE")
	}
	mapName = status.Map
	if mapName == "" {
		mapName = "-"
	}
	humanCount := 0
	for _, p := range status.Players {
		if p.Ping > 0 {
			humanCount++
		}
	}
	statusStr = green("ONLINE")
	if !status.Online {
		statusStr = red("OFFLINE")
	}
	return name, mapName, fmt.Sprintf("%d", len(status.Players)), fmt.Sprintf("%d", humanCount), statusStr
}

// jsonString returns "" for a missing or non-string field.
func jsonString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// reachable counts any answer below 500 as up.
func reachable(url string) bool {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func cmdPlayers(args []string) {
	fs := flag.NewFlagSet("players", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	humansOnly := fs.Bool("humans", false, "show only human players")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	loadCLIConfigFromFlags(*configPath, *url)

	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	serverCol := column{header: "SERVER"}
	playerCol := column{header: "PLAYER"}
	scoreCol := column{header: "SCORE", align: alignRight}
	pingCol := column{header: "PING", align: alignRight}
	typeCol := column{header: "TYPE"}

	for _, srv := range servers {
		id := int64(srv["id"].(float64))
		source, _ := srv["source"].(string)
		key, _ := srv["key"].(string)
		name := source + "/" + key

		var status map[string]interface{}
		if err := getJSON(fmt.Sprintf("/api/servers/%d/status", id), &status); err != nil {
			continue
		}

		players, ok := status["players"].([]interface{})
		if !ok {
			continue
		}

		for _, player := range players {
			pm, ok := player.(map[string]interface{})
			if !ok {
				continue
			}

			isBot := pm["is_bot"].(bool)
			if *humansOnly && isBot {
				continue
			}

			playerType := "Human"
			if isBot {
				playerType = "Bot"
			}

			coloredName, _ := pm["name"].(string)
			cleanName := pm["clean_name"].(string)
			score := int(pm["score"].(float64))
			ping := int(pm["ping"].(float64))

			// Bot rows render dim so the human/bot distinction is
			// visible at a glance even before scanning the TYPE col.
			nameCell := displayName(coloredName, cleanName)
			scoreCell := fmt.Sprintf("%d", score)
			pingCell := fmt.Sprintf("%d", ping)
			typeCell := playerType
			if isBot {
				nameCell = dim(cleanName) // Q3 colors would override the dim
				scoreCell = dim(scoreCell)
				pingCell = dim(pingCell)
				typeCell = dim(typeCell)
			}

			serverCol.cells = append(serverCol.cells, name)
			playerCol.cells = append(playerCol.cells, nameCell)
			scoreCol.cells = append(scoreCol.cells, scoreCell)
			pingCol.cells = append(pingCol.cells, pingCell)
			typeCol.cells = append(typeCol.cells, typeCell)
		}
	}

	renderTable(os.Stdout, []column{serverCol, playerCol, scoreCol, pingCol, typeCol})
}

func cmdMatches(args []string) {
	fs := flag.NewFlagSet("matches", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	limit := fs.Int("recent", 20, "number of recent matches to show")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	loadCLIConfigFromFlags(*configPath, *url)

	var matches []map[string]interface{}
	if err := getJSON(fmt.Sprintf("/api/matches?limit=%d", *limit), &matches); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	idCol := column{header: "ID", align: alignRight}
	mapCol := column{header: "MAP"}
	startedCol := column{header: "STARTED"}
	endedCol := column{header: "ENDED"}
	exitCol := column{header: "EXIT REASON"}

	for _, match := range matches {
		id := int64(match["id"].(float64))
		mapName := match["map_name"].(string)

		started := dim("-")
		if s, ok := match["started_at"].(string); ok {
			started = formatTime(s)
		}

		ended := green("In Progress")
		if e, ok := match["ended_at"].(string); ok && e != "" {
			ended = formatTime(e)
		}

		exitReason := dim("-")
		if r, ok := match["exit_reason"].(string); ok && r != "" {
			exitReason = faint(r)
		}

		idCol.cells = append(idCol.cells, fmt.Sprintf("%d", id))
		mapCol.cells = append(mapCol.cells, mapName)
		startedCol.cells = append(startedCol.cells, started)
		endedCol.cells = append(endedCol.cells, ended)
		exitCol.cells = append(exitCol.cells, exitReason)
	}

	renderTable(os.Stdout, []column{idCol, mapCol, startedCol, endedCol, exitCol})
}

func cmdLeaderboard(args []string) {
	fs := flag.NewFlagSet("leaderboard", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	limit := fs.Int("top", 20, "number of top players to show")
	category := fs.String("category", "frags",
		"category: frags, deaths, kd_ratio, matches, victories, captures, flag_returns, assists, defends, impressives, excellents, humiliations")
	period := fs.String("period", "all", "time window: day|week|month|year|all")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	spec, ok := digestCategoryRegistry[*category]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown category %q\n", *category)
		os.Exit(1)
	}

	loadCLIConfigFromFlags(*configPath, *url)

	var resp domain.LeaderboardResponse
	apiURL := fmt.Sprintf("/api/stats/leaderboard?category=%s&period=%s&limit=%d",
		*category, *period, *limit)
	if err := getJSON(apiURL, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	rankCol := column{header: "RANK", align: alignRight}
	playerCol := column{header: "PLAYER"}
	valueCol := column{header: spec.CLILabel, align: alignRight}
	matchesCol := column{header: "MATCHES", align: alignRight}

	for i, e := range resp.Entries {
		rank := fmt.Sprintf("%d", i+1)
		if i < 3 {
			rank = bold(rank)
		}

		name := discord.DisplayName(e.Player.Name, e.Player.IsVR)
		clean := discord.DisplayName(e.Player.CleanName, e.Player.IsVR)
		playerCell := fmt.Sprintf("%s %s %s",
			playerVerificationBadge(e.Player.IsVerified, e.Player.IsAdmin),
			playerPlatformBadge(e.Player.IsVR),
			displayName(name, clean),
		)

		rankCol.cells = append(rankCol.cells, rank)
		playerCol.cells = append(playerCol.cells, playerCell)
		valueCol.cells = append(valueCol.cells, spec.Format(e))
		matchesCol.cells = append(matchesCol.cells, fmt.Sprintf("%d", e.CompletedMatches))
	}

	renderTable(os.Stdout, []column{rankCol, playerCol, valueCol, matchesCol})
}

// cmdDiscordDigest posts a leaderboard digest to a Discord webhook. A
// systemd timer or cron runs it.
func cmdDiscordDigest(args []string) {
	fs := flag.NewFlagSet("discord-digest", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	period := fs.String("period", "week", "time window: day|week|month|year|all")
	topN := fs.Int("top", 5, "top entries per category (max 25)")
	webhookOverride := fs.String("webhook", "", "override discord.webhook_url (e.g. for staging)")
	dryRun := fs.Bool("dry-run", false, "print embed JSON to stdout, do not POST")
	fs.Parse(args)

	if *topN < 1 || *topN > 25 {
		fmt.Fprintf(os.Stderr, "Error: --top must be between 1 and 25\n")
		os.Exit(1)
	}

	cfg := loadCLIConfigFromFlags(*configPath, *url)

	// Prefer the public URL, so the digest also works from a host other
	// than the hub; --url still wins.
	if *url == "" && cfg != nil && cfg.Tracker != nil && cfg.Tracker.Collector != nil && cfg.Tracker.Collector.PublicURL != "" {
		baseURL = strings.TrimSuffix(cfg.Tracker.Collector.PublicURL, "/")
	}

	webhookURL := *webhookOverride
	if webhookURL == "" {
		if cfg == nil || cfg.Discord == nil || cfg.Discord.Digest == nil || cfg.Discord.Digest.WebhookURL == "" {
			fmt.Fprintln(os.Stderr, "Error: discord.digest.webhook_url not set in config (or pass --webhook)")
			os.Exit(1)
		}
		webhookURL = cfg.Discord.Digest.WebhookURL
	}

	categories := defaultDigestCategories
	if cfg != nil && cfg.Discord != nil && cfg.Discord.Digest != nil && len(cfg.Discord.Digest.Categories) > 0 {
		categories = cfg.Discord.Digest.Categories
	}

	// One asOf for every fetch, so the title link shows the same rankings.
	asOf := time.Now().UTC().Truncate(time.Second)
	asOfStr := asOf.Format(time.RFC3339)

	results := make(map[string]*domain.LeaderboardResponse, len(categories))
	for _, cat := range categories {
		var resp domain.LeaderboardResponse
		path := fmt.Sprintf("/api/stats/leaderboard?category=%s&period=%s&limit=%d&as_of=%s",
			cat, *period, *topN, asOfStr)
		if err := getJSON(path, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching %s: %v\n", cat, err)
			os.Exit(1)
		}
		results[cat] = &resp
	}

	// People click this link in Discord, so it uses the public URL when
	// one is configured.
	publicURL := strings.TrimSuffix(baseURL, "/")
	if cfg != nil && cfg.Tracker != nil && cfg.Tracker.Collector != nil && cfg.Tracker.Collector.PublicURL != "" {
		publicURL = strings.TrimSuffix(cfg.Tracker.Collector.PublicURL, "/")
	}
	footerURL := fmt.Sprintf("%s/leaderboard?period=%s&as_of=%s", publicURL, *period, asOfStr)
	staticDir := ""
	if cfg != nil {
		staticDir = cfg.Server.StaticDir
	}
	embed := renderDigestEmbed(results, categories, *topN, footerURL, publicURL, staticDir, time.Now())

	if *dryRun {
		out, _ := json.MarshalIndent(discord.WebhookPayload{Embeds: []discord.Embed{embed}}, "", "  ")
		fmt.Println(string(out))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := discord.PostWebhook(ctx, webhookURL, embed); err != nil {
		fmt.Fprintf(os.Stderr, "Error posting to Discord: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Posted digest (%d categories, period=%s, as_of=%s)\n", len(categories), *period, asOfStr)
}

// cmdUser lets each subcommand parse all its own flags, --config and
// --url included, since pflag mangles args a global FlagSet pre-parses.
func cmdUser(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: user subcommand required: add, remove, list, reset, admin, disable, enable\n")
		os.Exit(1)
	}
	subCmd := args[0]
	subArgs := args[1:]

	ctx := context.Background()

	var err error
	switch subCmd {
	case "add":
		err = cmdUserAdd(ctx, subArgs)
	case "remove":
		err = cmdUserRemove(ctx, subArgs)
	case "list":
		err = cmdUserList(ctx, subArgs)
	case "reset":
		err = cmdUserReset(ctx, subArgs)
	case "admin":
		err = cmdUserAdmin(ctx, subArgs)
	case "disable":
		err = cmdUserDisable(ctx, subArgs, true)
	case "enable":
		err = cmdUserDisable(ctx, subArgs, false)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown user command: %s (use: add, remove, list, reset, admin, disable, enable)\n", subCmd)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdUserAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	isAdmin := fs.Bool("admin", false, "create as admin user")
	playerIDFlag := fs.Int64("player-id", 0, "link to player ID")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user add [--admin] [--player-id N] <username>")
	}

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	username := remaining[0]
	var playerID *int64
	if *playerIDFlag != 0 {
		playerID = playerIDFlag
	}

	if _, err := store.GetUserByUsername(ctx, username); err == nil {
		return fmt.Errorf("user '%s' already exists", username)
	}

	if playerID != nil {
		claimed, err := store.IsPlayerClaimed(ctx, *playerID)
		if err != nil {
			return fmt.Errorf("couldn't check the player: %w", err)
		}
		if claimed {
			return fmt.Errorf("player %d is already linked to another user", *playerID)
		}
	}

	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("couldn't read the password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("couldn't read the password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("the passwords don't match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("couldn't hash the password: %w", err)
	}

	if err := store.CreateUser(ctx, username, hash, *isAdmin, playerID); err != nil {
		return fmt.Errorf("couldn't create the user: %w", err)
	}

	roleStr := "user"
	if *isAdmin {
		roleStr = "admin"
	}
	fmt.Printf("User '%s' created (role: %s)\n", username, roleStr)
	return nil
}

func cmdUserRemove(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user remove", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user remove <username>")
	}
	username := remaining[0]

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	if err := store.DeleteUser(ctx, username); err != nil {
		// The source_audit foreign key keeps users with history.
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return fmt.Errorf("user %q has audit history, so it can't be deleted; run trinity user disable %s instead", username, username)
		}
		return fmt.Errorf("couldn't remove the user: %w", err)
	}

	fmt.Printf("User '%s' removed\n", username)
	return nil
}

// cmdUserDisable disables or re-enables a user. A disabled account can't
// log in anywhere, and its row and audit history stay.
func cmdUserDisable(ctx context.Context, args []string, disabled bool) error {
	verb := "disable"
	if !disabled {
		verb = "enable"
	}
	fs := flag.NewFlagSet("user "+verb, flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user %s <username>", verb)
	}
	username := remaining[0]

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	if err := store.SetUserDisabled(ctx, username, disabled); err != nil {
		return fmt.Errorf("couldn't %s the user: %w", verb, err)
	}
	fmt.Printf("User '%s' %sd\n", username, verb)
	return nil
}

func cmdUserList(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user list", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	users, err := store.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("couldn't list users: %w", err)
	}

	if len(users) == 0 {
		fmt.Println(dim("No users configured"))
		return nil
	}

	usernameCol := column{header: "USERNAME"}
	roleCol := column{header: "ROLE"}
	playerIDCol := column{header: "PLAYER_ID", align: alignRight}
	pwdChangeCol := column{header: "PWD_CHANGE"}
	lastLoginCol := column{header: "LAST_LOGIN"}

	for _, user := range users {
		role := "user"
		if user.IsAdmin {
			role = cyan("admin")
		}
		if user.DisabledAt != nil {
			role = red("disabled")
		}
		playerID := dim("-")
		if user.PlayerID != nil {
			playerID = fmt.Sprintf("%d", *user.PlayerID)
		}
		pwdChange := "no"
		if user.PasswordChangeRequired {
			pwdChange = yellow("yes")
		}
		lastLogin := dim("never")
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04")
		}
		usernameCol.cells = append(usernameCol.cells, user.Username)
		roleCol.cells = append(roleCol.cells, role)
		playerIDCol.cells = append(playerIDCol.cells, playerID)
		pwdChangeCol.cells = append(pwdChangeCol.cells, pwdChange)
		lastLoginCol.cells = append(lastLoginCol.cells, lastLogin)
	}
	renderTable(os.Stdout, []column{usernameCol, roleCol, playerIDCol, pwdChangeCol, lastLoginCol})
	return nil
}

func cmdUserReset(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user reset", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user reset <username>")
	}
	username := remaining[0]

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	fmt.Print("Enter new password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("couldn't read the password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("couldn't read the password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("the passwords don't match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("couldn't hash the password: %w", err)
	}

	if err := store.ResetUserPassword(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("couldn't reset the password: %w", err)
	}

	fmt.Printf("Password reset for '%s' (user will be required to change it on next login)\n", username)
	return nil
}

func cmdUserAdmin(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("user admin", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the tracker")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user admin <username>")
	}
	username := remaining[0]

	store := openStoreForCLI(*configPath, *url)
	defer store.Close()

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	newAdminStatus := !user.IsAdmin
	if err := store.UpdateUserAdmin(ctx, user.ID, newAdminStatus); err != nil {
		return fmt.Errorf("couldn't update admin status: %w", err)
	}

	if newAdminStatus {
		fmt.Printf("User '%s' is now an admin\n", username)
	} else {
		fmt.Printf("User '%s' is no longer an admin\n", username)
	}
	return nil
}

// dropToServiceUser keeps a `sudo trinity ...` asset command from leaving
// root-owned files the service can't overwrite.
func dropToServiceUser(cfg *config.Config) {
	if err := dropPrivileges(serviceUser(cfg)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't drop privileges: %v\n", err)
	}
}

func loadAssetConfig(configPath string) *config.Config {
	cfg := loadCLIConfigFromFlags(configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't load the config\n")
		os.Exit(1)
	}
	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}
	dropToServiceUser(cfg)
	return cfg
}

func assetInputPath(cfg *config.Config, extras []string) string {
	if len(extras) > 0 {
		return extras[0]
	}
	return cfg.Server.Quake3Dir
}

func cmdLevelshots(args []string) {
	fs := flag.NewFlagSet("levelshots", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runLevelshots(cfg, assetInputPath(cfg, fs.Args()))
}

func runLevelshots(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "levelshots")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractLevelshotsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Levelshots: %d extracted\n", totalExtracted)
}

func extractLevelshotsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("couldn't open the pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		if !strings.HasPrefix(lowerName, "levelshots/") {
			continue
		}

		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".jpg" && ext != ".tga" {
			continue
		}

		mapName := strings.TrimSuffix(base, filepath.Ext(base))
		mapName = strings.ToLower(mapName)

		outputPath := filepath.Join(outputDir, mapName+".jpg")

		if err := extractLevelshot(f, outputPath, ext); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s: %v\n", mapName, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, mapName)
		extracted++
	}

	return extracted, nil
}

func extractLevelshot(f *zip.File, outputPath, ext string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	var img image.Image
	if ext == ".jpg" {
		img, err = jpeg.Decode(rc)
	} else {
		img, err = tga.Decode(rc)
	}
	if err != nil {
		return fmt.Errorf("couldn't decode %s: %w", ext, err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 640 || bounds.Dy() != 480 {
		dst := image.NewRGBA(image.Rect(0, 0, 640, 480))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}

	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: 90}); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

func pk3DisplayPath(pk3Path, basePath string) string {
	if rel, err := filepath.Rel(basePath, pk3Path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(pk3Path)
}

func getJSON(path string, target interface{}) error {
	url := baseURL + path
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("couldn't reach the tracker: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func formatTime(isoTime string) string {
	if idx := strings.Index(isoTime, "T"); idx != -1 {
		time := isoTime[idx+1:]
		if dotIdx := strings.Index(time, "."); dotIdx != -1 {
			time = time[:dotIdx]
		}
		if zIdx := strings.Index(time, "Z"); zIdx != -1 {
			time = time[:zIdx]
		}
		return time
	}
	return isoTime
}

// cmdArenas writes the maps.json the frontend fetches, with each map's long
// name from its worldspawn "message".
func cmdArenas(args []string) {
	fs := flag.NewFlagSet("arenas", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runArenas(cfg, assetInputPath(cfg, fs.Args()))
}

func runArenas(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	sources := assets.NewMapNameSources()
	for i, pk3Path := range pk3Files {
		if err := sources.Collect(pk3Path); err != nil {
			fmt.Fprintf(os.Stderr, "\n  Warning: %s: %v\n", pk3DisplayPath(pk3Path, inputPath), err)
			continue
		}
		fmt.Printf("\rScanned %d/%d pk3s", i+1, len(pk3Files))
	}
	fmt.Println()

	maps := sources.Resolve()

	outPath := filepath.Join(outputDir, "maps.json")
	out, err := json.MarshalIndent(maps, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: marshal maps.json: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: write %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("Maps: %d entries written to %s\n", len(maps), outPath)
	if filled := sources.ArenaFilled(); len(filled) > 0 {
		fmt.Printf("Filled %d longname(s) from .arena files: %s\n", len(filled), strings.Join(filled, ", "))
	}
	if unnamed := sources.Unnamed(); len(unnamed) > 0 {
		fmt.Printf("No longname for %d map(s): %s\n", len(unnamed), strings.Join(unnamed, ", "))
	}
}

func cmdPortraits(args []string) {
	fs := flag.NewFlagSet("portraits", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runPortraits(cfg, assetInputPath(cfg, fs.Args()))
}

func runPortraits(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "portraits")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	taOnly := make(map[string]bool)
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractPortraitsFromPk3(pk3Path, outputDir, displayPath, taOnly)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	// Built from the output directory, so icons from earlier runs stay listed.
	manifest, err := assets.BuildPortraitManifest(outputDir, taOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: build portrait manifest: %v\n", err)
		os.Exit(1)
	}
	manifestPath := filepath.Join(cfg.Server.StaticDir, "assets", "portraits.json")
	out, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: marshal portraits.json: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(manifestPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: write %s: %v\n", manifestPath, err)
		os.Exit(1)
	}

	fmt.Printf("Portraits: %d extracted, %d models in %s\n", totalExtracted, len(manifest), manifestPath)
}

// extractPortraitsFromPk3 records in taOnly whether each model's
// icon_default came from missionpack. Mod pk3s bundle team skins across
// both trees, so only the default icon, the one the picker shows, decides.
func extractPortraitsFromPk3(pk3Path, outputDir, displayPath string, taOnly map[string]bool) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("couldn't open the pk3: %w", err)
	}
	defer r.Close()

	isTA := strings.Split(filepath.ToSlash(displayPath), "/")[0] == "missionpack"
	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		if !strings.HasPrefix(lowerName, "models/players/") {
			continue
		}
		base := filepath.Base(f.Name)
		if !strings.HasPrefix(strings.ToLower(base), "icon_") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".tga" {
			continue
		}

		parts := strings.Split(f.Name, "/")
		if len(parts) < 4 {
			continue
		}

		var model string
		if strings.ToLower(parts[2]) == "heads" {
			// Team Arena heads: models/players/heads/<name>/icon_*.tga
			if len(parts) < 5 {
				continue
			}
			model = strings.ToLower(parts[3])
		} else {
			model = strings.ToLower(parts[2])
		}

		modelDir := filepath.Join(outputDir, model)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: couldn't create directory %s: %v\n", modelDir, err)
			continue
		}

		outputName := strings.TrimSuffix(strings.ToLower(base), ".tga") + ".png"
		outputPath := filepath.Join(modelDir, outputName)
		assetName := model + "/" + outputName

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, assetName)
		extracted++

		// The last write wins, like the icon_default.png left on disk.
		if outputName == "icon_default.png" {
			taOnly[model] = isTA
		}
	}

	return extracted, nil
}

func cmdHeads(args []string) {
	fs := flag.NewFlagSet("heads", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runHeads(cfg, assetInputPath(cfg, fs.Args()))
}

func runHeads(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "heads")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	fileIndex, err := assets.BuildFileIndex(pk3Files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: build file index: %v\n", err)
		os.Exit(1)
	}
	shaderIndex := map[string]assets.ShaderDef{}
	for _, pk3 := range pk3Files {
		_ = assets.IteratePk3(pk3, func(name string, open func() (io.ReadCloser, error)) error {
			if !strings.HasSuffix(strings.ToLower(name), ".shader") {
				return nil
			}
			rc, err := open()
			if err != nil {
				return nil
			}
			defer rc.Close()
			if defs, err := assets.ParseShaderScript(rc); err == nil {
				assets.MergeShaderDefs(shaderIndex, defs)
			}
			return nil
		})
	}

	total := 0
	for lower, srcPk3 := range fileIndex {
		if !strings.HasPrefix(lower, "models/players/") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(lower, "models/players/"), "/")
		var model, assetDir string
		switch {
		case len(parts) == 2 && parts[1] == "head.md3":
			model, assetDir = parts[0], "models/players/"+parts[0]+"/"
		// Team Arena heads live at heads/<name>/<name>.md3.
		case len(parts) == 3 && parts[0] == "heads" && parts[2] == parts[1]+".md3":
			model, assetDir = parts[1], "models/players/heads/"+parts[1]+"/"
		default:
			continue
		}
		if err := writeHeadBundle(model, assetDir, lower, srcPk3, fileIndex, shaderIndex, outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", lower, err)
			continue
		}
		total++
	}
	fmt.Printf("Heads: %d extracted in %s\n", total, outputDir)
}

// writeHeadBundle writes head.geo and head.json into outputDir/<model>/.
func writeHeadBundle(model, assetDir, md3Path, md3Pk3 string, fileIndex map[string]string, shaderIndex map[string]assets.ShaderDef, outputDir string) error {
	raw, err := assets.ReadFileFromPk3(md3Pk3, md3Path)
	if err != nil {
		return err
	}
	geo, err := assets.ParseMD3Geometry(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("parse md3: %w", err)
	}
	geoBytes, surfaces, vertCount, idxCount := assets.EncodeHeadGeometry(geo)

	modelDir := filepath.Join(outputDir, model)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return err
	}

	skins := map[string]map[string][]assets.StageManifest{}
	for lower := range fileIndex {
		if !strings.HasPrefix(lower, assetDir+"head_") || !strings.HasSuffix(lower, ".skin") {
			continue
		}
		skinName := strings.TrimSuffix(strings.TrimPrefix(lower, assetDir+"head_"), ".skin")
		raw, err := assets.ReadFileFromPk3(fileIndex[lower], lower)
		if err != nil {
			continue
		}
		entries, err := assets.ParseSkinMap(bytes.NewReader(raw))
		if err != nil {
			continue
		}
		surfStages := map[string][]assets.StageManifest{}
		for _, e := range entries {
			if stages := resolveSurfaceStages(e.Texture, fileIndex, shaderIndex, modelDir); len(stages) > 0 {
				surfStages[e.Surface] = stages
			}
		}
		if len(surfStages) > 0 {
			skins[skinName] = surfStages
		}
	}

	var headOffset [3]float32
	if cfgPath := assetDir + "animation.cfg"; fileIndex[cfgPath] != "" {
		if raw, err := assets.ReadFileFromPk3(fileIndex[cfgPath], cfgPath); err == nil {
			headOffset = assets.ParseHeadOffset(bytes.NewReader(raw))
		}
	}

	manifest := assets.HeadManifest{
		Bounds:      assets.HeadBounds{Mins: geo.Mins, Maxs: geo.Maxs},
		VertexCount: vertCount,
		IndexCount:  idxCount,
		Surfaces:    surfaces,
		Skins:       skins,
		HeadOffset:  headOffset,
	}
	if err := os.WriteFile(filepath.Join(modelDir, "head.geo"), geoBytes, 0644); err != nil {
		return err
	}
	mj, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modelDir, "head.json"), mj, 0644)
}

// resolveSurfaceStages uses a matching shader's stages, or else one opaque
// stage from the texture file. Every texture it touches becomes a PNG in
// the bundle.
func resolveSurfaceStages(texRef string, fileIndex map[string]string, shaderIndex map[string]assets.ShaderDef, modelDir string) []assets.StageManifest {
	shaderName := strings.ToLower(strings.TrimSuffix(texRef, filepath.Ext(texRef)))
	convert := func(ref string) (string, bool) {
		resolved, ok := assets.ResolveTexture(ref, fileIndex)
		if !ok {
			return "", false
		}
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(resolved), filepath.Ext(resolved)))
		out := stem + ".png"
		if err := convertIndexedTextureToPng(resolved, fileIndex[resolved], filepath.Join(modelDir, out)); err != nil {
			return "", false
		}
		return out, true
	}

	if def, ok := shaderIndex[shaderName]; ok && len(def.Stages) > 0 {
		var out []assets.StageManifest
		for _, s := range def.Stages {
			var pngs []string
			for _, m := range s.Maps {
				if strings.EqualFold(m, "$whiteimage") {
					pngs = append(pngs, "$white")
					continue
				}
				if strings.HasPrefix(m, "$") {
					continue
				}
				if png, ok := convert(m); ok {
					pngs = append(pngs, png)
				}
			}
			if len(pngs) == 0 {
				continue
			}
			out = append(out, assets.StageManifest{
				Maps: pngs, AnimFreq: s.AnimFreq, Blend: s.Blend, AlphaFunc: s.AlphaFunc,
				TcGen: s.TcGen, TcMod: s.TcMod, RgbGen: s.RgbGen, Clamp: s.Clamp,
				DoubleSided: def.DoubleSided, Deform: def.Deform,
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	if png, ok := convert(texRef); ok {
		return []assets.StageManifest{{Maps: []string{png}, Blend: "opaque", RgbGen: "lightingDiffuse"}}
	}
	return nil
}

func convertIndexedTextureToPng(virtualPath, srcPk3, outPath string) error {
	raw, err := assets.ReadFileFromPk3(srcPk3, virtualPath)
	if err != nil {
		return err
	}
	var img image.Image
	switch strings.ToLower(filepath.Ext(virtualPath)) {
	case ".tga":
		img, err = tga.Decode(bytes.NewReader(raw))
	case ".jpg", ".jpeg":
		img, err = jpeg.Decode(bytes.NewReader(raw))
	case ".png":
		img, err = png.Decode(bytes.NewReader(raw))
	default:
		return fmt.Errorf("unsupported texture ext: %s", virtualPath)
	}
	if err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, img)
}

func cmdMedals(args []string) {
	fs := flag.NewFlagSet("medals", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runMedals(cfg, assetInputPath(cfg, fs.Args()))
}

func runMedals(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "medals")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractMedalsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Medals: %d extracted\n", totalExtracted)
}

func extractMedalsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("couldn't open the pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		isMedalPath := (strings.HasPrefix(lowerName, "menu/medals/") || strings.HasPrefix(lowerName, "ui/assets/")) &&
			strings.HasPrefix(base, "medal_") &&
			strings.HasSuffix(base, ".tga")

		if !isMedalPath {
			continue
		}

		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

func cmdSkills(args []string) {
	fs := flag.NewFlagSet("skills", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runSkills(cfg, assetInputPath(cfg, fs.Args()))
}

func runSkills(cfg *config.Config, inputPath string) {
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "skills")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory: %v\n", err)
		os.Exit(1)
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractSkillsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Skills: %d extracted\n", totalExtracted)
}

func extractSkillsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("couldn't open the pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		// menu/art/skill[1-5].tga
		if !strings.HasPrefix(lowerName, "menu/art/") {
			continue
		}
		if !strings.HasPrefix(base, "skill") || !strings.HasSuffix(base, ".tga") {
			continue
		}
		numPart := strings.TrimPrefix(base, "skill")
		numPart = strings.TrimSuffix(numPart, ".tga")
		if len(numPart) != 1 || numPart[0] < '1' || numPart[0] > '5' {
			continue
		}

		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

// flagTeamColors tints the source's white silhouette per team. "neutral"
// keeps it white for the One Flag CTF flag.
var flagTeamColors = map[string]color.NRGBA{
	"red":     {R: 0xFF, G: 0x44, B: 0x44, A: 0xFF},
	"blue":    {R: 0x66, G: 0x88, B: 0xFF, A: 0xFF},
	"neutral": {R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
}

// flagSourceStates are the missionpack statusbar images FlagIcon.tsx
// shows, each written once per team color.
var flagSourceStates = []string{"flag_in_base", "flag_capture", "flag_missing"}

// cmdObjectives writes team-tinted objective icons:
//   - ui/assets/statusbar/flag_*.tga → <static>/assets/flags/
//   - icons/skull_{red,blue}.tga     → <static>/assets/icons/
//
// Both use flagTeamColors so skulls and flags match.
func cmdObjectives(args []string) {
	fs := flag.NewFlagSet("objectives", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	runObjectives(cfg, assetInputPath(cfg, fs.Args()))
}

func runObjectives(cfg *config.Config, inputPath string) {
	flagsDir := filepath.Join(cfg.Server.StaticDir, "assets", "flags")
	iconsDir := filepath.Join(cfg.Server.StaticDir, "assets", "icons")
	for _, d := range []string{flagsDir, iconsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: couldn't create the output directory %s: %v\n", d, err)
			os.Exit(1)
		}
	}

	pk3Files := collectPk3FilesOrdered(inputPath)
	if len(pk3Files) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no pk3 files found in %s\n", inputPath)
		os.Exit(1)
	}

	var totalExtracted int
	for _, pk3Path := range pk3Files {
		displayPath := pk3DisplayPath(pk3Path, inputPath)
		n, err := extractObjectivesFromPk3(pk3Path, flagsDir, iconsDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Objectives: %d extracted\n", totalExtracted)
}

func extractObjectivesFromPk3(pk3Path, flagsDir, iconsDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("couldn't open the pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))
		if !strings.HasSuffix(base, ".tga") {
			continue
		}

		if strings.HasPrefix(lowerName, "ui/assets/statusbar/") {
			stem := strings.TrimSuffix(base, ".tga")
			matched := false
			for _, want := range flagSourceStates {
				if stem == want {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			for _, team := range []string{"red", "blue", "neutral"} {
				outputName := stem + "_" + team + ".png"
				outputPath := filepath.Join(flagsDir, outputName)
				if err := extractFlagToPng(f, outputPath, flagTeamColors[team], 128); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s as %s: %v\n", f.Name, team, err)
					continue
				}
				fmt.Printf("  %s: %s\n", displayPath, outputName)
				extracted++
			}
			continue
		}

		// The missionpack skulls are too saturated, so they're retinted.
		if strings.HasPrefix(lowerName, "icons/") {
			var team string
			switch base {
			case "skull_red.tga":
				team = "red"
			case "skull_blue.tga":
				team = "blue"
			default:
				continue
			}
			outputName := strings.TrimSuffix(base, ".tga") + ".png"
			outputPath := filepath.Join(iconsDir, outputName)
			if err := extractSkullToPng(f, outputPath, flagTeamColors[team], 128); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: couldn't extract %s: %v\n", f.Name, err)
				continue
			}
			fmt.Printf("  %s: %s\n", displayPath, outputName)
			extracted++
			continue
		}
	}

	return extracted, nil
}

// extractSkullToPng keeps the source alpha and takes intensity from
// max(R,G,B), so a fully red or fully blue skull still reaches full
// brightness under the tint.
func extractSkullToPng(f *zip.File, outputPath string, tint color.NRGBA, targetSize int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	src, err := tga.Decode(rc)
	if err != nil {
		return fmt.Errorf("decode TGA: %w", err)
	}

	bounds := src.Bounds()
	tinted := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			maxChan := r
			if g > maxChan {
				maxChan = g
			}
			if b > maxChan {
				maxChan = b
			}
			intensity := maxChan >> 8 // 0-255
			tinted.SetNRGBA(x, y, color.NRGBA{
				R: uint8(uint32(tint.R) * intensity / 255),
				G: uint8(uint32(tint.G) * intensity / 255),
				B: uint8(uint32(tint.B) * intensity / 255),
				A: uint8(a >> 8),
			})
		}
	}

	dst := image.NewNRGBA(image.Rect(0, 0, targetSize, targetSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), tinted, bounds, draw.Over, nil)

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, dst)
}

// extractFlagToPng reads the red channel as luminance: the source is
// opaque, with the silhouette drawn as a black-to-white ramp. White
// becomes the team color and black stays black.
func extractFlagToPng(f *zip.File, outputPath string, tint color.NRGBA, targetSize int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	src, err := tga.Decode(rc)
	if err != nil {
		return fmt.Errorf("decode TGA: %w", err)
	}

	bounds := src.Bounds()
	tinted := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, _, _, _ := src.At(x, y).RGBA()
			lum := uint32(r >> 8)
			tinted.SetNRGBA(x, y, color.NRGBA{
				R: uint8(uint32(tint.R) * lum / 255),
				G: uint8(uint32(tint.G) * lum / 255),
				B: uint8(uint32(tint.B) * lum / 255),
				A: 255,
			})
		}
	}

	dst := image.NewNRGBA(image.Rect(0, 0, targetSize, targetSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), tinted, bounds, draw.Over, nil)

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, dst)
}

// cmdMapsync regenerates everything derived from the installed maps:
// levelshots, maps.json and the demo pk3s.
func cmdMapsync(args []string) {
	fs := flag.NewFlagSet("mapsync", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	inputPath := assetInputPath(cfg, fs.Args())

	fmt.Println("=== Extracting levelshots ===")
	runLevelshots(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting map names ===")
	runArenas(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Baking demo pk3s ===")
	runDemobake(cfg, inputPath, "")
	fmt.Println()

	fmt.Println("=== Map sync complete ===")
}

func cmdAssets(args []string) {
	fs := flag.NewFlagSet("assets", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadAssetConfig(*configPath)
	inputPath := assetInputPath(cfg, fs.Args())

	fmt.Println("=== Extracting levelshots ===")
	runLevelshots(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting portraits ===")
	runPortraits(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting heads ===")
	runHeads(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting medals ===")
	runMedals(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting skills ===")
	runSkills(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting objectives ===")
	runObjectives(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== Extracting map names ===")
	runArenas(cfg, inputPath)
	fmt.Println()

	fmt.Println("=== All asset extraction complete ===")
}

func cmdDemobake(args []string) {
	fs := flag.NewFlagSet("demobake", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	output := fs.String("output", "", "output directory (default: {static_dir}/pk3s/)")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: couldn't load the config\n")
		os.Exit(1)
	}
	dropToServiceUser(cfg)

	runDemobake(cfg, assetInputPath(cfg, fs.Args()), *output)
}

// runDemobake writes to {static_dir}/demopk3s unless outputOverride is set.
func runDemobake(cfg *config.Config, quake3Dir, outputOverride string) {
	outputDir := outputOverride
	if outputDir == "" {
		if cfg.Server.StaticDir == "" {
			fmt.Fprintf(os.Stderr, "Error: static_dir not configured and --output not specified\n")
			os.Exit(1)
		}
		outputDir = filepath.Join(cfg.Server.StaticDir, "demopk3s")
	}

	if err := assets.BuildBaseline(quake3Dir, outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Demobake complete")
}

func cmdUnpackmaps(args []string) {
	fs := flag.NewFlagSet("unpackmaps", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	output := fs.String("output", "", "output directory (default: ./<firstsource>-maps/)")
	quake3Dir := fs.String("quake3-dir", "", "Quake III directory holding the stock paks (default: config, else /usr/lib/quake3)")
	prefix := fs.String("prefix", "", "output filename prefix, e.g. ql-")
	game := fs.String("game", "baseq3", "target game whose stock assets the client has: baseq3 or missionpack")
	dryRun := fs.Bool("dry-run", false, "print the split report without writing pk3s")
	fs.Parse(args)

	sources := fs.Args()
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one source pk3 is required")
		os.Exit(1)
	}

	var baselineGames []string
	switch *game {
	case "baseq3":
		baselineGames = []string{"baseq3"}
	case "missionpack":
		baselineGames = []string{"baseq3", "missionpack"}
	default:
		fmt.Fprintf(os.Stderr, "Error: --game must be baseq3 or missionpack, got %q\n", *game)
		os.Exit(1)
	}

	q3 := *quake3Dir
	if q3 == "" {
		if cfg := loadCLIConfigFromFlags(*configPath, ""); cfg != nil && cfg.Server.Quake3Dir != "" {
			q3 = cfg.Server.Quake3Dir
		} else {
			q3 = "/usr/lib/quake3"
		}
	}

	out := *output
	if out == "" {
		base := strings.TrimSuffix(filepath.Base(sources[0]), filepath.Ext(sources[0]))
		out = base + "-maps"
	}

	report, err := assets.SplitMapPack(assets.SplitOptions{
		Sources:       sources,
		Quake3Dir:     q3,
		BaselineGames: baselineGames,
		OutputDir:     out,
		Prefix:        *prefix,
		DryRun:        *dryRun,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	printSplitReport(report, *dryRun, out)
}

func printSplitReport(report *assets.SplitReport, dryRun bool, outputDir string) {
	mb := func(b int64) string { return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024)) }
	verb := "Wrote"
	if dryRun {
		verb = "Would write"
	}
	fmt.Printf("%s %d map pk3s to %s (%s)\n", verb, len(report.Maps), outputDir, mb(report.TotalBytes))
	fmt.Printf("Dropped %d unreferenced files (%s)\n", report.DeadFiles, mb(report.DeadBytes))
	if len(report.Failed) > 0 {
		fmt.Printf("Failed (%d): %s\n", len(report.Failed), strings.Join(report.Failed, ", "))
	}
	var withMissing []assets.MapReport
	for _, m := range report.Maps {
		if len(m.MissingAudio) > 0 {
			withMissing = append(withMissing, m)
		}
	}
	if len(withMissing) > 0 {
		fmt.Printf("%d map(s) reference audio missing from pack and stock (engine will play the hit sound):\n", len(withMissing))
		for _, m := range withMissing {
			fmt.Printf("  %s: %s\n", m.Name, strings.Join(m.MissingAudio, ", "))
		}
	}
}

// dropPrivileges does nothing unless running as root.
func dropPrivileges(username string) error {
	if os.Getuid() != 0 {
		return nil
	}
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("looking up user %s: %w", username, err)
	}
	gid, _ := strconv.Atoi(u.Gid)
	uid, _ := strconv.Atoi(u.Uid)
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("setgid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("setuid: %w", err)
	}
	return nil
}

func serviceUser(cfg *config.Config) string {
	if cfg != nil && cfg.Server.ServiceUser != "" {
		return cfg.Server.ServiceUser
	}
	return "quake"
}

func useSystemd(cfg *config.Config) bool {
	if cfg != nil && cfg.Server.UseSystemd != nil {
		return *cfg.Server.UseSystemd
	}
	return detectSystemd()
}

func detectSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func systemctlRun(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func systemctlIsActive(unit string) string {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	if err != nil {
		return "not-found"
	}
	return strings.TrimSpace(string(out))
}

func writeEnvFile(path string, port int, game string) error {
	opts := fmt.Sprintf("+set net_port %d", port)
	if game != "" && game != "baseq3" {
		opts += fmt.Sprintf(" +set fs_game %s", game)
	}
	content := fmt.Sprintf("SERVER_OPTS=%s\n", opts)
	return os.WriteFile(path, []byte(content), 0644)
}

func readEnvFile(path string) (port int, game string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "SERVER_OPTS=") {
			continue
		}
		opts := strings.TrimPrefix(line, "SERVER_OPTS=")
		parts := strings.Fields(opts)
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "+set" && i+2 < len(parts) {
				switch parts[i+1] {
				case "net_port":
					port, _ = strconv.Atoi(parts[i+2])
				case "fs_game":
					game = parts[i+2]
				}
			}
		}
		break
	}
	if game == "" {
		game = "baseq3"
	}
	return port, game, scanner.Err()
}

// cmdInit is the interactive installer. It won't run over an existing
// config, so re-running it means deleting the config by hand first.
func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	noSystemd := fs.Bool("no-systemd", false, "skip systemd unit installation")
	dryRun := fs.Bool("dry-run", false, "print what would happen instead of doing it (no root required)")
	// Hidden: the supported install is a collector that joins trinity.run.
	// docs/distributed-deployment.md covers running a hub.
	allowHub := fs.Bool("allow-hub", false, "")
	_ = fs.MarkHidden("allow-hub")
	skipCert := fs.Bool("skip-cert", false, "install nginx, render its config and reload it, but skip certbot (for a caller that already staged /etc/letsencrypt/live/<host>/, as in a hub migration)")
	skipFirewall := fs.Bool("skip-firewall", false, "leave ufw and firewalld alone, for hosts whose firewall is managed another way (cloud dashboard, nftables, config management)")
	skipNginx := fs.Bool("skip-nginx", false, "do not install or configure nginx, for hosts running their own reverse proxy (Caddy, Traefik and so on). Implies --skip-cert.")
	skipLogrotate := fs.Bool("skip-logrotate", false, "do not write /etc/logrotate.d/quake3, for hosts that rotate logs another way (fluent-bit, vector, journald only)")
	configPathFlag := fs.String("config", "/etc/trinity/config.yml", "destination config path")
	fs.Parse(args)

	if *skipNginx {
		*skipCert = true
	}

	// A dry run changes nothing, so it needs no root and may run over an
	// existing config.
	if !*dryRun && os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "Error: trinity init must be run as root (or use --dry-run).")
		os.Exit(1)
	}

	configPath := *configPathFlag
	if !*dryRun {
		if _, err := os.Stat(configPath); err == nil {
			fmt.Fprintf(os.Stderr, "Trinity is already initialized (%s exists).\n", configPath)
			fmt.Fprintln(os.Stderr, "To re-init, remove the config file first:")
			fmt.Fprintf(os.Stderr, "  sudo rm %s\n", configPath)
			os.Exit(1)
		}
	}

	if !setup.IsTTY() && !*dryRun {
		fmt.Fprintln(os.Stderr, "Error: trinity init is interactive. Run it from a terminal.")
		fmt.Fprintln(os.Stderr, "For unattended or scripted installs, write /etc/trinity/config.yml directly")
		fmt.Fprintln(os.Stderr, "and use `trinity server add` to add servers afterward.")
		os.Exit(1)
	}

	useSd := !*noSystemd && detectSystemd()
	if *noSystemd {
		fmt.Fprintln(os.Stderr, "Note: --no-systemd passed; will not install or enable units.")
	} else if !useSd {
		fmt.Fprintln(os.Stderr, "Note: systemd not detected; will not install units.")
	}

	prompter := setup.NewStdPrompter()
	answers, err := setup.RunWizard(prompter, os.Stderr, setup.WizardOptions{
		AllowHub:      *allowHub,
		SkipCert:      *skipCert,
		SkipFirewall:  *skipFirewall,
		SkipNginx:     *skipNginx,
		SkipLogrotate: *skipLogrotate,
	})
	if errors.Is(err, setup.ErrMissingPrereqs) {
		// The wizard already explained; install.sh still needs the failure.
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Wizard failed: %v\n", err)
		os.Exit(1)
	}

	if err := answers.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid answers: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Review:")
	fmt.Fprintln(os.Stderr, "  Mode:        ", answers.Mode)
	fmt.Fprintln(os.Stderr, "  User:        ", answers.ServiceUser)
	fmt.Fprintln(os.Stderr, "  Listen:      ", fmt.Sprintf("%s:%d", answers.ListenAddr, answers.HTTPPort))
	if answers.HasHubFields() {
		fmt.Fprintln(os.Stderr, "  Database:    ", answers.DatabasePath)
		fmt.Fprintln(os.Stderr, "  Web assets:  ", answers.StaticDir)
	}
	if answers.RunsLocalServers() {
		fmt.Fprintln(os.Stderr, "  Game dir:    ", answers.Quake3Dir)
		if answers.InstallEngine {
			fmt.Fprintln(os.Stderr, "  Engine:       latest from github.com/ernie/trinity-engine")
		}
	}
	if answers.Mode == setup.ModeCollector {
		fmt.Fprintln(os.Stderr, "  Hub:         ", answers.HubHost)
		fmt.Fprintln(os.Stderr, "  Public URL:  ", answers.PublicURL)
		fmt.Fprintln(os.Stderr, "  Source ID:   ", answers.SourceID)
		fmt.Fprintln(os.Stderr, "  Creds file:  ", answers.CredsFile)
	}
	for _, s := range answers.Servers {
		fmt.Fprintf(os.Stderr, "  Server:       %s (%s) :%d\n", s.Key, s.Gametype.Label(), s.Port)
	}
	fmt.Fprintln(os.Stderr)

	prompt := "Apply this configuration?"
	if *dryRun {
		prompt = "Show the plan?"
	}
	confirm, err := prompter.YesNo(prompt, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't read your answer: %v\n", err)
		os.Exit(1)
	}
	if !confirm {
		fmt.Fprintln(os.Stderr, "Aborted; nothing was written.")
		os.Exit(1)
	}

	err = setup.Apply(answers, setup.ApplyOptions{
		ConfigPath: configPath,
		UseSystemd: useSd,
		DryRun:     *dryRun,
		Out:        os.Stderr,
	})
	// Apply is done with install.sh's staging dir whether or not it
	// succeeded; a dry run keeps it for the real run.
	if !*dryRun {
		setup.CleanupStage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Apply failed: %v\n", err)
		os.Exit(1)
	}

	if *dryRun {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Dry run complete; nothing changed. Run it again as root without --dry-run to apply.")
		return
	}

	// Places the paks and starts services; whatever it can't do falls
	// to the next steps printed below.
	var pakResult setup.PakStepResult
	if answers.RunsLocalServers() {
		cwd, _ := os.Getwd()
		pakResult = setup.RunPakStep(setup.PakStepOptions{
			Quake3Dir:   answers.Quake3Dir,
			ServiceUser: answers.ServiceUser,
			UseSystemd:  useSd,
			HubHost:     answers.HubHost,
			StaticDir:   answers.StaticDir,
			Cwd:         cwd,
			Out:         os.Stderr,
			Prompter:    prompter,
		})
	}

	needStart := useSd && !pakResult.Started
	needBake := answers.Mode == setup.ModeCollector && !pakResult.Baked

	fmt.Fprintln(os.Stderr)
	if !needStart && !needBake {
		if pakResult.Started {
			fmt.Fprintln(os.Stderr, "All set. Trinity and your Quake III servers are running.")
		} else {
			fmt.Fprintln(os.Stderr, "All set.")
		}
		if answers.HubHost != "" {
			fmt.Fprintf(os.Stderr, "Find your source on the hub: https://%s/\n", answers.HubHost)
		}
		fmt.Fprintln(os.Stderr, "Now go play some Quake III.")
		return
	}

	fmt.Fprintln(os.Stderr, "Done. Next steps:")
	step := 1
	if needStart {
		fmt.Fprintf(os.Stderr, "  %d. Start: sudo systemctl start trinity.service", step)
		if answers.RunsLocalServers() {
			fmt.Fprint(os.Stderr, " quake3-servers.target")
		}
		fmt.Fprintln(os.Stderr)
		step++
	}
	if needBake {
		fmt.Fprintf(os.Stderr, "  %d. Required: generate the levelshot images and demo-playback pk3s the\n", step)
		fmt.Fprintln(os.Stderr, "     hub serves to web viewers. Run these again when you add new maps.")
		// The target user's PATH may lack /usr/local/bin.
		fmt.Fprintf(os.Stderr, "       sudo -u %s /usr/local/bin/trinity levelshots\n", answers.ServiceUser)
		fmt.Fprintf(os.Stderr, "       sudo -u %s /usr/local/bin/trinity demobake\n", answers.ServiceUser)
		step++
	}
}

func cmdServer(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: server subcommand required: list, add, remove\n")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdServerList(args[1:])
	case "add":
		cmdServerAdd(args[1:])
	case "remove":
		cmdServerRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown server command: %s (use: list, add, remove)\n", args[0])
		os.Exit(1)
	}
}

// cmdServerList shows configured game servers with optional systemd status
func cmdServerList(args []string) {
	fs := flag.NewFlagSet("server list", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	colorMode := addColorFlag(fs)
	fs.Parse(args)
	applyColorMode(*colorMode)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Q3Servers) == 0 {
		fmt.Println(dim("No servers configured"))
		return
	}

	useSd := useSystemd(cfg)
	configDir := filepath.Dir(*configPath)

	nameCol := column{header: "NAME"}
	portCol := column{header: "PORT", align: alignRight}
	gameCol := column{header: "GAME"}
	serviceCol := column{header: "SERVICE"}
	statusCol := column{header: "STATUS"}

	for _, srv := range cfg.Q3Servers {
		port := ""
		if parts := strings.SplitN(srv.Address, ":", 2); len(parts) == 2 {
			port = parts[1]
		}

		serverName := strings.ToLower(srv.Key)
		game := "baseq3"
		envPath := filepath.Join(configDir, serverName+".env")
		if envPort, envGame, err := readEnvFile(envPath); err == nil {
			game = envGame
			if port == "" {
				port = strconv.Itoa(envPort)
			}
		}

		nameCol.cells = append(nameCol.cells, srv.Key)
		portCol.cells = append(portCol.cells, port)
		gameCol.cells = append(gameCol.cells, game)

		if useSd {
			unit := "quake3-server@" + serverName
			status := systemctlIsActive(unit)
			coloredStatus := status
			switch strings.TrimSpace(status) {
			case "active":
				coloredStatus = green(status)
			case "failed", "inactive":
				coloredStatus = red(status)
			default:
				coloredStatus = dim(status)
			}
			serviceCol.cells = append(serviceCol.cells, unit)
			statusCol.cells = append(statusCol.cells, coloredStatus)
		}
	}
	if useSd {
		renderTable(os.Stdout, []column{nameCol, portCol, gameCol, serviceCol, statusCol})
	} else {
		renderTable(os.Stdout, []column{nameCol, portCol, gameCol})
	}
}

// nextAvailablePort returns the lowest port from 27960 up that neither the
// config nor an .env file uses.
func nextAvailablePort(cfg *config.Config, configDir string) int {
	used := make(map[int]bool)

	for _, srv := range cfg.Q3Servers {
		if parts := strings.SplitN(srv.Address, ":", 2); len(parts) == 2 {
			if p, err := strconv.Atoi(parts[1]); err == nil {
				used[p] = true
			}
		}
	}

	entries, _ := os.ReadDir(configDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".env") {
			if p, _, err := readEnvFile(filepath.Join(configDir, e.Name())); err == nil && p > 0 {
				used[p] = true
			}
		}
	}

	for port := 27960; ; port++ {
		if !used[port] {
			return port
		}
	}
}

// cmdServerAdd prompts on a TTY when given no name, and otherwise runs
// from flags. It writes the server's .env file, writes the game type's
// shared <stem>.cfg and rotation.<stem> if they're missing, and enables
// the systemd instance.
func cmdServerAdd(args []string) {
	fs := flag.NewFlagSet("server add", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	port := fs.Int("port", 0, "server port (default: next available)")
	gametypeFlag := fs.String("gametype", "", "gametype: ffa, tournament, tdm, ctf, oneflag, overload, harvester")
	taFlag := fs.Bool("ta", false, "run under fs_game=missionpack (Team Arena weapons and maps); applies to tdm and ctf only")
	rconPassword := fs.String("rcon-password", "", "RCON password (default: generate)")
	logPath := fs.String("log-path", "", "log file path")
	allowHubAdminRcon := fs.Bool("allow-hub-admin-rcon", false, "allow hub admins to RCON this server in your absence (default: false)")
	fs.Parse(args)

	remaining := fs.Args()
	interactive := len(remaining) == 0 && setup.IsTTY()
	if len(remaining) < 1 && !interactive {
		fmt.Fprintln(os.Stderr, "Usage: trinity server add [<key>] [--gametype X] [--port N] [--rcon-password P] [--log-path P]")
		fmt.Fprintln(os.Stderr, "       (or run with no args on a TTY for an interactive prompt)")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	configDir := filepath.Dir(*configPath)
	sysUser := serviceUser(cfg)
	useSd := useSystemd(cfg)

	answers := answersFromConfig(cfg)
	var s setup.ServerAnswers
	if interactive {
		fmt.Fprintln(os.Stderr, "Add a Quake III server.")
		s, err = setup.PromptServer(setup.NewStdPrompter(), answers, len(cfg.Q3Servers))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Wizard failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		s, err = serverAnswersFromFlags(remaining[0], *gametypeFlag, *taFlag, *port, *rconPassword, *logPath, *allowHubAdminRcon, cfg, configDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// The wizard accepts any key, so uniqueness is checked here.
	for _, existing := range cfg.Q3Servers {
		if strings.EqualFold(existing.Key, s.Key) {
			fmt.Fprintf(os.Stderr, "Error: server '%s' already exists\n", s.Key)
			os.Exit(1)
		}
	}

	uid, gid := uidGid(sysUser)

	if useSd && os.Getuid() == 0 {
		unit := "quake3-server@" + s.Key
		if err := systemctlRun("enable", unit); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: systemctl enable %s failed: %v\n", unit, err)
		} else {
			fmt.Println("Enabled:", unit)
		}
	}

	stem := setup.Stem(s.Gametype, s.UseMissionpack)

	envPath := filepath.Join(configDir, s.Key+".env")
	opts := fmt.Sprintf("+set net_port %d", s.Port)
	if s.RunsMissionpack() {
		opts += " +set fs_game missionpack"
	}
	opts += " +exec " + stem + ".cfg"
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf("SERVER_OPTS=%s\n", opts)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing env file: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Wrote:", envPath)

	// Servers of the same game type share these files.
	if cfg.Server.Quake3Dir != "" {
		cfgPath := filepath.Join(cfg.Server.Quake3Dir, s.ModFolder(), stem+".cfg")
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Printf("  Note: %s already exists, so it was left alone and its rconpassword kept.\n", cfgPath)
		} else if body, rerr := setup.RenderServerCfg(s.Gametype, s.UseMissionpack, s.RconPassword); rerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cfg template render: %v\n", rerr)
		} else if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err == nil {
			// It holds the rconpassword.
			if err := os.WriteFile(cfgPath, []byte(body), 0640); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", cfgPath, err)
			} else {
				_ = os.Chown(cfgPath, 0, gid)
				fmt.Println("Wrote:", cfgPath)
			}
		}

		rotPath := filepath.Join(cfg.Server.Quake3Dir, s.ModFolder(), "rotation."+stem)
		if _, err := os.Stat(rotPath); err == nil {
			// rotation already in place; nothing to do.
		} else if body, rerr := setup.RenderRotation(s.Gametype, s.UseMissionpack); rerr != nil {
			fmt.Fprintf(os.Stderr, "Warning: rotation render: %v\n", rerr)
		} else if err := os.WriteFile(rotPath, body, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", rotPath, err)
		} else {
			_ = os.Chown(rotPath, uid, gid)
			fmt.Println("Wrote:", rotPath)
		}
	}

	// Saved last: stray files without a config entry are harmless.
	server := config.Q3Server{
		Key:               s.Key,
		Address:           s.Address,
		LogPath:           s.LogPath,
		RconPassword:      s.RconPassword,
		AllowHubAdminRcon: s.AllowHubAdminRcon,
	}
	config.AddServer(cfg, server)
	if err := config.Save(*configPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Updated:", *configPath)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Restart trinity:    sudo systemctl restart trinity")
	if useSd {
		fmt.Printf("  2. Start this server:  sudo systemctl start quake3-server@%s\n", s.Key)
	}
}

// answersFromConfig gives PromptServer what it needs for its defaults.
func answersFromConfig(cfg *config.Config) *setup.Answers {
	a := &setup.Answers{Quake3Dir: cfg.Server.Quake3Dir}
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil {
		a.PublicURL = cfg.Tracker.Collector.PublicURL
	}
	// suggestKey reads only the keys.
	for _, srv := range cfg.Q3Servers {
		a.Servers = append(a.Servers, setup.ServerAnswers{Key: srv.Key})
	}
	return a
}

// serverAnswersFromFlags uses the same defaults PromptServer suggests.
func serverAnswersFromFlags(name, gametypeName string, ta bool, port int, rcon, logPath string, allowHubAdminRcon bool, cfg *config.Config, configDir string) (setup.ServerAnswers, error) {
	s := setup.ServerAnswers{Key: strings.ToLower(name), AllowHubAdminRcon: allowHubAdminRcon}
	gt, err := parseGametype(gametypeName)
	if err != nil {
		return s, err
	}
	s.Gametype = gt
	if ta && (gt == setup.GametypeTDM || gt == setup.GametypeCTF) {
		s.UseMissionpack = true
	}
	s.Port = port
	if s.Port == 0 {
		s.Port = nextAvailablePort(cfg, configDir)
	}
	host := "127.0.0.1"
	if cfg.Tracker != nil && cfg.Tracker.Collector != nil && cfg.Tracker.Collector.PublicURL != "" {
		if h := setup.HostFromURL(cfg.Tracker.Collector.PublicURL); h != "" {
			host = h
		}
	}
	s.Address = fmt.Sprintf("%s:%d", host, s.Port)
	s.RconPassword = rcon
	if s.RconPassword == "" {
		s.RconPassword = setup.GenerateRCONPassword()
	}
	s.LogPath = logPath
	if s.LogPath == "" {
		s.LogPath = filepath.Join("/var/log/quake3", strings.ToLower(s.Key)+".log")
	}
	return s, nil
}

// parseGametype defaults an empty value to FFA.
func parseGametype(name string) (setup.Gametype, error) {
	switch strings.ToLower(name) {
	case "", "ffa":
		return setup.GametypeFFA, nil
	case "tournament", "1v1":
		return setup.GametypeTournament, nil
	case "tdm":
		return setup.GametypeTDM, nil
	case "ctf":
		return setup.GametypeCTF, nil
	case "oneflag":
		return setup.GametypeOneFlag, nil
	case "overload":
		return setup.GametypeOverload, nil
	case "harvester":
		return setup.GametypeHarvester, nil
	}
	return 0, fmt.Errorf("unknown gametype %q (try ffa, tournament, tdm, ctf, oneflag, overload, harvester)", name)
}

// uidGid returns 0, 0 when the lookup fails, so files stay owned by root.
func uidGid(name string) (int, int) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, 0
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid
}

func cmdServerRemove(args []string) {
	fs := flag.NewFlagSet("server remove", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: trinity server remove <name>\n")
		os.Exit(1)
	}

	name := strings.ToLower(remaining[0])

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	sysUser := serviceUser(cfg)
	useSd := useSystemd(cfg)
	configDir := filepath.Dir(*configPath)

	if useSd && os.Getuid() == 0 {
		unit := "quake3-server@" + name
		fmt.Printf("Stopping %s…\n", unit)
		systemctlRun("stop", unit)
		fmt.Printf("Disabling %s…\n", unit)
		systemctlRun("disable", unit)
	}

	if err := dropPrivileges(sysUser); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't drop privileges: %v\n", err)
	}

	// The .env is kept, renamed, for an operator who adds the server back.
	envPath := filepath.Join(configDir, name+".env")
	if _, err := os.Stat(envPath); err == nil {
		stamp := time.Now().UTC().Format("20060102-150405")
		archived := envPath + ".removed-" + stamp
		if err := os.Rename(envPath, archived); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't archive %s: %v\n", envPath, err)
		} else {
			fmt.Println("Archived:", archived)
		}
	}

	found := config.RemoveServerByKey(cfg, name)
	if !found {
		found = config.RemoveServerByKey(cfg, strings.ToUpper(name))
	}
	if !found {
		for _, srv := range cfg.Q3Servers {
			if strings.EqualFold(srv.Key, name) {
				found = config.RemoveServerByKey(cfg, srv.Key)
				break
			}
		}
	}

	if found {
		if err := config.Save(*configPath, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Config: %s updated\n", *configPath)
	} else {
		fmt.Fprintf(os.Stderr, "Warning: no matching server entry found in config\n")
	}

	fmt.Println()
	fmt.Println("Restart trinity to apply: sudo systemctl restart trinity")
}

// collectPk3FilesOrdered returns pk3s in the game's load order, later
// files overriding earlier ones: baseq3, then missionpack, each with
// pak0-9 first and the rest alphabetical.
func collectPk3FilesOrdered(quake3Dir string) []string {
	var files []string

	info, err := os.Stat(quake3Dir)
	if err != nil {
		return files
	}
	if !info.IsDir() {
		if strings.HasSuffix(strings.ToLower(quake3Dir), ".pk3") {
			return []string{quake3Dir}
		}
		return files
	}

	hasStructure := false
	for _, subdir := range []string{"baseq3", "missionpack"} {
		if _, err := os.Stat(filepath.Join(quake3Dir, subdir)); err == nil {
			hasStructure = true
			break
		}
	}

	if hasStructure {
		for _, subdir := range []string{"baseq3", "missionpack"} {
			dir := filepath.Join(quake3Dir, subdir)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			files = append(files, collectPk3FilesFromDir(dir)...)
		}
	} else {
		files = collectPk3FilesFromDir(quake3Dir)
	}

	return files
}

// collectPk3FilesFromDir walks dir recursively; only root-level pak0-9
// sort ahead of the rest.
func collectPk3FilesFromDir(dir string) []string {
	var pakFiles []string
	var otherFiles []string

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".pk3") {
			return nil
		}

		name := d.Name()
		lowerName := strings.ToLower(name)

		isRootLevel := filepath.Dir(path) == dir
		if isRootLevel && strings.HasPrefix(lowerName, "pak") && len(lowerName) == 8 {
			numChar := lowerName[3]
			if numChar >= '0' && numChar <= '9' {
				pakFiles = append(pakFiles, path)
				return nil
			}
		}
		otherFiles = append(otherFiles, path)
		return nil
	})

	sort.Slice(pakFiles, func(i, j int) bool {
		return pakFiles[i] < pakFiles[j]
	})
	sort.Strings(otherFiles)

	return append(pakFiles, otherFiles...)
}

func extractTgaToPng(f *zip.File, outputPath string, targetSize int) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	img, err := tga.Decode(rc)
	if err != nil {
		return fmt.Errorf("decode TGA: %w", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != targetSize || bounds.Dy() != targetSize {
		dst := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)
		img = dst
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, img)
}
