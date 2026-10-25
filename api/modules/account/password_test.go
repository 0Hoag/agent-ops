package account

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndValidatePassword(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	require.NoError(t, err)
	require.NotEqual(t, password, hash)
	require.True(t, strings.HasPrefix(hash, "$2"), "password should be stored as bcrypt hash")
	require.NoError(t, ValidatePassword(password, hash))
	require.ErrorIs(t, ValidatePassword("wrong password", hash), ErrInvalidCredentials)
}

func TestHashPasswordRejectsEmptyPassword(t *testing.T) {
	_, err := HashPassword("")
	require.EqualError(t, err, "password is required")
}

func TestValidatePasswordRejectsMalformedInputs(t *testing.T) {
	for _, test := range []struct{ password, hash string }{{"", "hash"}, {"password", ""}, {"password", "not-bcrypt"}} {
		require.ErrorIs(t, ValidatePassword(test.password, test.hash), ErrInvalidCredentials)
	}
}
