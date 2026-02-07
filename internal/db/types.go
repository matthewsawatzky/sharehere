package db

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	PasswordHash string    `json:"-"`
	Disabled     bool      `json:"disabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	Token      string    `json:"token"`
	UserID     *int64    `json:"user_id"`
	CSRFToken  string    `json:"csrf_token"`
	Remember   bool      `json:"remember"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type ShareLink struct {
	Token        string     `json:"token"`
	Path         string     `json:"path"`
	Mode         string     `json:"mode"`
	CreatedBy    *int64     `json:"created_by"`
	ExpiresAt    time.Time  `json:"expires_at"`
	Revoked      bool       `json:"revoked"`
	CreatedAt    time.Time  `json:"created_at"`
	LastAccessed *time.Time `json:"last_accessed"`
}

type AuditLog struct {
	ID          int64      `json:"id"`
	ActorUserID *int64     `json:"actor_user_id"`
	Action      string     `json:"action"`
	Target      string     `json:"target"`
	Metadata    string     `json:"metadata"`
	CreatedAt   time.Time  `json:"created_at"`
	Username    *string    `json:"username,omitempty"`
}

type AppSettings struct {
	GuestMode          string `json:"guest_mode"`
	MaxUploadSizeMB    int64  `json:"max_upload_size_mb"`
	UploadAllowRegex   string `json:"upload_allow_regex"`
	UploadDenyRegex    string `json:"upload_deny_regex"`
	UploadSubdir       string `json:"upload_subdir"`
	CollisionPolicy    string `json:"collision_policy"`
	DefaultShareExpiry string `json:"default_share_expiry"`
	AllowDelete        bool   `json:"allow_delete"`
	AllowRename        bool   `json:"allow_rename"`
	ReadOnly           bool   `json:"read_only"`
	Theme              string `json:"theme"`
	ThemeOverridesJSON string `json:"theme_overrides_json"`
	VirusScanCommand   string `json:"virus_scan_command"`
}

type LoginAttempt struct {
	Key        string     `json:"key"`
	Failed     int        `json:"failed"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
