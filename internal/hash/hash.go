package hash

import (
	"crypto/sha256"
	"encoding/hex"

	"datascape.dev/platformctl/internal/canonical"
)

func Bytes(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func Canonical(value any) (string, error) {
	content, err := canonical.JSON(value)
	if err != nil {
		return "", err
	}
	return Bytes(content), nil
}
