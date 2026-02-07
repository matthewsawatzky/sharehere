package db

import (
	"fmt"
	"strconv"
)

var defaultSettings = map[string]string{
	"guest_mode":           "read",
	"max_upload_size_mb":   "1024",
	"upload_allow_regex":   "",
	"upload_deny_regex":    "",
	"upload_subdir":        "",
	"collision_policy":     "rename",
	"default_share_expiry": "24h",
	"allow_delete":         "false",
	"allow_rename":         "false",
	"read_only":            "false",
	"theme":                "light",
	"theme_overrides_json": "{}",
	"virus_scan_command":   "",
}

func (s *Store) ensureDefaultSettings() error {
	for k, v := range defaultSettings {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO settings(key, value) VALUES (?, ?)`, k, v); err != nil {
			return fmt.Errorf("seed setting %s: %w", k, err)
		}
	}
	return nil
}

func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

func parseBool(value string) bool {
	b, _ := strconv.ParseBool(value)
	return b
}

func (s *Store) GetAppSettings() (AppSettings, error) {
	result := AppSettings{}
	read := func(key string) (string, error) {
		v, err := s.GetSetting(key)
		if err != nil {
			if dv, ok := defaultSettings[key]; ok {
				return dv, nil
			}
			return "", err
		}
		return v, nil
	}
	var err error
	if result.GuestMode, err = read("guest_mode"); err != nil {
		return AppSettings{}, err
	}
	ms, err := read("max_upload_size_mb")
	if err != nil {
		return AppSettings{}, err
	}
	result.MaxUploadSizeMB, _ = strconv.ParseInt(ms, 10, 64)
	if result.MaxUploadSizeMB <= 0 {
		result.MaxUploadSizeMB = 1024
	}
	if result.UploadAllowRegex, err = read("upload_allow_regex"); err != nil {
		return AppSettings{}, err
	}
	if result.UploadDenyRegex, err = read("upload_deny_regex"); err != nil {
		return AppSettings{}, err
	}
	if result.UploadSubdir, err = read("upload_subdir"); err != nil {
		return AppSettings{}, err
	}
	if result.CollisionPolicy, err = read("collision_policy"); err != nil {
		return AppSettings{}, err
	}
	if result.DefaultShareExpiry, err = read("default_share_expiry"); err != nil {
		return AppSettings{}, err
	}
	v, err := read("allow_delete")
	if err != nil {
		return AppSettings{}, err
	}
	result.AllowDelete = parseBool(v)
	v, err = read("allow_rename")
	if err != nil {
		return AppSettings{}, err
	}
	result.AllowRename = parseBool(v)
	v, err = read("read_only")
	if err != nil {
		return AppSettings{}, err
	}
	result.ReadOnly = parseBool(v)
	if result.Theme, err = read("theme"); err != nil {
		return AppSettings{}, err
	}
	if result.ThemeOverridesJSON, err = read("theme_overrides_json"); err != nil {
		return AppSettings{}, err
	}
	if result.VirusScanCommand, err = read("virus_scan_command"); err != nil {
		return AppSettings{}, err
	}
	return result, nil
}

func (s *Store) SetAppSettings(v AppSettings) error {
	entries := map[string]string{
		"guest_mode":           v.GuestMode,
		"max_upload_size_mb":   strconv.FormatInt(v.MaxUploadSizeMB, 10),
		"upload_allow_regex":   v.UploadAllowRegex,
		"upload_deny_regex":    v.UploadDenyRegex,
		"upload_subdir":        v.UploadSubdir,
		"collision_policy":     v.CollisionPolicy,
		"default_share_expiry": v.DefaultShareExpiry,
		"allow_delete":         strconv.FormatBool(v.AllowDelete),
		"allow_rename":         strconv.FormatBool(v.AllowRename),
		"read_only":            strconv.FormatBool(v.ReadOnly),
		"theme":                v.Theme,
		"theme_overrides_json": v.ThemeOverridesJSON,
		"virus_scan_command":   v.VirusScanCommand,
	}
	for k, val := range entries {
		if err := s.SetSetting(k, val); err != nil {
			return err
		}
	}
	return nil
}
