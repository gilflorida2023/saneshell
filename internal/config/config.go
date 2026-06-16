package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Editor   EditorConfig   `toml:"editor"`
	Intel    IntelConfig    `toml:"intel"`
	History  HistoryConfig  `toml:"history"`
	Completion CompletionConfig `toml:"completion"`
}

type EditorConfig struct {
	Mode        string `toml:"mode"`         // "emacs" | "vi"
	Prompt      string `toml:"prompt"`       // template
	GhostColor  string `toml:"ghost_color"`  // ANSI color
	ShowPreview bool   `toml:"show_preview"` // inline preview on Ctrl+X
}

type IntelConfig struct {
	SocketPath string `toml:"socket_path"` // empty = default
	Enabled    bool   `toml:"enabled"`
	TimeoutMs  int    `toml:"timeout_ms"`
}

type HistoryConfig struct {
	Path       string `toml:"path"`
	MaxEntries int    `toml:"max_entries"`
}

type CompletionConfig struct {
	MinScore     float64 `toml:"min_score"`
	MaxItems     int     `toml:"max_items"`
	ShowKind     bool    `toml:"show_kind"`
	FuzzyMatch   bool    `toml:"fuzzy_match"`
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Editor: EditorConfig{
			Mode:        "vi",
			Prompt:      "\033[32m{{.User}}@\033[36m{{.Host}}:\033[34m{{.CWD}}\033[32m$ \033[0m",
			GhostColor:  "\033[90m",
			ShowPreview: true,
		},
		Intel: IntelConfig{
			SocketPath: "",
			Enabled:    false,
			TimeoutMs:  5000,
		},
		History: HistoryConfig{
			Path:       filepath.Join(home, ".local", "share", "saneshell", "history.db"),
			MaxEntries: 100000,
		},
		Completion: CompletionConfig{
			MinScore:   0.1,
			MaxItems:   20,
			ShowKind:   true,
			FuzzyMatch: true,
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "saneshell", "config.toml")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "saneshell", "config.toml")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}