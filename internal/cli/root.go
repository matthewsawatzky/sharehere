package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/matthewsawatzky/sharehere/internal/auth"
	"github.com/matthewsawatzky/sharehere/internal/config"
	"github.com/matthewsawatzky/sharehere/internal/db"
	"github.com/matthewsawatzky/sharehere/internal/server"
	"github.com/matthewsawatzky/sharehere/internal/theme"
	"github.com/matthewsawatzky/sharehere/internal/util"
)

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

type rootState struct {
	configPath string
	dataDir    string
}

type serveFlags struct {
	host      string
	port      int
	bind      string
	open      bool
	readonly  bool
	auth      string
	guestMode string
	basePath  string
	logLevel  string
	https     bool
	cert      string
	key       string
}

func NewRootCmd(v VersionInfo) *cobra.Command {
	state := &rootState{}
	serve := &serveFlags{}

	cmd := &cobra.Command{
		Use:   "sharehere [path]",
		Short: "Share directories over LAN with auth, uploads, and temporary links",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathArg := "."
			if len(args) == 1 {
				pathArg = args[0]
			}
			return runServe(cmd, state, serve, pathArg, v)
		},
	}
	cmd.PersistentFlags().StringVar(&state.configPath, "config", "", "config path (default: platform user config)")
	cmd.PersistentFlags().StringVar(&state.dataDir, "data-dir", "", "data directory for SQLite/config cache")
	addServeFlags(cmd, serve)

	serveCmd := &cobra.Command{
		Use:   "serve [path]",
		Short: "Serve a directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathArg := "."
			if len(args) == 1 {
				pathArg = args[0]
			}
			return runServe(cmd, state, serve, pathArg, v)
		},
	}
	addServeFlags(serveCmd, serve)

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Interactive first-run setup",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(state)
		},
	}

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Print config location and effective config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			fmt.Printf("Config path: %s\n", cfgPath)
			fmt.Printf("Data dir: %s\n", cfg.DataDir)
			if err := config.Validate(cfg); err != nil {
				fmt.Printf("Validation: failed (%v)\n", err)
			} else {
				fmt.Println("Validation: ok")
			}
			b, _ := json.MarshalIndent(cfg, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}

	userCmd := buildUserCommands(state)
	linkCmd := buildLinkCommands(state)
	themeCmd := buildThemeCommands(state)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("sharehere %s\ncommit: %s\nbuilt: %s\n", v.Version, v.Commit, v.Date)
		},
	}

	cmd.AddCommand(serveCmd, initCmd, configCmd, userCmd, linkCmd, themeCmd, versionCmd)
	return cmd
}

func addServeFlags(cmd *cobra.Command, f *serveFlags) {
	cmd.Flags().StringVar(&f.host, "host", "", "advertised host override for generated links")
	cmd.Flags().IntVar(&f.port, "port", 0, "server port")
	cmd.Flags().StringVar(&f.bind, "bind", "", "bind address (default from config, typically 0.0.0.0)")
	cmd.Flags().BoolVar(&f.open, "open", false, "open browser on startup")
	cmd.Flags().BoolVar(&f.readonly, "readonly", false, "force read-only mode")
	cmd.Flags().StringVar(&f.auth, "auth", "", "auth mode: on|off")
	cmd.Flags().StringVar(&f.guestMode, "guest-mode", "", "guest mode: off|read|upload")
	cmd.Flags().StringVar(&f.basePath, "basepath", "", "base URL path for reverse proxy (e.g. /sharehere)")
	cmd.Flags().StringVar(&f.logLevel, "log-level", "", "log level: debug|info|warn|error")
	cmd.Flags().BoolVar(&f.https, "https", false, "enable HTTPS")
	cmd.Flags().StringVar(&f.cert, "cert", "", "TLS certificate path")
	cmd.Flags().StringVar(&f.key, "key", "", "TLS key path")
}

func loadConfig(state *rootState) (string, config.Config, error) {
	cfgPath := strings.TrimSpace(state.configPath)
	if cfgPath == "" {
		p, err := config.ConfigPathFromEnv()
		if err != nil {
			return "", config.Config{}, err
		}
		cfgPath = p
	}
	cfg, err := config.LoadOrDefault(cfgPath, state.dataDir)
	if err != nil {
		return "", config.Config{}, err
	}
	if state.dataDir != "" {
		cfg.DataDir = state.dataDir
	}
	return cfgPath, cfg, nil
}

func mergeServeFlags(cmd *cobra.Command, cfg config.Config, f *serveFlags) (config.Config, bool, bool) {
	guestSet := cmd.Flags().Changed("guest-mode")
	readonlySet := cmd.Flags().Changed("readonly")

	if cmd.Flags().Changed("host") {
		cfg.Host = f.host
	}
	if cmd.Flags().Changed("port") {
		cfg.Port = f.port
	}
	if cmd.Flags().Changed("bind") {
		cfg.Bind = f.bind
	}
	if cmd.Flags().Changed("readonly") {
		cfg.ReadOnly = f.readonly
	}
	if cmd.Flags().Changed("auth") {
		cfg.Auth = strings.ToLower(strings.TrimSpace(f.auth))
	}
	if cmd.Flags().Changed("guest-mode") {
		cfg.GuestMode = strings.ToLower(strings.TrimSpace(f.guestMode))
	}
	if cmd.Flags().Changed("basepath") {
		cfg.BasePath = config.NormalizeBasePath(f.basePath)
	}
	if cmd.Flags().Changed("log-level") {
		cfg.LogLevel = strings.ToLower(strings.TrimSpace(f.logLevel))
	}
	if cmd.Flags().Changed("https") {
		cfg.HTTPS = f.https
	}
	if cmd.Flags().Changed("cert") {
		cfg.CertFile = f.cert
	}
	if cmd.Flags().Changed("key") {
		cfg.KeyFile = f.key
	}
	return cfg, guestSet, readonlySet
}

func runServe(cmd *cobra.Command, state *rootState, flags *serveFlags, pathArg string, v VersionInfo) error {
	cfgPath, cfg, err := loadConfig(state)
	if err != nil {
		return err
	}
	cfg, guestSet, readonlySet := mergeServeFlags(cmd, cfg, flags)
	if err := config.Validate(cfg); err != nil {
		return err
	}

	rootPath, err := filepath.Abs(pathArg)
	if err != nil {
		return err
	}
	if _, err := os.Stat(rootPath); err != nil {
		return err
	}

	opts := server.Options{
		RootDir:      rootPath,
		DataDir:      cfg.DataDir,
		Bind:         cfg.Bind,
		Host:         cfg.Host,
		Port:         cfg.Port,
		BasePath:     cfg.BasePath,
		ReadOnly:     cfg.ReadOnly,
		AuthMode:     cfg.Auth,
		LogLevel:     cfg.LogLevel,
		HTTPS:        cfg.HTTPS,
		CertFile:     cfg.CertFile,
		KeyFile:      cfg.KeyFile,
		OpenBrowser:  flags.open,
		Version:      v.Version,
		GuestMode:    cfg.GuestMode,
		GuestModeSet: guestSet,
		ReadOnlySet:  readonlySet,
	}

	scheme := "http"
	if opts.HTTPS {
		scheme = "https"
	}
	urls := util.DiscoverURLs(opts.Bind, opts.Port, opts.HTTPS, opts.BasePath)
	fmt.Printf("Serving: %s\n", rootPath)
	fmt.Printf("Config:  %s\n", cfgPath)
	fmt.Printf("Data:    %s\n", cfg.DataDir)
	fmt.Printf("Mode:    auth=%s guest=%s readonly=%v\n", cfg.Auth, cfg.GuestMode, cfg.ReadOnly)
	fmt.Println("URLs:")
	for _, u := range urls {
		fmt.Printf("  - %s\n", u)
	}
	if len(urls) > 0 {
		fmt.Println("QR (scan from phone on same LAN):")
		util.PrintTerminalQR(urls[0])
		if flags.open {
			go func(url string) {
				time.Sleep(350 * time.Millisecond)
				_ = util.OpenBrowser(url)
			}(urls[0])
		}
	} else {
		fallback := fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, opts.Port, opts.BasePath)
		fmt.Printf("  - %s\n", fallback)
	}
	fmt.Println("Press Ctrl+C to stop.")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return server.Run(ctx, opts)
}

func runInit(state *rootState) error {
	cfgPath := strings.TrimSpace(state.configPath)
	if cfgPath == "" {
		p, err := config.ConfigPathFromEnv()
		if err != nil {
			return err
		}
		cfgPath = p
	}
	cfg, err := config.LoadOrDefault(cfgPath, state.dataDir)
	if err != nil {
		return err
	}

	r := bufio.NewReader(os.Stdin)
	fmt.Println("sharehere first-run setup")
	cfg.DataDir = askWithDefault(r, "Data directory", cfg.DataDir)
	cfg.Bind = askWithDefault(r, "Bind address", cfg.Bind)
	cfg.Port = askIntWithDefault(r, "Port", cfg.Port)
	cfg.Auth = strings.ToLower(askWithDefault(r, "Authentication (on/off)", cfg.Auth))
	cfg.GuestMode = strings.ToLower(askWithDefault(r, "Guest mode (off/read/upload)", cfg.GuestMode))
	cfg.BasePath = config.NormalizeBasePath(askWithDefault(r, "Base path", cfg.BasePath))
	cfg.Theme = askWithDefault(r, "Default theme", cfg.Theme)
	cfg.DefaultShareExpiry = askWithDefault(r, "Default share expiry", cfg.DefaultShareExpiry)
	cfg.CollisionPolicy = strings.ToLower(askWithDefault(r, "Collision policy (rename/overwrite)", cfg.CollisionPolicy))
	cfg.MaxUploadSizeMB = int64(askIntWithDefault(r, "Max upload size MB", int(cfg.MaxUploadSizeMB)))
	cfg.AllowDelete = askBoolWithDefault(r, "Enable delete", cfg.AllowDelete)
	cfg.AllowRename = askBoolWithDefault(r, "Enable rename/move", cfg.AllowRename)
	cfg.ReadOnly = askBoolWithDefault(r, "Read-only mode", cfg.ReadOnly)

	if err := config.Validate(cfg); err != nil {
		return err
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}

	store, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	u, err := store.AdminCount()
	if err != nil {
		return err
	}
	if u == 0 {
		username := strings.ToLower(strings.TrimSpace(askWithDefault(r, "Admin username", "admin")))
		if username == "" {
			username = "admin"
		}
		password, err := promptPasswordTwice("Admin password")
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		if _, err := store.CreateUser(username, hash, auth.RoleAdmin); err != nil {
			return err
		}
		fmt.Printf("Created admin user %q\n", username)
	}

	if err := store.SetAppSettings(db.AppSettings{
		GuestMode:          cfg.GuestMode,
		MaxUploadSizeMB:    cfg.MaxUploadSizeMB,
		UploadAllowRegex:   cfg.UploadAllowRegex,
		UploadDenyRegex:    cfg.UploadDenyRegex,
		UploadSubdir:       cfg.UploadSubdir,
		CollisionPolicy:    cfg.CollisionPolicy,
		DefaultShareExpiry: cfg.DefaultShareExpiry,
		AllowDelete:        cfg.AllowDelete,
		AllowRename:        cfg.AllowRename,
		ReadOnly:           cfg.ReadOnly,
		Theme:              cfg.Theme,
		ThemeOverridesJSON: "{}",
	}); err != nil {
		return err
	}

	fmt.Printf("Config saved to %s\n", cfgPath)
	fmt.Println("Run `sharehere` to start serving the current directory.")
	return nil
}

func askWithDefault(r *bufio.Reader, label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	text, _ := r.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		return def
	}
	return text
}

func askIntWithDefault(r *bufio.Reader, label string, def int) int {
	for {
		value := askWithDefault(r, label, strconv.Itoa(def))
		n, err := strconv.Atoi(value)
		if err == nil && n > 0 {
			return n
		}
		fmt.Println("Please enter a positive integer.")
	}
}

func askBoolWithDefault(r *bufio.Reader, label string, def bool) bool {
	defaultStr := "n"
	if def {
		defaultStr = "y"
	}
	for {
		v := strings.ToLower(askWithDefault(r, label+" (y/n)", defaultStr))
		switch v {
		case "y", "yes", "true", "1":
			return true
		case "n", "no", "false", "0":
			return false
		default:
			fmt.Println("Enter y or n.")
		}
	}
}

func promptPassword(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return string(b), err
	}
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	return strings.TrimSpace(text), err
}

func promptPasswordTwice(label string) (string, error) {
	first, err := promptPassword(label)
	if err != nil {
		return "", err
	}
	second, err := promptPassword(label + " (confirm)")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("passwords do not match")
	}
	if strings.TrimSpace(first) == "" {
		return "", errors.New("password cannot be empty")
	}
	return first, nil
}

func buildUserCommands(state *rootState) *cobra.Command {
	userCmd := &cobra.Command{Use: "user", Short: "User management"}
	role := "user"

	addCmd := &cobra.Command{
		Use:   "add <username>",
		Short: "Create a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			pass, err := promptPasswordTwice("Password")
			if err != nil {
				return err
			}
			hash, err := auth.HashPassword(pass)
			if err != nil {
				return err
			}
			username := strings.ToLower(strings.TrimSpace(args[0]))
			if role != auth.RoleAdmin {
				role = auth.RoleUser
			}
			id, err := store.CreateUser(username, hash, role)
			if err != nil {
				return err
			}
			fmt.Printf("created user %s (id=%d role=%s)\n", username, id, role)
			return nil
		},
	}
	addCmd.Flags().StringVar(&role, "role", "user", "role: user|admin")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			users, err := store.ListUsers()
			if err != nil {
				return err
			}
			for _, u := range users {
				status := "active"
				if u.Disabled {
					status = "disabled"
				}
				fmt.Printf("%s\t%s\t%s\n", u.Username, u.Role, status)
			}
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <username>",
		Short: "Remove a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.DeleteUser(args[0])
		},
	}

	passwdCmd := &cobra.Command{
		Use:   "passwd <username>",
		Short: "Set a user password",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			pass, err := promptPasswordTwice("New password")
			if err != nil {
				return err
			}
			hash, err := auth.HashPassword(pass)
			if err != nil {
				return err
			}
			return store.SetUserPassword(args[0], hash)
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable <username>",
		Short: "Disable a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.SetUserDisabled(args[0], true)
		},
	}
	enableCmd := &cobra.Command{
		Use:   "enable <username>",
		Short: "Enable a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.SetUserDisabled(args[0], false)
		},
	}

	userCmd.AddCommand(addCmd, listCmd, removeCmd, passwdCmd, disableCmd, enableCmd)
	return userCmd
}

func buildLinkCommands(state *rootState) *cobra.Command {
	linkCmd := &cobra.Command{Use: "link", Short: "Share link operations"}
	expiry := "1h"
	mode := "browse"

	createCmd := &cobra.Command{
		Use:   "create [path]",
		Short: "Create a temporary share link",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			p := ""
			if len(args) == 1 {
				p = util.NormalizeRelPath(args[0])
			}
			d, err := time.ParseDuration(expiry)
			if err != nil {
				return err
			}
			token, err := util.RandomToken(18)
			if err != nil {
				return err
			}
			if err := store.CreateShareLink(db.ShareLink{Token: token, Path: p, Mode: mode, ExpiresAt: time.Now().Add(d)}); err != nil {
				return err
			}
			urls := util.DiscoverURLs(cfg.Bind, cfg.Port, cfg.HTTPS, config.NormalizeBasePath(cfg.BasePath))
			fmt.Printf("Token: %s\n", token)
			for _, u := range urls {
				fmt.Printf("%s/s/%s\n", strings.TrimRight(u, "/"), token)
			}
			return nil
		},
	}
	createCmd.Flags().StringVar(&expiry, "expiry", "1h", "expiry duration (e.g. 1h, 24h)")
	createCmd.Flags().StringVar(&mode, "mode", "browse", "mode: browse|download|upload")
	linkCmd.AddCommand(createCmd)
	return linkCmd
}

func buildThemeCommands(state *rootState) *cobra.Command {
	themeCmd := &cobra.Command{Use: "theme", Short: "Theme management"}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available themes",
		Run: func(cmd *cobra.Command, args []string) {
			for _, t := range theme.List() {
				fmt.Printf("%s\t%s\n", t.Name, t.Description)
			}
		},
	}
	setCmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Set active theme",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if _, err := theme.Resolve(name, theme.Overrides{}); err != nil {
				return err
			}
			_, cfg, err := loadConfig(state)
			if err != nil {
				return err
			}
			store, err := db.Open(cfg.DataDir)
			if err != nil {
				return err
			}
			defer store.Close()
			return store.SetSetting("theme", name)
		},
	}
	themeCmd.AddCommand(listCmd, setCmd)
	return themeCmd
}
