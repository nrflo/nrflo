package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ctxKey struct{}

var (
	mu         sync.Mutex
	writer     io.Writer = os.Stderr
	logFile    *os.File
	logPath    string
	maxLogSize int64 = 10 * 1024 * 1024 // 10MB
)

// Init sets up the logger to write to both the given file path and stderr.
// Creates parent directories if needed.
func Init(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	mu.Lock()
	logFile = f
	logPath = path
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

	prefix := fmt.Sprintf("%s %s [%s] ", ts, level, trx)

	var suffix string
	for i := 0; i+1 < len(args); i += 2 {
		suffix += fmt.Sprintf(" %v=%v", args[i], args[i+1])
	}

	// Split multi-line messages so each line gets the full prefix
	lines := strings.Split(msg, "\n")
	var out string
	for _, l := range lines {
		out += prefix + l + suffix + "\n"
	}

	mu.Lock()
	fmt.Fprint(writer, out)
	if logFile != nil {
		rotate()
	}
	mu.Unlock()
}

// rotate checks if the current log file exceeds maxLogSize and rotates it.
// Must be called while mu is held.
func rotate() {
	info, err := logFile.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: stat log file: %v\n", err)
		return
	}
	if info.Size() < maxLogSize {
		return
	}

	logFile.Close()

	dir := filepath.Dir(logPath)
	archiveName := time.Now().Format("20060102_150405") + ".log"
	archivePath := filepath.Join(dir, archiveName)

	if err := os.Rename(logPath, archivePath); err != nil {
		fmt.Fprintf(os.Stderr, "logger: rename log file: %v\n", err)
		// Try to reopen the original path
		f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if openErr != nil {
			fmt.Fprintf(os.Stderr, "logger: reopen log file after failed rename: %v\n", openErr)
			return
		}
		logFile = f
		writer = io.MultiWriter(f, os.Stderr)
		return
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: open new log file: %v\n", err)
		return
	}
	logFile = f
	writer = io.MultiWriter(f, os.Stderr)
}
