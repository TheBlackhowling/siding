// Generate an Ed25519 keypair for signing release checksums.txt.
//
//   go run ./scripts/gen-checksum-keypair -out .docker/checksum-sign
//
// Without local Go, from the repo root:
//
//   .\docker\dev.ps1 gen-checksum-key
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	outDir := flag.String("out", "", "write seed and public key files here (created if missing)")
	flag.Parse()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	seedHex := hex.EncodeToString(priv.Seed())
	pubHex := hex.EncodeToString(pub)

	fmt.Println("SIDING_CHECKSUM_SIGN_SEED=" + seedHex)
	fmt.Println("PUB=" + pubHex)

	if *outDir == "" {
		return
	}
	if err := os.MkdirAll(*outDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	seedPath := filepath.Join(*outDir, "checksum-sign.seed")
	pubPath := filepath.Join(*outDir, "checksum-sign.pub.hex")
	readmePath := filepath.Join(*outDir, "README.txt")

	if err := os.WriteFile(seedPath, []byte(seedHex+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(pubPath, []byte(pubHex+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	readme := `Checksum signing keypair (do not commit the seed).

checksum-sign.seed     → GitHub Actions secret SIDING_CHECKSUM_SIGN_SEED (64 hex chars, no newline in the UI)
checksum-sign.pub.hex  → update internal/update/checksums_sig.go mustDecodePubKey("...")

Delete this folder when the secret and embedded public key are in place.
`
	if err := os.WriteFile(readmePath, []byte(readme), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", filepath.ToSlash(seedPath))
	fmt.Println("wrote", filepath.ToSlash(pubPath))
	fmt.Println("wrote", filepath.ToSlash(readmePath))
}
