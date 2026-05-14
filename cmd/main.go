package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/cheatsnake/airstation/internal/config"
	"github.com/cheatsnake/airstation/internal/http"
	"github.com/cheatsnake/airstation/internal/logger"
	"github.com/cheatsnake/airstation/internal/pkg/fs"
	"github.com/cheatsnake/airstation/internal/storage"
	"github.com/cheatsnake/airstation/internal/storage/postgres"
	"github.com/cheatsnake/airstation/internal/storage/sqlite"
)

func main() {
	conf := config.Load()

	fs.DeleteDirIfExists(conf.TmpDir)
	fs.MustDir(conf.TmpDir)
	fs.MustDir(conf.TracksDir)

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, os.Interrupt, syscall.SIGTERM)

	log := logger.New()
	store, err := openStore(conf, log)
	if err != nil {
		log.Error("Failed connect to database: " + err.Error())
		os.Exit(1)
	}

	httpServer := http.NewServer(store, conf, log)
	go httpServer.Run()

	<-stopSignal
	shutdown(log, store, httpServer)
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

func shutdown(log *slog.Logger, store storage.Storage, httpServer *http.Server) {
	println()
	log.Info("Shutting down the app...")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("HTTP server shutdown failed: " + err.Error())
	}

	if err := store.Close(); err != nil {
		log.Error("Failed to close database connection: " + err.Error())
	}

	log.Info("App gracefully stopped")
}
