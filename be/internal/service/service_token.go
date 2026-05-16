package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/repo"
)

const (
	serviceTokenPrefix = "nrf_"
	serviceTokenBodyLen = 32
	maxServiceTokenNameLen = 64
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// ServiceTokenService owns generation, hashing, and lifecycle for service tokens.
type ServiceTokenService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewServiceTokenService creates a new service token service.
func NewServiceTokenService(pool *db.Pool, clk clock.Clock) *ServiceTokenService {
	return &ServiceTokenService{pool: pool, clock: clk}
}

// Create generates a new service token, persists its hash, and returns the
// model row alongside the one-time plaintext. The plaintext is never stored.
func (s *ServiceTokenService) Create(projectID, name, createdBy string) (*model.ServiceToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("name is required")
	}
	if len(name) > maxServiceTokenNameLen {
		return nil, "", fmt.Errorf("name exceeds maximum length of %d", maxServiceTokenNameLen)
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, "", fmt.Errorf("project_id is required")
	}

	plaintext, err := generateServiceToken()
	if err != nil {
		return nil, "", err
	}
	hash := sha256Hex(plaintext)

	tok := &model.ServiceToken{
		ID:          uuid.NewString(),
		ProjectID:   strings.ToLower(projectID),
		Name:        name,
		TokenHash:   hash,
		DisplayHint: displayHint(plaintext),
		CreatedBy:   createdBy,
	}

	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	if err := r.Create(tok); err != nil {
		return nil, "", err
	}
	return tok, plaintext, nil
}

// List returns every service token across projects, newest first.
func (s *ServiceTokenService) List() ([]*model.ServiceToken, error) {
	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	return r.ListAll()
}

// ListByProject returns service tokens scoped to one project.
func (s *ServiceTokenService) ListByProject(projectID string) ([]*model.ServiceToken, error) {
	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	return r.ListByProject(projectID)
}

// Delete removes a service token by id.
func (s *ServiceTokenService) Delete(id string) error {
	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	return r.Delete(id)
}

// LookupByPlaintext hashes the supplied plaintext token and returns the matching
// row, or (nil, nil) if no token matches. On hit, last_used_at is updated in a
// background goroutine; lookup errors from the touch are ignored.
func (s *ServiceTokenService) LookupByPlaintext(plaintext string) (*model.ServiceToken, error) {
	if plaintext == "" || !strings.HasPrefix(plaintext, serviceTokenPrefix) {
		return nil, nil
	}
	hash := sha256Hex(plaintext)
	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	tok, err := r.GetByHash(hash)
	if err != nil || tok == nil {
		return nil, err
	}
	go func(id string) {
		_ = repo.NewServiceTokenRepo(s.pool, s.clock).TouchLastUsed(id)
	}(tok.ID)
	return tok, nil
}

// Get returns a token by id, or (nil, nil) if missing.
func (s *ServiceTokenService) Get(id string) (*model.ServiceToken, error) {
	r := repo.NewServiceTokenRepo(s.pool, s.clock)
	tokens, err := r.ListAll()
	if err != nil {
		return nil, err
	}
	for _, t := range tokens {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, nil
}

func generateServiceToken() (string, error) {
	body, err := randomBase62(serviceTokenBodyLen)
	if err != nil {
		return "", err
	}
	return serviceTokenPrefix + body, nil
}

func randomBase62(n int) (string, error) {
	max := big.NewInt(int64(len(base62Alphabet)))
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = base62Alphabet[v.Int64()]
	}
	return string(buf), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// displayHint returns a short, non-secret string a user can recognise. The body
// is at least 32 chars so first-4 + last-4 leaks ~48 bits — safe.
func displayHint(plaintext string) string {
	body := strings.TrimPrefix(plaintext, serviceTokenPrefix)
	if len(body) < 8 {
		return serviceTokenPrefix + "…"
	}
	return serviceTokenPrefix + body[:4] + "…" + body[len(body)-4:]
}
