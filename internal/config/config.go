// Package config loads runtime configuration from environment variables.
package config

import "os"

// Config holds the runtime configuration for the st0r4g9 server.
type Config struct {
	// Addr is the TCP address the HTTP server listens on (e.g. ":9000").
	Addr string
	// DBPath is the filesystem path to the SQLite database file.
	DBPath string
	// DataDir is the root directory under which object blobs are stored.
	DataDir string
	// LogLevel is the minimum log level: debug, info, warn, or error.
	LogLevel string
	// LogFormat selects the log output format: "text" or "json".
	LogFormat string
}

// Load reads configuration from the environment, applying sane defaults.
//
//	ST0R4G9_ADDR        TCP listen address         (default ":9000")
//	ST0R4G9_DB          SQLite database file path  (default "store.db")
//	ST0R4G9_DATA_DIR    object blob root dir       (default "data")
//	ST0R4G9_LOG_LEVEL   debug|info|warn|error      (default "info")
//	ST0R4G9_LOG_FORMAT  text|json                  (default "text")
func Load() Config {
	return Config{
		Addr:      getenv("ST0R4G9_ADDR", ":9000"),
		DBPath:    getenv("ST0R4G9_DB", "store.db"),
		DataDir:   getenv("ST0R4G9_DATA_DIR", "data"),
		LogLevel:  getenv("ST0R4G9_LOG_LEVEL", "info"),
		LogFormat: getenv("ST0R4G9_LOG_FORMAT", "text"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
