package controllers

import (
	"github.com/sethvargo/go-password/password"
)

const passwordSizeBytes = 24

// GeneratePassword creates a password consisting of a random array of bytes
func GeneratePassword() []byte {
	return []byte(password.MustGenerate(passwordSizeBytes, 10, 0, false, true))
}
