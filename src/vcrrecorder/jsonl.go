package vcrrecorder

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type ReadError struct {
	Line   int
	Offset int64
	Err    error
}

func (e *ReadError) Error() string {
	return fmt.Sprintf("read vcr JSONL at line %d byte %d: %v", e.Line, e.Offset, e.Err)
}

func (e *ReadError) Unwrap() error {
	return e.Err
}

type IntegrityError struct {
	Line     int
	Expected uint64
	Got      uint64
	Err      error
}

func (e *IntegrityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("vcr integrity check failed at line %d: %v", e.Line, e.Err)
	}
	return fmt.Sprintf("vcr integrity check failed at line %d: expected sequence %d, got %d", e.Line, e.Expected, e.Got)
}

func (e *IntegrityError) Unwrap() error {
	return e.Err
}

func ReadJSONL(reader io.Reader, filter Filter) (Recording, error) {
	if reader == nil {
		return Recording{}, errors.New("vcr JSONL reader is required")
	}

	buffered := bufio.NewReader(reader)
	var events []Event
	var offset int64
	lineNumber := 0

	for {
		line, err := buffered.ReadString('\n')
		if len(line) > 0 {
			lineNumber++
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(trimmed) != "" {
				var event Event
				if decodeErr := json.Unmarshal([]byte(trimmed), &event); decodeErr != nil {
					return Recording{}, &ReadError{
						Line:   lineNumber,
						Offset: offset,
						Err:    decodeErr,
					}
				}
				events = append(events, event)
			}
			offset += int64(len(line))
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			break
		}
		return Recording{}, err
	}

	if err := CheckIntegrity(events); err != nil {
		return Recording{}, err
	}

	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		if filter.Match(event) {
			filtered = append(filtered, event)
		}
	}
	return Recording{SchemaVersion: SchemaVersion, Events: filtered}, nil
}

func CheckIntegrity(events []Event) error {
	for i, event := range events {
		line := i + 1
		if event.SchemaVersion != SchemaVersion {
			return &IntegrityError{
				Line: line,
				Err:  fmt.Errorf("expected schema version %d, got %d", SchemaVersion, event.SchemaVersion),
			}
		}
		expected := uint64(i + 1)
		if event.Sequence != expected {
			return &IntegrityError{
				Line:     line,
				Expected: expected,
				Got:      event.Sequence,
			}
		}
	}
	return nil
}
