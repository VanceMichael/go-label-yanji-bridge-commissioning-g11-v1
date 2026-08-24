package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const passwordCost = 12

func HashPassword(password string) ([]byte, error) {
	if len(password) < 12 || len(password) > 128 {
		return nil, fmt.Errorf("password must be between 12 and 128 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	return hash, nil
}

func VerifyPassword(hash []byte, password string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}
