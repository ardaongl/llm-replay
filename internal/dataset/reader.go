package dataset

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ardao/llm-replay/internal/domain"
)

const defaultMaxLineSize = 16 * 1024 * 1024

// LineError identifies the exact JSONL line that could not be read.
type LineError struct {
	Line int
	Err  error
}

func (e *LineError) Error() string {
	return fmt.Sprintf("dataset line %d: %v", e.Line, e.Err)
}

func (e *LineError) Unwrap() error {
	return e.Err
}

// Reader decodes a dataset one line at a time and keeps memory bounded.
type Reader struct {
	scanner *bufio.Scanner
	line    int
}

func NewReader(input io.Reader) *Reader {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), defaultMaxLineSize)
	return &Reader{scanner: scanner}
}

// Next returns the next valid record or io.EOF.
func (r *Reader) Next() (domain.Record, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return domain.Record{}, fmt.Errorf("scan dataset after line %d: %w", r.line, err)
		}
		return domain.Record{}, io.EOF
	}

	r.line++
	data := r.scanner.Bytes()
	if len(data) == 0 {
		return domain.Record{}, &LineError{Line: r.line, Err: errors.New("empty line")}
	}

	var record domain.Record
	if err := json.Unmarshal(data, &record); err != nil {
		return domain.Record{}, &LineError{Line: r.line, Err: fmt.Errorf("decode JSON: %w", err)}
	}
	if err := record.Validate(); err != nil {
		return domain.Record{}, &LineError{Line: r.line, Err: fmt.Errorf("validate record: %w", err)}
	}

	return record, nil
}

// ReadFile streams every record to visit and stops on the first error.
func ReadFile(path string, visit func(domain.Record) error) error {
	if visit == nil {
		return errors.New("dataset visitor is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open dataset: %w", err)
	}
	defer file.Close()

	reader := NewReader(file)
	for {
		record, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return fmt.Errorf("visit dataset record %q: %w", record.ID, err)
		}
	}
}
