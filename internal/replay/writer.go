package replay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type resultWriter struct {
	file   *os.File
	closed bool
}

func newResultWriter(path string) (*resultWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create replay results: %w", err)
	}
	return &resultWriter{file: file}, nil
}

func (w *resultWriter) Append(result Result) error {
	if w.closed {
		return errors.New("replay result writer is closed")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode replay result: %w", err)
	}
	data = append(data, '\n')
	n, err := w.file.Write(data)
	if err != nil {
		return fmt.Errorf("append replay result: %w", err)
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func (w *resultWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	syncErr := w.file.Sync()
	closeErr := w.file.Close()
	if syncErr != nil {
		return fmt.Errorf("flush replay results: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close replay results: %w", closeErr)
	}
	return nil
}
