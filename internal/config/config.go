package config

import (
	"fmt"
	"os"
)

type Config struct {
	ConsoleURL string // wss://domain/ws/pupp/{id}
	AuthToken  string
	PuppID     string
	ScanrayBin string
	NucleiBin  string
	DataDir    string
}

func Load() (*Config, error) {
	c := &Config{
		ConsoleURL: os.Getenv("PUPP_CONSOLE_URL"),
		AuthToken:  os.Getenv("PUPP_AUTH_TOKEN"),
		PuppID:     os.Getenv("PUPP_ID"),
		ScanrayBin: envOrDefault("SCANRAY_BINARY", "/opt/scanray/bin/scanray"),
		NucleiBin:  envOrDefault("NUCLEI_BINARY", "/opt/scanray/bin/nuclei"),
		DataDir:    envOrDefault("PUPP_DATA_DIR", "/opt/scanray/data"),
	}
	if c.ConsoleURL == "" {
		return nil, fmt.Errorf("PUPP_CONSOLE_URL is required")
	}
	if c.AuthToken == "" {
		return nil, fmt.Errorf("PUPP_AUTH_TOKEN is required")
	}
	if c.PuppID == "" {
		return nil, fmt.Errorf("PUPP_ID is required")
	}
	return c, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
