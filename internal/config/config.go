package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "openrouter" | "anthropic" | "ollama" | "openai_compat"
	BaseURL string            `json:"base_url,omitempty"`
	APIKey  string            `json:"api_key,omitempty"`
	Model   string            `json:"model"`
	Extra   map[string]string `json:"extra,omitempty"`
}

type RouterConfig struct {
	ClarifyProvider string            `json:"clarify_provider"`
	Routes          map[string]string `json:"routes"`
}

type Config struct {
	Providers  []ProviderConfig `json:"providers"`
	Router     RouterConfig     `json:"router"`
	EditorCmd  string           `json:"editor"`
	HistoryDir string           `json:"history_dir,omitempty"`
}

func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiflow")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aiflow")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found at %s\nRun: aiflow init", path)
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	for i := range cfg.Providers {
		if key := cfg.Providers[i].APIKey; len(key) > 0 && key[0] == '$' {
			cfg.Providers[i].APIKey = os.Getenv(key[1:])
		}
	}
	return &cfg, nil
}

func (c *Config) GetEditor() string {
	if c.EditorCmd != "" {
		return c.EditorCmd
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "nvim"
}

func (c *Config) FindProvider(name string) (*ProviderConfig, error) {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider %q not found in config", name)
}

func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(), data, 0600)
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		EditorCmd:  "",
		HistoryDir: filepath.Join(home, ".local", "share", "aiflow", "history"),
		Providers: []ProviderConfig{
			{
				Name:    "deepseek",
				Type:    "openrouter",
				APIKey:  "$OPENROUTER_API_KEY",
				Model:   "deepseek/deepseek-chat",
				BaseURL: "https://openrouter.ai/api/v1",
			},
			{
				Name:    "gemini",
				Type:    "openrouter",
				APIKey:  "$OPENROUTER_API_KEY",
				Model:   "google/gemini-2.0-flash-001",
				BaseURL: "https://openrouter.ai/api/v1",
			},
			{
				Name:    "qwen",
				Type:    "openrouter",
				APIKey:  "$OPENROUTER_API_KEY",
				Model:   "qwen/qwen-2.5-coder-32b-instruct",
				BaseURL: "https://openrouter.ai/api/v1",
			},
			{
				Name:    "claude",
				Type:    "anthropic",
				APIKey:  "$ANTHROPIC_API_KEY",
				Model:   "claude-sonnet-4-20250514",
				BaseURL: "https://api.anthropic.com",
			},
		},
		Router: RouterConfig{
			ClarifyProvider: "deepseek",
			Routes: map[string]string{
				"text":         "gemini",
				"code":         "qwen",
				"quality_code": "claude",
			},
		},
	}
}
