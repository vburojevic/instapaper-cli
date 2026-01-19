package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type User struct {
	UserID   int64  `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
}

type Defaults struct {
	ListLimit       int    `json:"list_limit,omitempty"`
	ResolveFinalURL *bool  `json:"resolve_final_url,omitempty"`
	Format          string `json:"format,omitempty"`
}

func (d Defaults) ResolveFinalURLValue() bool {
	if d.ResolveFinalURL == nil {
		return true
	}
	return *d.ResolveFinalURL
}

type Config struct {
	APIBase          string   `json:"api_base,omitempty"`
	ConsumerKey      string   `json:"consumer_key,omitempty"`
	ConsumerSecret   string   `json:"consumer_secret,omitempty"`
	OAuthToken       string   `json:"oauth_token,omitempty"`
	OAuthTokenSecret string   `json:"oauth_token_secret,omitempty"`
	User             User     `json:"user,omitempty"`
	Defaults         Defaults `json:"defaults,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		APIBase: DefaultBaseURL(),
		Defaults: Defaults{
			ListLimit: 0,
			Format:    "ndjson",
		},
	}
}

func Load(path string) (*Config, error) {
	c := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	// Fill defaults if missing
	if c.APIBase == "" {
		c.APIBase = DefaultBaseURL()
	}
	if c.Defaults.ListLimit < 0 {
		c.Defaults.ListLimit = 0
	}
	if c.Defaults.Format == "" {
		c.Defaults.Format = "ndjson"
	}
	return c, nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		return errors.New("config path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	// Windows can't replace existing files via rename.
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func (c *Config) HasAuth() bool {
	return c.OAuthToken != "" && c.OAuthTokenSecret != ""
}

func (c *Config) ClearAuth() {
	c.OAuthToken = ""
	c.OAuthTokenSecret = ""
	c.User = User{}
}
