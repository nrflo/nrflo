package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the nrflow server",
	Long: `Start the nrflow server for the ticket management system.

The server provides:
  - HTTP API on port 6587 (for web UI and REST clients)
  - Unix socket for agent communication (findings, completion)

Database migrations are applied automatically on startup.

Example usage:
  nrflow_server serve              # Start on default port (6587)
  nrflow_server serve --port=8080  # Start HTTP on custom port`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Initialize logger
		if err := logger.Init("/tmp/nrflow/logs/be.log"); err != nil {
			return fmt.Errorf("failed to init logger: %w", err)
		}

		// Override port if specified via flag
		if servePort != 0 {
			cfg.Server.Port = servePort
		}

		// Auto-migrate database
		migrateDB, err := db.Open(DataPath)
		if err != nil {
			return fmt.Errorf("failed to open database for migration: %w", err)
		}
		if err := db.RunMigrations(migrateDB.DB); err != nil {
			migrateDB.Close()
			return fmt.Errorf("failed to run migrations: %w", err)
		}
		migrateDB.Close()

		// Create database connection pool
		pool, err := db.NewPool(DataPath, db.DefaultPoolConfig())
		if err != nil {
			return fmt.Errorf("failed to create database pool: %w", err)
		}
		defer pool.Close()

		// Create HTTP server (creates WebSocket hub)
		httpServer := api.NewServer(cfg, DataPath, pool)

		// Create and start Unix socket server with shared WebSocket hub
		clk := clock.Real()
		socketServer := socket.NewServerWithHub(pool, httpServer.GetWSHub(), clk)
		if err := socketServer.Start(); err != nil {
			return fmt.Errorf("failed to start socket server: %w", err)
		}

		// Handle graceful shutdown
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

		// Start HTTP server in a goroutine
		serverError := make(chan error, 1)
		go func() {
			serverError <- httpServer.Start(cfg.Server.Port)
		}()

		ctx := context.Background()
		logger.Info(ctx, "nrflow server started", "port", cfg.Server.Port, "db", pool.Path)

		// Wait for shutdown signal or server error
		select {
		case err := <-serverError:
			if err != nil {
				return fmt.Errorf("HTTP server error: %w", err)
			}
		case sig := <-shutdown:
			logger.Info(ctx, "received signal, shutting down", "signal", sig)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Stop socket server
		if err := socketServer.Stop(shutdownCtx); err != nil {
			logger.Error(ctx, "socket server shutdown error", "error", err)
		}

		// Stop HTTP server
		if err := httpServer.Stop(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}

		logger.Info(ctx, "server stopped gracefully")
		return nil
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 0, "HTTP port to listen on (default: 6587 or from config)")
}
