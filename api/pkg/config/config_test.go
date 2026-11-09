package config

import (
	"strings"
	"testing"
)

func TestLoadEncryptionKeyLength(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantError string
	}{
		{name: "missing key is allowed", wantError: ""},
		{name: "32 byte key", key: strings.Repeat("k", 32), wantError: ""},
		{name: "short key", key: strings.Repeat("k", 31), wantError: "REPOSITORY_ENCRYPTION_KEY must be 32 bytes"},
		{name: "long key", key: strings.Repeat("k", 33), wantError: "REPOSITORY_ENCRYPTION_KEY must be 32 bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JWT_SECRET", "test-jwt-secret")
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("REPOSITORY_ENCRYPTION_KEY", tt.key)

			_, err := Load()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Load() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("Load() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}
