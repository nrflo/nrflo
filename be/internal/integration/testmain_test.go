package integration

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/argon2"

	"be/internal/db"
)

// templateDBPath is a pre-migrated SQLite file shared across all tests.
// Tests copy it to a fresh temp path instead of running migrations per-test.
var templateDBPath string

func TestMain(m *testing.M) {
	// Create template DB with all migrations applied once.
	tmpDir, err := os.MkdirTemp("", "nrf-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	templateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(templateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	// Replace the seeded admin's full-strength hash with a fast one so tests that
	// call loginAdminClient don't spend 200ms+ per Argon2id verification.
	if err := patchFastAdminHash(templateDBPath); err != nil {
		panic("failed to patch admin hash: " + err.Error())
	}

	os.Exit(m.Run())
}

// patchFastAdminHash replaces the seeded admin user's password hash with a minimal-param
// Argon2id hash (m=4096, t=1, p=1) so tests that login via HTTP don't pay 200ms+ per call.
// auth.Verify reads params from the hash string, so this is transparent to the auth layer.
func patchFastAdminHash(dbPath string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := argon2.IDKey([]byte("nrfloAdmin"), salt, 1, 4096, 1, 32)
	enc := base64.RawStdEncoding
	hash := fmt.Sprintf("$argon2id$v=19$m=4096,t=1,p=1$%s$%s",
		enc.EncodeToString(salt), enc.EncodeToString(key))
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()
	_, err = database.Exec(`UPDATE users SET password_hash = ? WHERE email = 'admin@nrflo.com'`, hash)
	return err
}

// copyTemplateDB copies the template DB to dst and returns the path.
// Much faster than running migrations from scratch.
func copyTemplateDB(dst string) error {
	src, err := os.Open(templateDBPath)
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}
