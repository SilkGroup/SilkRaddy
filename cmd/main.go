package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/cheatsnake/airstation/internal/config"
	"github.com/cheatsnake/airstation/internal/filestore"
	"github.com/cheatsnake/airstation/internal/filestore/gcs"
	"github.com/cheatsnake/airstation/internal/filestore/local"
	"github.com/cheatsnake/airstation/internal/http"
	"github.com/cheatsnake/airstation/internal/logger"
	"github.com/cheatsnake/airstation/internal/pkg/fs"
	"github.com/cheatsnake/airstation/internal/storage"
	"github.com/cheatsnake/airstation/internal/storage/postgres"
	"github.com/cheatsnake/airstation/internal/storage/sqlite"
)

func main() {
	log := logger.New()

	conf, err := config.Load()
	if err != nil {
		log.Error("Configuration error", "error", err)
		os.Exit(1)
	}

	fs.DeleteDirIfExists(conf.TmpDir)
	fs.MustDir(conf.TmpDir)
	fs.MustDir(conf.TracksDir)

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)

	store, err := openStore(conf, log)
	if err != nil {
		log.Error("Failed connect to database", "error", err)
		os.Exit(1)
	}

	fileStore, err := openFileStore(context.Background(), conf, log)
	if err != nil {
		log.Error("Failed to open file store", "error", err)
		os.Exit(1)
	}

	httpServer := http.NewServer(store, conf, log)
	go httpServer.Run()

	<-stopSignal
	shutdown(log, store, fileStore, httpServer)
}

// openFileStore picks the FileStore backend based on SILKRADDY_FILESTORE_DRIVER.
// Default is "local" so existing self-host deploys are unaffected. The "gcs"
// driver requires SILKRADDY_FILESTORE_BUCKET to be set.
func openFileStore(ctx context.Context, conf *config.Config, log *slog.Logger) (filestore.FileStore, error) {
	switch conf.FileStoreDriver {
	case "", "local":
		log.Info("FileStore: local", "root", conf.TracksDir)
		return local.New(conf.TracksDir, "/static/tracks")
	case "gcs":
		if conf.FileStoreBucket == "" {
			return nil, errors.New("SILKRADDY_FILESTORE_BUCKET is required when SILKRADDY_FILESTORE_DRIVER=gcs")
		}
		log.Info("FileStore: gcs", "bucket", conf.FileStoreBucket)
		return gcs.New(ctx, conf.FileStoreBucket, conf.FileStoreServiceAccount)
	default:
		return nil, fmt.Errorf("unknown SILKRADDY_FILESTORE_DRIVER: %s", conf.FileStoreDriver)
	}
}

// openStore picks the storage backend based on AIRSTATION_DB_DRIVER. The
// default is sqlite (no external dependencies) so existing docker-compose and
// VM deploys keep working. Cloud Run sets AIRSTATION_DB_DRIVER=postgres plus
// AIRSTATION_POSTGRES_URL to point at Cloud SQL.
func openStore(conf *config.Config, log *slog.Logger) (storage.Storage, error) {
	switch conf.DBDriver {
	case "postgres":
		if conf.PostgresURL == "" {
			return nil, errMissingPostgresURL
		}
		return postgres.New(conf.PostgresURL, log.WithGroup("storage"))
	case "sqlite", "":
		fs.MustDir(conf.DBDir)
		return sqlite.New(path.Join(conf.DBDir, conf.DBFile), log.WithGroup("storage"))
	default:
		return nil, &unknownDriverError{driver: conf.DBDriver}
	}
}

var errMissingPostgresURL = &configError{
	msg: "AIRSTATION_POSTGRES_URL is required when AIRSTATION_DB_DRIVER=postgres",
}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

type unknownDriverError struct{ driver string }

func (e *unknownDriverError) Error() string {
	return "unknown AIRSTATION_DB_DRIVER: " + e.driver
}

func shutdown(log *slog.Logger, store storage.Storage, fileStore filestore.FileStore, httpServer *http.Server) {
	println()
	log.Info("Shutting down the app...")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("HTTP server shutdown failed", "error", err)
	}

	if c, ok := fileStore.(interface{ Close() error }); ok {
		if err := c.Close(); err != nil {
			log.Error("Failed to close file store", "error", err)
		}
	}

	if err := store.Close(); err != nil {
		log.Error("Failed to close database connection", "error", err)
	}

	log.Info("App gracefully stopped")
}
