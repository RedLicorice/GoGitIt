package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Auth    AuthConfig    `mapstructure:"auth"`
	Storage StorageConfig `mapstructure:"storage"`
	CORS    CORSConfig    `mapstructure:"cors"`
	GH      GHConfig      `mapstructure:"gh"`
}

// GHConfig gates the optional GitHub CLI integration. Allowed in both local and
// server mode; operators can disable it globally here or via GOGITIT_GH_ENABLED.
type GHConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

type ServerConfig struct {
	Addr string `mapstructure:"addr"`
}

type AuthConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	Issuer       string   `mapstructure:"issuer"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	Scopes       []string `mapstructure:"scopes"`
	CookieDomain string   `mapstructure:"cookie_domain"`
	CookieSecure bool     `mapstructure:"cookie_secure"`
}

type StorageConfig struct {
	ReposDir string `mapstructure:"repos_dir"`
	StateDir string `mapstructure:"state_dir"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// Load reads configuration from config.yaml (if present) and environment
// variables. Env vars take precedence; nested keys map with underscores:
// GOGITIT_AUTH_ENABLED, GOGITIT_SERVER_ADDR, etc.
func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/gogitit")

	// Defaults — sensible for local dev with auth disabled.
	v.SetDefault("server.addr", "0.0.0.0:8080")
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.scopes", []string{"openid", "profile", "email"})
	v.SetDefault("auth.cookie_secure", false)
	v.SetDefault("storage.repos_dir", "./data/repos")
	v.SetDefault("storage.state_dir", "./data/state")
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:5173"})
	v.SetDefault("gh.enabled", true)

	v.SetEnvPrefix("GOGITIT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// Missing config file is fine; defaults + env are enough.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Auth.Enabled {
		if cfg.Auth.Issuer == "" || cfg.Auth.ClientID == "" || cfg.Auth.RedirectURL == "" {
			return nil, fmt.Errorf("auth enabled but issuer/client_id/redirect_url missing")
		}
	}

	return &cfg, nil
}
