package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen            string `toml:"listen"`
	DBPath            string `toml:"db_path"`
	TLSCert           string `toml:"tls_cert"`
	TLSKey            string `toml:"tls_key"`
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
		Listen:           ":8443",
		DBPath:           "airpool.db",
		TLSCert:          "server.crt",
		TLSKey:           "server.key",
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

// EnsureTLSCert generates a self-signed certificate if cert/key files don't exist.
func EnsureTLSCert(certPath, keyPath string) error {
	if _, err := os.Stat(certPath); err == nil {
		return nil // already exists
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "airpool"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	certFile, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyFile.Close()
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return nil
}
