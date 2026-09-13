package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maskedsyntax/goggles/internal/apperr"
	toml "github.com/pelletier/go-toml/v2"
)

const envHome = "GOGGLES_HOME"

type Paths struct {
	Home       string
	ConfigFile string
	DataDir    string
	DBFile     string
	LogDir     string
}

func ResolvePaths(configFlag string) (Paths, error) {
	if home := strings.TrimSpace(os.Getenv(envHome)); home != "" {
		p := Paths{
			Home:       home,
			ConfigFile: filepath.Join(home, "config.toml"),
			DataDir:    home,
			DBFile:     filepath.Join(home, "goggles.db"),
			LogDir:     filepath.Join(home, "logs"),
		}
		if configFlag != "" {
			p.ConfigFile = configFlag
		}
		return p, nil
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, apperr.Wrap(apperr.ConfigMissing, "cannot resolve home directory", err)
	}
	p := Paths{
		Home:       userHome,
		ConfigFile: filepath.Join(userHome, ".config", "goggles", "config.toml"),
		DataDir:    filepath.Join(userHome, ".local", "share", "goggles"),
		LogDir:     filepath.Join(userHome, ".local", "share", "goggles", "logs"),
	}
	p.DBFile = filepath.Join(p.DataDir, "goggles.db")
	if configFlag != "" {
		p.ConfigFile = configFlag
	}
	return p, nil
}

type File struct {
	General    General    `toml:"general"`
	Publishing Publishing `toml:"publishing"`
	Storage    Storage    `toml:"storage"`
	R2         R2         `toml:"r2"`
	Meta       Meta       `toml:"meta"`
	Google     Google     `toml:"google"`
}

type General struct {
	Timezone string `toml:"timezone"`
}

type Publishing struct {
	MaxRetries         int  `toml:"max_retries"`
	PreventDuplicates  bool `toml:"prevent_duplicates"`
	GlobalConcurrency  int  `toml:"global_concurrency"`
	PerDestinationJobs int  `toml:"per_destination_jobs"`
}

type Storage struct {
	Provider          string `toml:"provider"`
	CleanupTTLMinutes int    `toml:"cleanup_ttl_minutes"`
}

type R2 struct {
	AccountID     string `toml:"account_id"`
	Bucket        string `toml:"bucket"`
	PublicBaseURL string `toml:"public_base_url"`
	Endpoint      string `toml:"endpoint"`
	AccessKeyRef  string `toml:"access_key_ref"`
	SecretKeyRef  string `toml:"secret_key_ref"`
}

type Meta struct {
	AppID        string `toml:"app_id"`
	AppSecretRef string `toml:"app_secret_ref"`
	GraphVersion string `toml:"graph_version"`
	GraphHost    string `toml:"graph_host"`
	OAuthPort    int    `toml:"oauth_port"`
}

type Google struct {
	ClientID        string `toml:"client_id"`
	ClientSecretRef string `toml:"client_secret_ref"`
}

func Default() File {
	return File{
		General: General{Timezone: "UTC"},
		Publishing: Publishing{
			MaxRetries:         5,
			PreventDuplicates:  true,
			GlobalConcurrency:  3,
			PerDestinationJobs: 1,
		},
		Storage: Storage{
			Provider:          "r2",
			CleanupTTLMinutes: 120,
		},
		R2: R2{
			AccessKeyRef: "keychain:goggles-r2-access",
			SecretKeyRef: "keychain:goggles-r2-secret",
		},
		Meta: Meta{
			AppSecretRef: "keychain:goggles-meta-app-secret",
			GraphVersion: "v25.0",
			GraphHost:    "https://graph.instagram.com",
			OAuthPort:    8787,
		},
		Google: Google{
			ClientSecretRef: "keychain:goggles-google-client-secret",
		},
	}
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, apperr.Wrap(apperr.ConfigMissing, "cannot read config file", err)
	}
	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return File{}, apperr.Wrap(apperr.ConfigMissing, "cannot parse config file", err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (f *File) applyDefaults() {
	d := Default()
	if f.General.Timezone == "" {
		f.General.Timezone = d.General.Timezone
	}
	if f.Publishing.MaxRetries == 0 {
		f.Publishing.MaxRetries = d.Publishing.MaxRetries
	}
	if f.Publishing.GlobalConcurrency == 0 {
		f.Publishing.GlobalConcurrency = d.Publishing.GlobalConcurrency
	}
	if f.Publishing.PerDestinationJobs == 0 {
		f.Publishing.PerDestinationJobs = d.Publishing.PerDestinationJobs
	}
	if f.Storage.Provider == "" {
		f.Storage.Provider = d.Storage.Provider
	}
	if f.Storage.CleanupTTLMinutes == 0 {
		f.Storage.CleanupTTLMinutes = d.Storage.CleanupTTLMinutes
	}
	if f.Meta.GraphVersion == "" {
		f.Meta.GraphVersion = d.Meta.GraphVersion
	}
	if f.Meta.GraphHost == "" {
		f.Meta.GraphHost = d.Meta.GraphHost
	}
	if f.Meta.AppSecretRef == "" {
		f.Meta.AppSecretRef = d.Meta.AppSecretRef
	}
	if f.Meta.OAuthPort == 0 {
		f.Meta.OAuthPort = d.Meta.OAuthPort
	}
	if f.Google.ClientSecretRef == "" {
		f.Google.ClientSecretRef = d.Google.ClientSecretRef
	}
	if f.R2.AccessKeyRef == "" {
		f.R2.AccessKeyRef = d.R2.AccessKeyRef
	}
	if f.R2.SecretKeyRef == "" {
		f.R2.SecretKeyRef = d.R2.SecretKeyRef
	}
}

func Ensure(path string) (File, error) {
	if _, err := os.Stat(path); err == nil {
		return Load(path)
	} else if !os.IsNotExist(err) {
		return File{}, apperr.Wrap(apperr.ConfigMissing, "cannot stat config file", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return File{}, apperr.Wrap(apperr.ConfigMissing, "cannot create config directory", err)
	}
	cfg := Default()
	if err := Write(path, cfg); err != nil {
		return File{}, err
	}
	return cfg, nil
}

func Write(path string, cfg File) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return apperr.Wrap(apperr.ConfigMissing, "cannot encode config", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return apperr.Wrap(apperr.ConfigMissing, "cannot create config directory", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return apperr.Wrap(apperr.ConfigMissing, "cannot write config file", err)
	}
	return nil
}

func EnsureDataDirs(p Paths) error {
	for _, dir := range []string{p.DataDir, p.LogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return apperr.Wrap(apperr.ConfigMissing, "cannot create data directory", err)
		}
	}
	return nil
}

func (f *File) Set(key, value string) error {
	switch key {
	case "general.timezone":
		f.General.Timezone = value
	case "publishing.max_retries":
		n, err := parsePositiveInt(value)
		if err != nil {
			return err
		}
		f.Publishing.MaxRetries = n
	case "publishing.prevent_duplicates":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		f.Publishing.PreventDuplicates = b
	case "publishing.global_concurrency":
		n, err := parsePositiveInt(value)
		if err != nil {
			return err
		}
		f.Publishing.GlobalConcurrency = n
	case "storage.provider":
		f.Storage.Provider = value
	case "storage.cleanup_ttl_minutes":
		n, err := parsePositiveInt(value)
		if err != nil {
			return err
		}
		f.Storage.CleanupTTLMinutes = n
	case "r2.account_id":
		f.R2.AccountID = value
	case "r2.bucket":
		f.R2.Bucket = value
	case "r2.public_base_url":
		f.R2.PublicBaseURL = value
	case "r2.endpoint":
		f.R2.Endpoint = value
	case "r2.access_key_ref":
		f.R2.AccessKeyRef = value
	case "r2.secret_key_ref":
		f.R2.SecretKeyRef = value
	case "meta.app_id":
		f.Meta.AppID = value
	case "meta.app_secret_ref":
		f.Meta.AppSecretRef = value
	case "meta.oauth_port":
		n, err := parsePositiveInt(value)
		if err != nil {
			return err
		}
		f.Meta.OAuthPort = n
	case "meta.graph_version":
		f.Meta.GraphVersion = value
	case "meta.graph_host":
		f.Meta.GraphHost = value
	case "google.client_id":
		f.Google.ClientID = value
	case "google.client_secret_ref":
		f.Google.ClientSecretRef = value
	default:
		return apperr.Invalid("unknown config key " + key)
	}
	return nil
}

func parsePositiveInt(value string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil || n <= 0 {
		return 0, apperr.Invalid("expected a positive integer")
	}
	return n, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, apperr.Invalid("expected true or false")
	}
}
