package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ardao/llm-replay/internal/domain"
)

// Writer appends complete JSONL records safely across concurrent goroutines.
type Writer struct {
	mu     sync.Mutex
	file   *os.File
	closed bool
}

func NewWriter(path string) (*Writer, error) {
	if path == "" {
		return nil, errors.New("dataset path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create dataset directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open dataset writer: %w", err)
	}
	return &Writer{file: file}, nil
}

// Append validates and writes exactly one complete JSONL line.
func (w *Writer) Append(record domain.Record) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("validate record: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	data = append(data, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("dataset writer is closed")
	}
	n, err := w.file.Write(data)
	if err != nil {
		return fmt.Errorf("append record: %w", err)
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// Flush requests that appended data be persisted to stable storage.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("dataset writer is closed")
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("flush dataset: %w", err)
	}
	return nil
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true

	syncErr := w.file.Sync()
	closeErr := w.file.Close()
	if syncErr != nil {
		return fmt.Errorf("flush dataset before close: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close dataset: %w", closeErr)
	}
	return nil
}
