package account

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

var ErrInvalidCredentials = errors.New("invalid credentials")

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func ValidatePassword(password, passwordHash string) error {
	if password == "" || passwordHash == "" {
		return ErrInvalidCredentials
	}
	if bcryptCompare(password, passwordHash) != nil {
		return ErrInvalidCredentials
	}
	return nil
}

var bcryptCompare = func(password, passwordHash string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}
