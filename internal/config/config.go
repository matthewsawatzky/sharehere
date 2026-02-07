package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	AuthOn  = "on"
	AuthOff = "off"
)

const (
	GuestOff    = "off"
	GuestRead   = "read"
	GuestUpload = "upload"
)

const (
	CollisionRename    = "rename"
	CollisionOverwrite = "overwrite"
)

type Config struct {
	Bind               string `json:"bind"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	BasePath           string `json:"base_path"`
	LogLevel           string `json:"log_level"`
	DataDir            string `json:"data_dir"`
	Auth               string `json:"auth"`
	GuestMode          string `json:"guest_mode"`
	ReadOnly           bool   `json:"read_only"`
	HTTPS              bool   `json:"https"`
	CertFile           string `json:"cert_file"`
	KeyFile            string `json:"key_file"`
	DefaultShareExpiry string `json:"default_share_expiry"`
	Theme              string `json:"theme"`

	MaxUploadSizeMB  int64  `json:"max_upload_size_mb"`
	UploadAllowRegex string `json:"upload_allow_regex"`
	UploadDenyRegex  string `json:"upload_deny_regex"`
	UploadSubdir     string `json:"upload_subdir"`
	CollisionPolicy  string `json:"collision_policy"`
	AllowDelete      bool   `json:"allow_delete"`
	AllowRename      bool   `json:"allow_rename"`
}

func DefaultPaths() (configPath, dataDir string, err error) {
	cfgRoot, err := os.UserConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve user config dir: %w", err)
	}
	var dataRoot string
	switch runtime.GOOS {
	case "windows":
		dataRoot = cfgRoot
	default:
		if p, derr := os.UserHomeDir(); derr == nil {
			dataRoot = filepath.Join(p, ".local", "share")
		} else {
			dataRoot = cfgRoot
		}
	}
	configPath = filepath.Join(cfgRoot, "sharehere", "config.json")
	dataDir = filepath.Join(dataRoot, "sharehere")
	return configPath, dataDir, nil
}

func Default(dataDir string) Config {
	return Config{
		Bind:               "0.0.0.0",
		Host:               "",
		Port:               7331,
		BasePath:           "/",
		LogLevel:           "info",
		DataDir:            dataDir,
		Auth:               AuthOn,
		GuestMode:          GuestOff,
		ReadOnly:           false,
		HTTPS:              false,
		CertFile:           "",
		KeyFile:            "",
		DefaultShareExpiry: "24h",
		Theme:              "light",
		MaxUploadSizeMB:    1024,
		UploadAllowRegex:   "",
		UploadDenyRegex:    "",
		UploadSubdir:       "",
		CollisionPolicy:    CollisionRename,
		AllowDelete:        false,
		AllowRename:        false,
	}
}

func NormalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}

func LoadOrDefault(configPath, dataDirOverride string) (Config, error) {
	_, defaultData, err := DefaultPaths()
	if err != nil {
		return Config{}, err
	}
	cfg := Default(defaultData)
	if dataDirOverride != "" {
		cfg.DataDir = dataDirOverride
	}

	b, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if dataDirOverride != "" {
		cfg.DataDir = dataDirOverride
	}
	cfg.BasePath = NormalizeBasePath(cfg.BasePath)
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(configPath string, cfg Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	cfg.BasePath = NormalizeBasePath(cfg.BasePath)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	buf, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(configPath, buf, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func Validate(cfg Config) error {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port %d", cfg.Port)
	}
	switch cfg.Auth {
	case AuthOn, AuthOff:
	default:
		return fmt.Errorf("invalid auth mode %q", cfg.Auth)
	}
	switch cfg.GuestMode {
	case GuestOff, GuestRead, GuestUpload:
	default:
		return fmt.Errorf("invalid guest mode %q", cfg.GuestMode)
	}
	switch cfg.CollisionPolicy {
	case CollisionRename, CollisionOverwrite:
	default:
		return fmt.Errorf("invalid collision policy %q", cfg.CollisionPolicy)
	}
	if cfg.MaxUploadSizeMB <= 0 {
		return fmt.Errorf("max upload size must be positive")
	}
	if cfg.HTTPS && (cfg.CertFile == "" || cfg.KeyFile == "") {
		return fmt.Errorf("https enabled but cert/key missing")
	}
	return nil
}

func ConfigPathFromEnv() (string, error) {
	if p := strings.TrimSpace(os.Getenv("SHAREHERE_CONFIG")); p != "" {
		return p, nil
	}
	cfgPath, _, err := DefaultPaths()
	return cfgPath, err
}
