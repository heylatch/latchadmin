package selfupdate

import (
	"os"
	"testing"
)

// TestSignedFile checks a SHA256SUMS signed by tools/sign with the real key. Set LATCHADMIN_SIGNED_SUMS to its path.
func TestSignedFile(t *testing.T) {
	p := os.Getenv("LATCHADMIN_SIGNED_SUMS")
	if p == "" {
		t.Skip("LATCHADMIN_SIGNED_SUMS not set")
	}
	sums, _ := os.ReadFile(p)
	sig, _ := os.ReadFile(p + ".sig")
	if _, err := VerifySums(sums, sig, "9.9.9", "latchadmin-linux-amd64"); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySums(sums, sig, "9.9.8", "latchadmin-linux-amd64"); err == nil {
		t.Fatal("accepted for another version")
	}
	tampered := append([]byte{}, sums...)
	tampered[len(tampered)-2] ^= 1
	if _, err := VerifySums(tampered, sig, "9.9.9", "latchadmin-linux-amd64"); err == nil {
		t.Fatal("accepted a tampered file")
	}
}
