package crypto

import (
	"strings"
	"testing"
)

func TestSealerEncryptsAndRoundTripsWithoutPlaintext(t *testing.T) {
	sealer, err := New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	plain := "disposable-webhook-secret"
	ciphertext, err := sealer.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == plain || strings.Contains(ciphertext, plain) {
		t.Fatalf("ciphertext contains plaintext: %q", ciphertext)
	}
	got, err := sealer.Open(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("round trip got %q, want %q", got, plain)
	}
	if _, err := sealer.Open(plain); err == nil {
		t.Fatal("plaintext unexpectedly accepted as ciphertext")
	}
}
