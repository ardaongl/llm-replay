package arena

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func randomID(prefix string) (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate secure identifier: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(data), nil
}
