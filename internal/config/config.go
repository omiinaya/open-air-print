package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the server configuration
type Config struct {
	PrinterName   string `yaml:"printer_name"`   // defaults to system default printer
	Interface     string `yaml:"interface"`     // "0.0.0.0" or specific IP
	Port          int    `yaml:"port"`          // IPP port (631)
	MDNSPort      int    `yaml:"mdns_port"`     // 5353 (usually not needed)
	LogLevel      string `yaml:"log_level"`     // "debug", "info", "warn", "error"
	WebUI         bool   `yaml:"web_ui"`        // optional management UI
	WebPort       int    `yaml:"web_port"`      // 8080
	PrinterModel   string `yaml:"printer_model"`  // e.g., "HP LaserJet Pro M404dn"
	PrinterLocation string `yaml:"printer_location"` // e.g., "Office"
	PrinterInfo    string `yaml:"printer_info"`    // description
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		PrinterName:    "AirPrintServer",
		Interface:      "0.0.0.0",
		Port:           631,
		MDNSPort:       5353,
		LogLevel:       "info",
		WebUI:          true,
		WebPort:        8080,
		PrinterModel:   "Generic AirPrint",
		PrinterLocation: "Local",
		PrinterInfo:    "AirPrint Server for Windows",
	}
}

// Load loads config from file, or returns defaults if file doesn't exist
func Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".airprint-server.yaml")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file exists, return defaults
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set derived defaults
	if cfg.Port == 0 {
		cfg.Port = 631
	}

	return cfg, nil
}

// Save writes config to file
func (c *Config) Save(configPath string) error {
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".airprint-server.yaml")
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
