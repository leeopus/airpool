package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen            string `toml:"listen"`
	DBPath            string `toml:"db_path"`
	APIToken          string `toml:"api_token"`
	SubscribeToken    string `toml:"subscribe_token"`
	Hysteria2Password string `toml:"hysteria2_password"`
	CheckInterval     int    `toml:"check_interval"`
	CheckTimeout      int    `toml:"check_timeout"`
	OfflineThreshold  int    `toml:"offline_threshold"`
	TelegramBotToken  string `toml:"telegram_bot_token"`
	TelegramChatID    string `toml:"telegram_chat_id"`
	AlertEmail        string `toml:"alert_email"`
}

func DefaultConfig() *Config {
	return &Config{
		Listen:           ":8080",
		DBPath:           "airpool.db",
		CheckInterval:    60,
		CheckTimeout:     5,
		OfflineThreshold: 3,
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg.APIToken = generateToken("ak_")
		cfg.SubscribeToken = generateToken("st_")
		cfg.Hysteria2Password = generateToken("hp_")
		if err := save(path, cfg); err != nil {
			return nil, fmt.Errorf("create config: %w", err)
		}
		fmt.Println("=== AirPool 首次初始化 ===")
		fmt.Printf("API Token:       %s\n", cfg.APIToken)
		fmt.Printf("Subscribe Token: %s\n", cfg.SubscribeToken)
		fmt.Printf("Hy2 Password:    %s\n", cfg.Hysteria2Password)
		fmt.Println("==========================")
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// GenerateToken creates a new random token with the given prefix.
func GenerateToken(prefix string) string {
	return generateToken(prefix)
}

// Save writes the config to disk.
func Save(path string, cfg *Config) error {
	return save(path, cfg)
}

func save(path string, cfg *Config) error {
	if dir := filepath.Dir(path); dir != "." {
		os.MkdirAll(dir, 0755)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func generateToken(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
