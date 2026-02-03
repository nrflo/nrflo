package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Generator creates unique IDs with a prefix
type Generator struct {
	prefix string
}

// New creates a new ID generator with the given prefix
func New(prefix string) *Generator {
	return &Generator{prefix: strings.ToLower(prefix)}
}

// Generate creates a new unique ID in the format prefix-xxx
func (g *Generator) Generate() (string, error) {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return fmt.Sprintf("%s-%s", g.prefix, hex.EncodeToString(bytes)), nil
}

// GetPrefix returns the current prefix
func (g *Generator) GetPrefix() string {
	return g.prefix
}
