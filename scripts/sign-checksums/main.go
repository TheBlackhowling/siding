// Signs dist/checksums.txt for GitHub Releases (SIDING_CHECKSUM_SIGN_SEED in CI).
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: sign-checksums <path/to/checksums.txt>")
		os.Exit(2)
	}
	seedHex := os.Getenv("SIDING_CHECKSUM_SIGN_SEED")
	if seedHex == "" {
		fmt.Fprintln(os.Stderr, "SIDING_CHECKSUM_SIGN_SEED is not set")
		os.Exit(1)
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		fmt.Fprintln(os.Stderr, "SIDING_CHECKSUM_SIGN_SEED must be 64 hex chars (32-byte seed)")
		os.Exit(1)
	}
	checksumsPath := os.Args[1]
	body, err := os.ReadFile(checksumsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	sig := ed25519.Sign(priv, body)
	sigPath := checksumsPath + ".sig"
	if err := os.WriteFile(sigPath, sig, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(filepath.Base(sigPath))
}
