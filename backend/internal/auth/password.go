// Package auth hashes and verifies account passwords with Argon2id.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// OWASP-recommended Argon2id baseline: 19 MiB, 2 passes, 1 lane.
const (
	memKiB  = 19 * 1024
	passes  = 2
	lanes   = 1
	keyLen  = 32
	saltLen = 16
)

// slots bounds concurrent hashing so a burst of logins cannot exhaust memory.
var slots = make(chan struct{}, 4)

func derive(password string, salt []byte, m, t uint32, p uint8, n uint32) []byte {
	slots <- struct{}{}
	defer func() { <-slots }()
	return argon2.IDKey([]byte(password), salt, t, m, p, n)
}

// Hash returns a self-describing encoded hash with a fresh random salt.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := derive(password, salt, memKiB, passes, lanes, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memKiB, passes, lanes, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify checks a password against an encoded hash in constant time.
func Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m > 64*1024 || t > 8 || p == 0 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 64 {
		return false
	}
	got := derive(password, salt, m, t, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummy lets a login for an unknown user cost the same as a real one.
var dummy, _ = Hash("timing-equaliser")

func VerifyDummy(password string) { Verify(password, dummy) }
