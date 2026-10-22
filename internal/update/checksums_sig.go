package update

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
)

const checksumSigAsset = "checksums.txt.sig"

// checksumSignPublicKey verifies checksums.txt on release downloads.
// Regenerate with: go run ./scripts/gen-checksum-keypair
// Store the seed in CI as SIDING_CHECKSUM_SIGN_SEED (never commit it).
var checksumSignPublicKey = mustDecodePubKey("c6af5756b3115bb771a647059681647e7352937f7bc23c1c22b5f4bce4cb581d")

func mustDecodePubKey(hexKey string) ed25519.PublicKey {
	b, err := hex.DecodeString(hexKey)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("update: bad embedded checksum sign public key")
	}
	return ed25519.PublicKey(b)
}

func SetChecksumSignPublicKeyForTest(pub ed25519.PublicKey) func() {
	old := checksumSignPublicKey
	checksumSignPublicKey = pub
	return func() { checksumSignPublicKey = old }
}

func VerifyChecksumsSignature(checksums, sig []byte) error {
	if len(checksumSignPublicKey) != ed25519.PublicKeySize {
		return errors.New("update: checksum signer not configured")
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("update: checksums.txt.sig has length %d, want %d", len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(checksumSignPublicKey, checksums, sig) {
		return errors.New("update: checksums.txt signature invalid")
	}
	return nil
}
