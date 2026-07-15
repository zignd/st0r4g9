// Command st0r4g9 is a simplified, S3-flavored object storage server: buckets,
// objects, metadata, and queryable annotations over a RESTful HTTP API, with
// API-key auth. Object bytes live on the filesystem; everything else in SQLite.
package main

import (
	"log/slog"
	"os"

	"github.com/zignd/st0r4g9/internal/api"
	"github.com/zignd/st0r4g9/internal/config"
	"github.com/zignd/st0r4g9/internal/logging"
	"github.com/zignd/st0r4g9/internal/storage"
	"github.com/zignd/st0r4g9/internal/store"
)

func main() {
	cfg := config.Load()
	log := logging.New(cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(log)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Error("open store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	blobs, err := storage.New(cfg.DataDir)
	if err != nil {
		log.Error("open blob storage", "error", err)
		os.Exit(1)
	}

	app := api.New(st, blobs, log)

	log.Info("st0r4g9 starting",
		"addr", cfg.Addr, "db", cfg.DBPath, "data", cfg.DataDir,
		"logLevel", cfg.LogLevel, "logFormat", cfg.LogFormat)
	if err := app.Listen(cfg.Addr); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
