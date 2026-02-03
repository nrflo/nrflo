package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nrworkflow/internal/api"
	"nrworkflow/internal/config"
	"nrworkflow/internal/db"
	"nrworkflow/internal/socket"

	"github.com/spf13/cobra"
)

var (
	servePort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the nrworkflow server (HTTP + Unix socket)",
	Long: `Start the nrworkflow server for the ticket management system.

The server provides:
  - HTTP API on port 6587 (for web UI)
  - Unix socket at /tmp/nrworkflow/nrworkflow.sock (for CLI)

All CLI commands communicate with this server via the Unix socket.
The server must be running for CLI commands to work.

Example usage:
  nrworkflow serve              # Start on default port (6587)
  nrworkflow serve --port=8080  # Start HTTP on custom port`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		// Override port if specified via flag
		if servePort != 0 {
			cfg.Server.Port = servePort
		}

		// Create database connection pool
		pool, err := db.NewPool(DataPath, db.DefaultPoolConfig())
		if err != nil {
			return fmt.Errorf("failed to create database pool: %w", err)
		}
		defer pool.Close()

		// Create and start Unix socket server
		socketServer := socket.NewServer(pool)
		if err := socketServer.Start(); err != nil {
			return fmt.Errorf("failed to start socket server: %w", err)
		}

		// Create HTTP server
		httpServer := api.NewServer(cfg, DataPath)

		// Handle graceful shutdown
		shutdown := make(chan os.Signal, 1)
		signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

		// Start HTTP server in a goroutine
		serverError := make(chan error, 1)
		go func() {
			serverError <- httpServer.Start(cfg.Server.Port)
		}()

		fmt.Printf("nrworkflow server started\n")
		fmt.Printf("  HTTP API:     http://localhost:%d\n", cfg.Server.Port)
		fmt.Printf("  Unix socket:  %s\n", socketServer.SocketPath())
		fmt.Printf("  Database:     %s\n", pool.Path)
		fmt.Println()

		// Wait for shutdown signal or server error
		select {
		case err := <-serverError:
			if err != nil {
				return fmt.Errorf("HTTP server error: %w", err)
			}
		case sig := <-shutdown:
			fmt.Printf("\nReceived %v, shutting down...\n", sig)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Stop socket server
		if err := socketServer.Stop(ctx); err != nil {
			fmt.Printf("Socket server shutdown error: %v\n", err)
		}

		// Stop HTTP server
		if err := httpServer.Stop(ctx); err != nil {
			return fmt.Errorf("HTTP server shutdown error: %w", err)
		}

		fmt.Println("Server stopped gracefully")
		return nil
	},
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", 0, "HTTP port to listen on (default: 6587 or from config)")
}
