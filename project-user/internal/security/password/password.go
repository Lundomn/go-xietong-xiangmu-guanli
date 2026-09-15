package password

import (
	"crypto/subtle"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"test.com/project-common/encrypts"
)

const bcryptPrefix = "$2"

// Hash creates a deliberately slow password hash suitable for storage.
func Hash(value string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(value), bcrypt.DefaultCost)
	return string(hash), err
}

// Verify accepts bcrypt hashes and temporarily supports the old MD5 format so
// existing installations can migrate on the next successful login.
func Verify(stored, value string) (valid bool, legacy bool) {
	if strings.HasPrefix(stored, bcryptPrefix) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(value)) == nil, false
	}
	legacyHash := encrypts.Md5(value)
	return subtle.ConstantTimeCompare([]byte(stored), []byte(legacyHash)) == 1, true
}
