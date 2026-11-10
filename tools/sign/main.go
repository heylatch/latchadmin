// Command sign signs a release's SHA256SUMS in CI. The key is the base64 Ed25519 seed in LATCHADMIN_SIGNING_KEY
// (a GitHub Actions secret); it is never stored in this repository.
//
//	go run ./tools/sign dist/SHA256SUMS   → writes dist/SHA256SUMS.sig
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/heylatch/latchadmin/internal/selfupdate"
)

func main() {
	if len(os.Args) != 2 {
		fail("usage: sign <file>")
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("LATCHADMIN_SIGNING_KEY")))
	if err != nil || len(seed) != ed25519.SeedSize {
		fail("LATCHADMIN_SIGNING_KEY must be a base64 32-byte Ed25519 seed")
	}
	key := ed25519.NewKeyFromSeed(seed)
	pub, _ := base64.StdEncoding.DecodeString(selfupdate.PublicKey)
	if !key.Public().(ed25519.PublicKey).Equal(ed25519.PublicKey(pub)) {
		fail("the signing key doesn't match the public key built into LatchAdmin")
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err.Error())
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data))
	if err := os.WriteFile(os.Args[1]+".sig", []byte(sig+"\n"), 0o644); err != nil {
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
