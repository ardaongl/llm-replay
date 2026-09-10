package dataset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type Metadata struct {
	SchemaVersion string    `json:"schema_version"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	RecordCount   int       `json:"record_count"`
	Source        string    `json:"source"`
	SHA256        string    `json:"sha256"`
	Tags          []string  `json:"tags,omitempty"`
}

func (m Metadata) Validate() error {
	if m.SchemaVersion != "1" {
		return errors.New("metadata schema_version must be \"1\"")
	}
	if m.Name == "" || m.CreatedAt.IsZero() || m.Source == "" {
		return errors.New("metadata name, created_at, and source are required")
	}
	if m.RecordCount < 0 {
		return errors.New("metadata record_count cannot be negative")
	}
	decoded, err := hex.DecodeString(m.SHA256)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("metadata sha256 must be a 64-character hexadecimal digest")
	}
	return nil
}

func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open dataset for hashing: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash dataset: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func WriteMetadata(path string, metadata Metadata) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

func ReadMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("read metadata: %w", err)
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return Metadata{}, fmt.Errorf("validate metadata: %w", err)
	}
	return metadata, nil
}
