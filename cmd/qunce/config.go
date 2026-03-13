package main

import (
	"os"
	"path/filepath"
	"strings"
)

type appConfig struct {
	AppName          string
	AppVersion       string
	DefaultPairToken string
	ServerDataDir    string
	Host             string
	Port             int
	EmbeddedNodeID   string
}

func loadConfig() appConfig {
	return appConfig{
		AppName:          getenvOrDefault("QUNCE_APP_NAME", "Qunce"),
		AppVersion:       getenvOrDefault("QUNCE_APP_VERSION", "0.1.0"),
		DefaultPairToken: getenvOrDefault("QUNCE_PAIR_TOKEN", "dev-pair-token"),
		ServerDataDir:    expandHome(getenvOrDefault("QUNCE_SERVER_DATA_DIR", "~/.qunce")),
		Host:             getenvOrDefault("QUNCE_SERVER_HOST", "0.0.0.0"),
		Port:             getenvIntOrDefault("QUNCE_SERVER_PORT", 8000),
		EmbeddedNodeID:   getenvOrDefault("QUNCE_SERVER_NODE_ID", ""),
	}
}

func getenvOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func getenvIntOrDefault(key string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue
	}

	for _, char := range raw {
		if char < '0' || char > '9' {
			return defaultValue
		}
	}

	var value int
	for _, char := range raw {
		value = value*10 + int(char-'0')
	}
	return value
}

func expandHome(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" || path == "~" {
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}

	return path
}
