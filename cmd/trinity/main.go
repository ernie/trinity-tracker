// trinity - Quake 3 Arena statistics and tools
package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/ernie/trinity-tools/internal/api"
	"github.com/ernie/trinity-tools/internal/auth"
	"github.com/ernie/trinity-tools/internal/collector"
	"github.com/ernie/trinity-tools/internal/config"
	"github.com/ernie/trinity-tools/internal/storage"
	"github.com/ftrvxmtrx/tga"
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
	case "serve":
		cmdServe(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "players":
		cmdPlayers(os.Args[2:])
	case "matches":
		cmdMatches(os.Args[2:])
	case "leaderboard":
		cmdLeaderboard(os.Args[2:])
	case "user":
		cmdUser(os.Args[2:])
	case "levelshots":
		cmdLevelshots(os.Args[2:])
	case "portraits":
		cmdPortraits(os.Args[2:])
	case "medals":
		cmdMedals(os.Args[2:])
	case "skills":
		cmdSkills(os.Args[2:])
	case "assets":
		cmdAssets(os.Args[2:])
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
	fmt.Println("  serve                              Start the stats server")
	fmt.Println("  status                             Show all servers status")
	fmt.Println("  players [--humans]                 Show current players across all servers")
	fmt.Println("  matches [--recent N]               Show recent matches (default: 20)")
	fmt.Println("  leaderboard [--top N]              Show top players (default: 20)")
	fmt.Println("  user add [--admin] [--player-id N] <username>")
	fmt.Println("                                     Add a user (prompts for password)")
	fmt.Println("  user remove <username>             Remove a user")
	fmt.Println("  user list                          List all users")
	fmt.Println("  user reset <username>              Reset a user's password")
	fmt.Println("  user admin <username>              Toggle admin status for a user")
	fmt.Println("  levelshots [path]                  Extract levelshots from pk3 file(s)")
	fmt.Println("  portraits [path]                   Extract player portraits from pk3 file(s)")
	fmt.Println("  medals [path]                      Extract medal icons from pk3 file(s)")
	fmt.Println("  skills [path]                      Extract skill icons from pk3 file(s)")
	fmt.Println("  assets [path]                      Extract all assets (portraits, medals, skills, levelshots)")
	fmt.Println("  version                            Show version")
	fmt.Println("  help                               Show this help")
	fmt.Println()
	fmt.Println("Global Options:")
	fmt.Println("  --config <path>    Path to configuration file (default /etc/trinity/config.yml)")
	fmt.Println("  --url <url>        Base URL of the trinity server (default: derived from config)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  trinity serve --config /etc/trinity/config.yml")
	fmt.Println("  trinity players --humans")
	fmt.Println("  trinity matches --recent 50")
	fmt.Println("  trinity user add --admin myuser")
}

// cmdServe starts the stats server
func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	// Determine config path
	cfgPath := *configPath
	if cfgPath == "" {
		if _, err := os.Stat(defaultConfigPath); err == nil {
			cfgPath = defaultConfigPath
		} else {
			log.Fatalf("No config file found at %s. Use --config to specify a config file.", defaultConfigPath)
		}
	}

	// Load configuration
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Trinity %s starting...", version)
	log.Printf("Monitoring %d servers", len(cfg.Q3Servers))

	// Initialize storage
	store, err := storage.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.Close()
	log.Printf("Database initialized at %s", cfg.Database.Path)

	// Create server manager
	manager := collector.NewServerManager(cfg, store)

	// Start the manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		log.Fatalf("Failed to start server manager: %v", err)
	}
	log.Printf("Server manager started, polling every %v", cfg.Server.PollInterval)

	// Create auth service
	authService := auth.NewService(cfg.Auth.JWTSecret, cfg.Auth.TokenDuration)
	if cfg.Auth.JWTSecret == "" {
		log.Printf("Warning: No JWT secret configured. Auth tokens will use an empty secret.")
	}

	// Create HTTP router
	router := api.NewRouter(store, manager, authService, cfg.Server.StaticDir)
	router.StartWebSocketHub()
	log.Printf("Serving static files from %s", cfg.Server.StaticDir)

	// Start HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP server listening on %s", addr)
		log.Printf("Web UI available at http://localhost%s", addr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for signal or error
	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-serverErr:
		log.Fatalf("HTTP server error: %v", err)
	}

	// Sequential shutdown
	log.Println("Shutting down HTTP server...")
	httpCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer httpCancel()
	if err := server.Shutdown(httpCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Stopping server manager...")
	manager.Stop()

	cancel()
	log.Println("Shutdown complete")
}

// CLI helper variables
var (
	baseURL = "http://localhost:8080"
	dbPath  string
)

// loadCLIConfigFromFlags loads config using pre-parsed flag values
func loadCLIConfigFromFlags(configPath, url string) *config.Config {
	// Load config file
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config from %s: %v\n", configPath, err)
		dbPath = "/var/lib/trinity/trinity.db"
		// Use explicit --url flag or default
		if url != "" {
			baseURL = url
		}
		return nil
	}

	dbPath = cfg.Database.Path
	// Derive URL from config, but allow --url flag to override
	if url != "" {
		baseURL = url
	} else {
		baseURL = fmt.Sprintf("http://%s:%d", cfg.Server.ListenAddr, cfg.Server.HTTPPort)
	}
	return cfg
}

func loadCLIConfig(args []string) (*config.Config, []string) {
	fs := flag.NewFlagSet("cli", flag.ContinueOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, *url)
	return cfg, fs.Args()
}

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	// Get servers
	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVER\tMAP\tPLAYERS\tHUMANS\tSTATUS")
	fmt.Fprintln(w, "------\t---\t-------\t------\t------")

	for _, srv := range servers {
		id := int64(srv["id"].(float64))
		name := srv["name"].(string)

		var status map[string]interface{}
		if err := getJSON(fmt.Sprintf("/api/servers/%d/status", id), &status); err != nil {
			fmt.Fprintf(w, "%s\t-\t-\t-\tOFFLINE\n", name)
			continue
		}

		mapName := "-"
		if m, ok := status["map"].(string); ok {
			mapName = m
		}

		players := 0
		humans := 0
		if p, ok := status["players"].([]interface{}); ok {
			players = len(p)
			for _, player := range p {
				if pm, ok := player.(map[string]interface{}); ok {
					if !pm["is_bot"].(bool) {
						humans++
					}
				}
			}
		}

		statusStr := "ONLINE"
		if online, ok := status["online"].(bool); ok && !online {
			statusStr = "OFFLINE"
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", name, mapName, players, humans, statusStr)
	}

	w.Flush()
}

func cmdPlayers(args []string) {
	fs := flag.NewFlagSet("players", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	humansOnly := fs.Bool("humans", false, "show only human players")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	// Get servers
	var servers []map[string]interface{}
	if err := getJSON("/api/servers", &servers); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SERVER\tPLAYER\tSCORE\tPING\tTYPE")
	fmt.Fprintln(w, "------\t------\t-----\t----\t----")

	for _, srv := range servers {
		id := int64(srv["id"].(float64))
		name := srv["name"].(string)

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

			cleanName := pm["clean_name"].(string)
			score := int(pm["score"].(float64))
			ping := int(pm["ping"].(float64))

			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", name, cleanName, score, ping, playerType)
		}
	}

	w.Flush()
}

func cmdMatches(args []string) {
	fs := flag.NewFlagSet("matches", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	limit := fs.Int("recent", 20, "number of recent matches to show")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	var matches []map[string]interface{}
	if err := getJSON(fmt.Sprintf("/api/matches?limit=%d", *limit), &matches); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tMAP\tSTARTED\tENDED\tEXIT REASON")
	fmt.Fprintln(w, "--\t---\t-------\t-----\t-----------")

	for _, match := range matches {
		id := int64(match["id"].(float64))
		mapName := match["map_name"].(string)

		started := "-"
		if s, ok := match["started_at"].(string); ok {
			started = formatTime(s)
		}

		ended := "In Progress"
		if e, ok := match["ended_at"].(string); ok && e != "" {
			ended = formatTime(e)
		}

		exitReason := "-"
		if r, ok := match["exit_reason"].(string); ok && r != "" {
			exitReason = r
		}

		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", id, mapName, started, ended, exitReason)
	}

	w.Flush()
}

func cmdLeaderboard(args []string) {
	fs := flag.NewFlagSet("leaderboard", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	url := fs.String("url", "", "base URL of the trinity server")
	limit := fs.Int("top", 20, "number of top players to show")
	fs.Parse(args)

	loadCLIConfigFromFlags(*configPath, *url)

	var response map[string]interface{}
	if err := getJSON(fmt.Sprintf("/api/stats/leaderboard?limit=%d", *limit), &response); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	entries, ok := response["entries"].([]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unexpected response format\n")
		os.Exit(1)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RANK\tPLAYER\tFRAGS\tDEATHS\tK/D\tMATCHES")
	fmt.Fprintln(w, "----\t------\t-----\t------\t---\t-------")

	for i, entry := range entries {
		stat := entry.(map[string]interface{})
		player := stat["player"].(map[string]interface{})
		name := player["clean_name"].(string)
		frags := int64(stat["total_frags"].(float64))
		deaths := int64(stat["total_deaths"].(float64))
		kd := stat["kd_ratio"].(float64)
		matches := int64(stat["total_matches"].(float64))

		fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%.2f\t%d\n", i+1, name, frags, deaths, kd, matches)
	}

	w.Flush()
}

// cmdUser handles user subcommands
func cmdUser(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Error: user subcommand required: add, remove, list, reset, admin\n")
		os.Exit(1)
	}

	// For user commands, we need config but also the subcommand
	subCmd := args[0]
	cfg, remaining := loadCLIConfig(args[1:])
	_ = cfg // cfg may be nil if config loading failed

	// Open database
	store, err := storage.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	ctx := context.Background()

	switch subCmd {
	case "add":
		if err := cmdUserAdd(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "remove":
		if err := cmdUserRemove(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := cmdUserList(ctx, store); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "reset":
		if err := cmdUserReset(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err := cmdUserAdmin(ctx, store, remaining); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown user command: %s (use: add, remove, list, reset, admin)\n", subCmd)
		os.Exit(1)
	}
}

func cmdUserAdd(ctx context.Context, store *storage.Store, args []string) error {
	fs := flag.NewFlagSet("user add", flag.ExitOnError)
	isAdmin := fs.Bool("admin", false, "create as admin user")
	playerIDFlag := fs.Int64("player-id", 0, "link to player ID")
	fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 1 {
		return fmt.Errorf("usage: trinity user add [--admin] [--player-id N] <username>")
	}

	username := remaining[0]
	var playerID *int64
	if *playerIDFlag != 0 {
		playerID = playerIDFlag
	}

	// Check if user already exists
	if _, err := store.GetUserByUsername(ctx, username); err == nil {
		return fmt.Errorf("user '%s' already exists", username)
	}

	// Check if player is already claimed
	if playerID != nil {
		claimed, err := store.IsPlayerClaimed(ctx, *playerID)
		if err != nil {
			return fmt.Errorf("failed to check player: %w", err)
		}
		if claimed {
			return fmt.Errorf("player %d is already linked to another user", *playerID)
		}
	}

	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := store.CreateUser(ctx, username, hash, *isAdmin, playerID); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	roleStr := "user"
	if *isAdmin {
		roleStr = "admin"
	}
	fmt.Printf("User '%s' created successfully (role: %s)\n", username, roleStr)
	return nil
}

func cmdUserRemove(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user remove <username>")
	}
	username := args[0]

	if err := store.DeleteUser(ctx, username); err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
	}

	fmt.Printf("User '%s' removed\n", username)
	return nil
}

func cmdUserList(ctx context.Context, store *storage.Store) error {
	users, err := store.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	if len(users) == 0 {
		fmt.Println("No users configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USERNAME\tROLE\tPLAYER_ID\tPWD_CHANGE\tLAST_LOGIN")
	fmt.Fprintln(w, "--------\t----\t---------\t----------\t----------")

	for _, user := range users {
		role := "user"
		if user.IsAdmin {
			role = "admin"
		}
		playerID := "-"
		if user.PlayerID != nil {
			playerID = fmt.Sprintf("%d", *user.PlayerID)
		}
		pwdChange := "no"
		if user.PasswordChangeRequired {
			pwdChange = "yes"
		}
		lastLogin := "never"
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", user.Username, role, playerID, pwdChange, lastLogin)
	}
	return w.Flush()
}

func cmdUserReset(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user reset <username>")
	}
	username := args[0]

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	fmt.Print("Enter new password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := auth.HashPassword(string(password))
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := store.ResetUserPassword(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	fmt.Printf("Password reset for '%s' (user will be required to change it on next login)\n", username)
	return nil
}

func cmdUserAdmin(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: trinity user admin <username>")
	}
	username := args[0]

	user, err := store.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("user not found: %s", username)
	}

	newAdminStatus := !user.IsAdmin
	if err := store.UpdateUserAdmin(ctx, user.ID, newAdminStatus); err != nil {
		return fmt.Errorf("failed to update admin status: %w", err)
	}

	if newAdminStatus {
		fmt.Printf("User '%s' is now an admin\n", username)
	} else {
		fmt.Printf("User '%s' is no longer an admin\n", username)
	}
	return nil
}

// cmdLevelshots extracts levelshot images from pk3 files
func cmdLevelshots(args []string) {
	fs := flag.NewFlagSet("levelshots", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	// Validate and create output directory
	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "levelshots")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory %s: %v\n", outputDir, err)
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

// extractLevelshotsFromPk3 extracts levelshot images from a single pk3 file
func extractLevelshotsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		// Check if this is a levelshot
		lowerName := strings.ToLower(f.Name)
		if !strings.HasPrefix(lowerName, "levelshots/") {
			continue
		}

		// Extract map name and extension
		base := filepath.Base(f.Name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".jpg" && ext != ".tga" {
			continue
		}

		mapName := strings.TrimSuffix(base, filepath.Ext(base))
		mapName = strings.ToLower(mapName)

		// Output path is always .jpg
		outputPath := filepath.Join(outputDir, mapName+".jpg")

		// Extract and potentially convert
		if err := extractLevelshot(f, outputPath, ext); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", mapName, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, mapName)
		extracted++
	}

	return extracted, nil
}

// extractLevelshot extracts a single levelshot, converting TGA to JPG if needed
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
		return fmt.Errorf("failed to decode %s: %w", ext, err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	return jpeg.Encode(out, img, &jpeg.Options{Quality: 90})
}

// pk3DisplayPath returns a display-friendly path for a pk3 file relative to basePath
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
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func formatTime(isoTime string) string {
	// Simple formatting - just show time portion
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

// cmdPortraits extracts player portrait icons from pk3 files
func cmdPortraits(args []string) {
	fs := flag.NewFlagSet("portraits", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "portraits")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
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
		n, err := extractPortraitsFromPk3(pk3Path, outputDir, displayPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", displayPath, err)
			continue
		}
		totalExtracted += n
	}

	fmt.Printf("Portraits: %d extracted\n", totalExtracted)
}

// extractPortraitsFromPk3 extracts player portrait icons from a pk3 file
func extractPortraitsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		// Match models/players/<model>/icon_<skin>.tga
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

		// Extract model name from path
		parts := strings.Split(f.Name, "/")
		if len(parts) < 4 {
			continue
		}
		model := strings.ToLower(parts[2])

		// Create model subdirectory
		modelDir := filepath.Join(outputDir, model)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to create directory %s: %v\n", modelDir, err)
			continue
		}

		// Output path: portraits/<model>/icon_<skin>.png
		outputName := strings.TrimSuffix(strings.ToLower(base), ".tga") + ".png"
		outputPath := filepath.Join(modelDir, outputName)
		assetName := model + "/" + outputName

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, assetName)
		extracted++
	}

	return extracted, nil
}

// cmdMedals extracts medal icons from pk3 files
func cmdMedals(args []string) {
	fs := flag.NewFlagSet("medals", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	// Use remaining arg as path override, or default to quake3_dir from config
	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "medals")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
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

// extractMedalsFromPk3 extracts medal icons from a pk3 file
func extractMedalsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		// Match menu/medals/medal_*.tga or ui/assets/medal_*.tga
		isMedalPath := (strings.HasPrefix(lowerName, "menu/medals/") || strings.HasPrefix(lowerName, "ui/assets/")) &&
			strings.HasPrefix(base, "medal_") &&
			strings.HasSuffix(base, ".tga")

		if !isMedalPath {
			continue
		}

		// Output: medals/medal_*.png (flat structure)
		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

// cmdSkills extracts skill icons from pk3 files
func cmdSkills(args []string) {
	fs := flag.NewFlagSet("skills", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	if cfg.Server.StaticDir == "" {
		fmt.Fprintf(os.Stderr, "Error: static_dir not configured in config file\n")
		os.Exit(1)
	}

	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	outputDir := filepath.Join(cfg.Server.StaticDir, "assets", "skills")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create output directory: %v\n", err)
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

// extractSkillsFromPk3 extracts skill icons from a pk3 file
func extractSkillsFromPk3(pk3Path, outputDir, displayPath string) (int, error) {
	r, err := zip.OpenReader(pk3Path)
	if err != nil {
		return 0, fmt.Errorf("failed to open pk3: %w", err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		lowerName := strings.ToLower(f.Name)
		base := strings.ToLower(filepath.Base(f.Name))

		// Match menu/art/skill[1-5].tga
		if !strings.HasPrefix(lowerName, "menu/art/") {
			continue
		}
		if !strings.HasPrefix(base, "skill") || !strings.HasSuffix(base, ".tga") {
			continue
		}
		// Verify it's skill1-5
		numPart := strings.TrimPrefix(base, "skill")
		numPart = strings.TrimSuffix(numPart, ".tga")
		if len(numPart) != 1 || numPart[0] < '1' || numPart[0] > '5' {
			continue
		}

		// Output: skills/skill[1-5].png
		outputName := strings.TrimSuffix(base, ".tga") + ".png"
		outputPath := filepath.Join(outputDir, outputName)

		if err := extractTgaToPng(f, outputPath, 128); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to extract %s: %v\n", f.Name, err)
			continue
		}

		fmt.Printf("  %s: %s\n", displayPath, outputName)
		extracted++
	}

	return extracted, nil
}

// cmdAssets runs all asset extraction commands
func cmdAssets(args []string) {
	fs := flag.NewFlagSet("assets", flag.ExitOnError)
	configPath := fs.String("config", defaultConfigPath, "path to configuration file")
	fs.Parse(args)

	cfg := loadCLIConfigFromFlags(*configPath, "")
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config\n")
		os.Exit(1)
	}

	remaining := fs.Args()
	inputPath := cfg.Server.Quake3Dir
	if len(remaining) > 0 {
		inputPath = remaining[0]
	}

	// Build args for sub-commands
	subArgs := []string{"--config", *configPath, inputPath}

	fmt.Println("=== Extracting Levelshots ===")
	cmdLevelshots(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Portraits ===")
	cmdPortraits(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Medals ===")
	cmdMedals(subArgs)
	fmt.Println()

	fmt.Println("=== Extracting Skills ===")
	cmdSkills(subArgs)
	fmt.Println()

	fmt.Println("=== All asset extraction complete ===")
}

// collectPk3FilesOrdered returns pk3 files in Quake 3 load order (later files override earlier)
// Order: pak0-9 numerically, then remaining pk3s alphabetically
// Applied to baseq3 first, then missionpack
func collectPk3FilesOrdered(quake3Dir string) []string {
	var files []string

	// Check if quake3Dir is a single file
	info, err := os.Stat(quake3Dir)
	if err != nil {
		return files
	}
	if !info.IsDir() {
		// Single pk3 file
		if strings.HasSuffix(strings.ToLower(quake3Dir), ".pk3") {
			return []string{quake3Dir}
		}
		return files
	}

	// Check if this directory has baseq3/missionpack structure
	hasStructure := false
	for _, subdir := range []string{"baseq3", "missionpack"} {
		if _, err := os.Stat(filepath.Join(quake3Dir, subdir)); err == nil {
			hasStructure = true
			break
		}
	}

	if hasStructure {
		// Process baseq3 and missionpack in order
		for _, subdir := range []string{"baseq3", "missionpack"} {
			dir := filepath.Join(quake3Dir, subdir)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			}
			files = append(files, collectPk3FilesFromDir(dir)...)
		}
	} else {
		// No standard structure, scan the directory directly
		files = collectPk3FilesFromDir(quake3Dir)
	}

	return files
}

// collectPk3FilesFromDir recursively collects pk3 files from a directory
// Returns them in Quake 3 load order: pak0-9 first, then others alphabetically
func collectPk3FilesFromDir(dir string) []string {
	var pakFiles []string   // pak[0-9].pk3 at root level
	var otherFiles []string // other pk3s

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".pk3") {
			return nil
		}

		name := d.Name()
		lowerName := strings.ToLower(name)

		// Only treat pak[0-9].pk3 at the root level specially
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

	// Sort pak files numerically (pak0, pak1, ..., pak9)
	sort.Slice(pakFiles, func(i, j int) bool {
		return pakFiles[i] < pakFiles[j]
	})

	// Sort other files alphabetically
	sort.Strings(otherFiles)

	// Pak files first, then other files
	return append(pakFiles, otherFiles...)
}

// extractTgaToPng extracts a TGA file from a zip, scales to targetSize, and saves as PNG
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

	// Scale to target size using Catmull-Rom (bicubic) interpolation
	// CatmullRom produces sharper results than bilinear, better for pixel art
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
