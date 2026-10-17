package secrets

import (
	"strings"
	"testing"
)

func TestBoundEncryption(t *testing.T) {
	b, err := New(strings.Repeat("ab", 32), "test")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal("workspace_a/credential_a", []byte("sentinel-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "sentinel-secret") {
		t.Fatal("plaintext stored")
	}
	plain, err := b.Open("workspace_a/credential_a", "test", sealed)
	if err != nil || string(plain) != "sentinel-secret" {
		t.Fatal(err)
	}
	if _, err = b.Open("workspace_b/credential_a", "test", sealed); err == nil {
		t.Fatal("scope not authenticated")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = b.Open("workspace_a/credential_a", "test", sealed); err == nil {
		t.Fatal("tamper accepted")
	}
}
