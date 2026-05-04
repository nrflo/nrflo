package api

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

// apiTemplateDBPath is a pre-migrated SQLite file shared across all api package tests.
var apiTemplateDBPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nrf-api-template-*")
	if err != nil {
		panic("failed to create template dir: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	apiTemplateDBPath = filepath.Join(tmpDir, "template.db")
	pool, err := db.NewPoolPath(apiTemplateDBPath, db.DefaultPoolConfig())
	if err != nil {
		panic("failed to create template DB: " + err.Error())
	}
	pool.Close()

	// Replace the seeded admin's full-strength hash with a fast one so auth tests
	// don't spend 200ms+ per Argon2id verification.
	if err := patchFastAdminHash(apiTemplateDBPath); err != nil {
		panic("failed to patch admin hash: " + err.Error())
	}

	os.Exit(m.Run())
}

// patchFastAdminHash replaces the seeded admin user's password hash with a minimal-param
// Argon2id hash (m=4096, t=1, p=1) so auth tests don't pay 200ms+ per login call.
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

// apiCopyTemplateDB copies the pre-migrated template DB to dst.
func apiCopyTemplateDB(dst string) error {
	src, err := os.Open(apiTemplateDBPath)
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
