package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func ComparePassword(hashed, password string) error {
	if hashed == "" {
		return fmt.Errorf("no stored password hash")
	}
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}
