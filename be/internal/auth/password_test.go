package auth

import (
	"strings"
	"testing"
)


func TestHash_ReturnsPHCFormat(t *testing.T) {
	t.Parallel()
	hash, err := Hash("somepassword")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("hash = %q, want prefix $argon2id$v=19$", hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("PHC parts = %d, want 6 (got %q)", len(parts), hash)
	}
	if parts[1] != "argon2id" {
		t.Errorf("algorithm = %q, want argon2id", parts[1])
	}
	if parts[2] != "v=19" {
		t.Errorf("version = %q, want v=19", parts[2])
	}
}

func TestHash_DifferentSaltsEachCall(t *testing.T) {
	t.Parallel()
	h1, err := Hash("password")
	if err != nil {
		t.Fatalf("Hash first: %v", err)
	}
	h2, err := Hash("password")
	if err != nil {
		t.Fatalf("Hash second: %v", err)
	}
	if h1 == h2 {
		t.Error("Hash() called twice with same password should produce different hashes (different salts)")
	}
}

func TestVerify_RoundTrip(t *testing.T) {
	t.Parallel()
	plain := "my-secret-password"
	hash, err := Hash(plain)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := Verify(hash, plain); err != nil {
		t.Errorf("Verify(hash, plain) = %v, want nil", err)
	}
}

func TestVerify_WrongPassword_ReturnsHashMismatch(t *testing.T) {
	t.Parallel()
	hash, err := Hash("correct-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	err = Verify(hash, "wrong-password")
	if err != ErrHashMismatch {
		t.Errorf("Verify with wrong password = %v, want ErrHashMismatch", err)
	}
}

func TestVerify_TamperedHash_ReturnsHashMismatch(t *testing.T) {
	t.Parallel()
	plain := "testpassword"
	hash, err := Hash(plain)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	// Replace the key segment (after the last $) with a hash of a different
	// password.  The key portion changes reliably without hitting base64 padding
	// edge cases.
	otherHash, err := Hash("a-completely-different-password")
	if err != nil {
		t.Fatalf("Hash other: %v", err)
	}
	// Swap the key segment of hash with the key from otherHash.
	lastDollar := strings.LastIndex(hash, "$")
	otherLastDollar := strings.LastIndex(otherHash, "$")
	tampered := hash[:lastDollar+1] + otherHash[otherLastDollar+1:]

	err = Verify(tampered, plain)
	if err == nil {
		t.Error("Verify with tampered key should fail, got nil")
	}
}

func TestVerify_MalformedHash(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		hash string
	}{
		{"not a PHC string", "not-a-phc"},
		{"empty string", ""},
		{"wrong algorithm", "$bcrypt$v=19$m=65536,t=3,p=2$salt$key"},
		{"too few parts", "$argon2id$v=19$m=65536,t=3,p=2$salt"},
		{"missing leading dollar", "argon2id$v=19$m=65536,t=3,p=2$salt$key"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Verify(tc.hash, "anypassword")
			if err != ErrMalformedHash {
				t.Errorf("Verify(%q) = %v, want ErrMalformedHash", tc.hash, err)
			}
		})
	}
}

// TestSeedAdminHashVerifiesAdmin verifies that the hash seeded in
// migration 000078_seed_admin.up.sql authenticates with the known password.
// This guarantees the seeded admin can log in before HTTP wiring (T2).
func TestSeedAdminHashVerifiesAdmin(t *testing.T) {
	t.Parallel()
	const seedHash = `$argon2id$v=19$m=65536,t=3,p=2$wp1TNUWXyUKgfR4DUO9lNw$I3aPnpvRtOZrJU4VESkpkNfcPpg7sncPAZtxKJZDpm4`
	if err := Verify(seedHash, "admin"); err != nil {
		t.Errorf("Verify(seedHash, \"admin\") = %v, want nil", err)
	}
}

func TestVerify_EmptyPassword_MatchesHashedEmpty(t *testing.T) {
	t.Parallel()
	hash, err := Hash("")
	if err != nil {
		t.Fatalf("Hash empty: %v", err)
	}
	if err := Verify(hash, ""); err != nil {
		t.Errorf("Verify hash of empty string with empty plain = %v, want nil", err)
	}
}
