package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"be/internal/api"
	"be/internal/clock"
	"be/internal/config"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/socket"

	"github.com/spf13/cobra"
)

var (
	servePort int
	noTray    bool
)

// serverComponents holds initialized server components for startup/shutdown.
type serverComponents struct {
	cfg          *config.Config
	pool         *db.Pool
	httpServer   *api.Server
	socketServer *socket.Server
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the nrflow server",
	Long: `Start the nrflow server for the ticket management system.

The server provides:
  - HTTP API on port 6587 (for web UI and REST clients)
  - Unix socket for agent communication (findings, completion)
  - macOS menu bar tray icon (disable with --no-tray)

Database migrations are applied automatically on startup.

Example usage:
  nrflow_server serve              # Start with tray icon
  nrflow_server serve --no-tray    # Start headless
  nrflow_server serve --port=8080  # Custom port`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sc, err := setupServer()
		if err != nil {
			return err
		}
		defer sc.pool.Close()

		if noTray || !trayAvailable {
			return runServer(sc)
		}

		// Tray mode: systray.Run() blocks the main thread (macOS requirement).
		// Signal handling is done by the tray, not runServer.
		var serverErr error
		runWithTray(sc.cfg.Server.Port, func() {
			serverError := make(chan error, 1)
			go func() {
				serverError <- sc.httpServer.Start(sc.cfg.Server.Port)
			}()
			ctx := context.Background()
			logger.Info(ctx, "nrflow server started", "port", sc.cfg.Server.Port, "db", sc.pool.Path)
			if err := <-serverError; err != nil {
				serverErr = err
			}
		}, func() {
			shutdownServer(sc)
		})
		return serverErr
	},
}

func setupServer() (*serverComponents, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	dataDir := db.DefaultDataDir()
	if DataPath != "" {
		dataDir = filepath.Dir(DataPath)
	}
	logsDir := filepath.Join(dataDir, "logs")
	if err := logger.Init(filepath.Join(logsDir, "be.log")); err != nil {
		return nil, fmt.Errorf("failed to init logger: %w", err)
	}

	if servePort != 0 {
		cfg.Server.Port = servePort
	}

	migrateDB, err := db.Open(DataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database for migration: %w", err)
	}
	if err := db.RunMigrations(migrateDB.DB); err != nil {
		migrateDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	migrateDB.Close()

	pool, err := db.NewPool(DataPath, db.DefaultPoolConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}

	httpServer := api.NewServer(cfg, DataPath, logsDir, pool)

	clk := clock.Real()
	socketServer := socket.NewServerWithHub(pool, httpServer.GetWSHub(), clk)
	if err := socketServer.Start(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to start socket server: %w", err)
	}

	return &serverComponents{
		cfg:          cfg,
		pool:         pool,
		httpServer:   httpServer,
		socketServer: socketServer,
	}, nil
}

func runServer(sc *serverComponents) error {
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	serverError := make(chan error, 1)
	go func() {
		serverError <- sc.httpServer.Start(sc.cfg.Server.Port)
	}()

	ctx := context.Background()
	logger.Info(ctx, "nrflow server started", "port", sc.cfg.Server.Port, "db", sc.pool.Path)

	select {
	case err := <-serverError:
		if err != nil {
			return fmt.Errorf("HTTP server error: %w", err)
		}
	case sig := <-shutdown:
		logger.Info(ctx, "received signal, shutting down", "signal", sig)
	}

	shutdownServer(sc)
	return nil
}

func shutdownServer(sc *serverComponents) {
	ctx := context.Background()
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := sc.socketServer.Stop(shutdownCtx); err != nil {
		logger.Error(ctx, "socket server shutdown error", "error", err)
	}
	if err := sc.httpServer.Stop(shutdownCtx); err != nil {
		logger.Error(ctx, "HTTP server shutdown error", "error", err)
	}
	logger.Info(ctx, "server stopped gracefully")
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 0, "HTTP port to listen on (default: 6587 or from config)")
	serveCmd.Flags().BoolVar(&noTray, "no-tray", false, "Disable macOS menu bar tray icon")
}
