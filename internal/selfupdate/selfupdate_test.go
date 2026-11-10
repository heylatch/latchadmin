package selfupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestVerifySumsRejectsWrongKeyAndTampering(t *testing.T) {
	sums := []byte("# latchadmin 1.0.0\naa11  latchadmin-windows-amd64.exe\nbb22  latchadmin-linux-amd64\n")
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(other, sums))
	if _, err := VerifySums(sums, []byte(sig), "1.0.0", "latchadmin-linux-amd64"); err == nil {
		t.Fatal("signature from another key was accepted")
	}
	if _, err := VerifySums(sums, []byte("not base64!"), "1.0.0", "latchadmin-linux-amd64"); err == nil {
		t.Fatal("garbage signature was accepted")
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"1.2.3", "1.2.3", 0}, {"1.10.0", "1.9.9", 1}, {"v0.1.0", "0.2.0", -1}, {"1.0.0-beta", "1.0.0", 0}, {"0.0.0-dev", "0.1.0", -1}}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s,%s)=%d", c.a, c.b, got)
		}
	}
}
