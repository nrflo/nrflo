package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ctxKey struct{}

var (
	mu     sync.Mutex
	writer io.Writer = os.Stderr
)

// Init sets up the logger to write to both the given file path and stderr.
// Creates parent directories if needed.
func Init(logPath string) error {
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	mu.Lock()
	writer = io.MultiWriter(f, os.Stderr)
	mu.Unlock()
	return nil
}

// NewTrx generates a short transaction ID (8-char hex string).
func NewTrx() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// WithTrx stores a trx ID in the context.
func WithTrx(ctx context.Context, trx string) context.Context {
	return context.WithValue(ctx, ctxKey{}, trx)
}

// TrxFromContext retrieves the trx ID from the context.
// Returns "-" if no trx is set.
func TrxFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok && v != "" {
		return v
	}
	return "-"
}

// Info logs a message at INFO level.
func Info(ctx context.Context, msg string, args ...any) {
	log("INFO", ctx, msg, args...)
}

// Warn logs a message at WARN level.
func Warn(ctx context.Context, msg string, args ...any) {
	log("WARN", ctx, msg, args...)
}

// Error logs a message at ERROR level.
func Error(ctx context.Context, msg string, args ...any) {
	log("ERROR", ctx, msg, args...)
}

// GetWriter returns the current writer (for testing)
func GetWriter() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	return writer
}

// SetWriter sets the writer (for testing)
func SetWriter(w io.Writer) {
	mu.Lock()
	writer = w
	mu.Unlock()
}

func log(level string, ctx context.Context, msg string, args ...any) {
	trx := TrxFromContext(ctx)
	ts := time.Now().Format("2006-01-02 15:04:05")

	line := fmt.Sprintf("%s %s [%s] %s", ts, level, trx, msg)
	for i := 0; i+1 < len(args); i += 2 {
		line += fmt.Sprintf(" %v=%v", args[i], args[i+1])
	}
	line += "\n"

	mu.Lock()
	fmt.Fprint(writer, line)
	mu.Unlock()
}
