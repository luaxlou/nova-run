package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort    = 32102
	DefaultTCPPort = 32101
	ConfigDir      = ".nova"
	ConfigName     = "server.yaml"
)

type ServerConfig struct {
	Port    int    `yaml:"port"`
	TCPPort int    `yaml:"tcp_port"`
	APIKey  string `yaml:"api_key"`
	DataDir string `yaml:"data_dir"`
}

func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigDir, ConfigName), nil
}

func GetDefaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ConfigDir, "data")
}

func LoadOrInitServerConfig() (*ServerConfig, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// Try to read existing config
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		var cfg ServerConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("config file exists but api_key is missing")
		}
		if cfg.Port == 0 {
			cfg.Port = DefaultPort
		}
		if cfg.TCPPort == 0 {
			cfg.TCPPort = DefaultTCPPort
		}
		if cfg.DataDir == "" {
			cfg.DataDir = GetDefaultDataDir()
		}
		return &cfg, nil
	}

	// Init new config
	fmt.Printf("Config not found, initializing at %s\n", configPath)
	apiKey, err := generateRandomKey(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate api key: %w", err)
	}

	cfg := &ServerConfig{
		Port:    DefaultPort,
		TCPPort: DefaultTCPPort,
		APIKey:  apiKey,
		DataDir: GetDefaultDataDir(),
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Initialized new configuration.\nAPI Key: %s\n", apiKey)
	return cfg, nil
}

func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
