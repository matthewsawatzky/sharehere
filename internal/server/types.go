package server

import (
	"time"
)

type Options struct {
	RootDir          string
	DataDir          string
	Bind             string
	Host             string
	Port             int
	BasePath         string
	ReadOnly         bool
	AuthMode         string
	LogLevel         string
	HTTPS            bool
	CertFile         string
	KeyFile          string
	OpenBrowser      bool
	Version          string
	GuestMode        string
	GuestModeSet     bool
	ReadOnlySet      bool
}

type Permissions struct {
	CanBrowse bool `json:"canBrowse"`
	CanUpload bool `json:"canUpload"`
	CanDelete bool `json:"canDelete"`
	CanRename bool `json:"canRename"`
	CanShare  bool `json:"canShare"`
	CanAdmin  bool `json:"canAdmin"`
	ReadOnly  bool `json:"readonly"`
}

type fileEntry struct {
	Name    string    `json:"name"`
	RelPath string    `json:"relPath"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Ext     string    `json:"ext"`
}

type breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
