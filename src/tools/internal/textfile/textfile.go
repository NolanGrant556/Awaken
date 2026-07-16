package textfile

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type Encoding string

const (
	EncodingUTF8    Encoding = "utf-8"
	EncodingUTF8BOM Encoding = "utf-8-bom"
	EncodingUTF16LE Encoding = "utf-16le"
	EncodingUTF16BE Encoding = "utf-16be"
)

type Document struct {
	Text     string
	Encoding Encoding
	Newline  string
}

func Decode(data []byte) (Document, error) {
	document := Document{Encoding: EncodingUTF8, Newline: detectNewline(data)}
	switch {
	case len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf:
		if !utf8.Valid(data[3:]) || containsNUL(data[3:]) {
			return Document{}, fmt.Errorf("invalid UTF-8 BOM text")
		}
		document.Encoding = EncodingUTF8BOM
		document.Text = string(data[3:])
	case len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe:
		document.Encoding = EncodingUTF16LE
		text, err := decodeUTF16(data[2:], binary.LittleEndian)
		if err != nil {
			return Document{}, err
		}
		document.Text = text
	case len(data) >= 2 && data[0] == 0xfe && data[1] == 0xff:
		document.Encoding = EncodingUTF16BE
		text, err := decodeUTF16(data[2:], binary.BigEndian)
		if err != nil {
			return Document{}, err
		}
		document.Text = text
	default:
		if !utf8.Valid(data) || containsNUL(data) {
			return Document{}, fmt.Errorf("unsupported or binary text encoding")
		}
		document.Text = string(data)
	}
	if strings.Contains(document.Text, "\r\n") {
		document.Newline = "\r\n"
	} else if strings.Contains(document.Text, "\r") && !strings.Contains(document.Text, "\n") {
		document.Newline = "\r"
	} else {
		document.Newline = "\n"
	}
	return document, nil
}

func Encode(text string, encoding Encoding) ([]byte, error) {
	switch encoding {
	case EncodingUTF8:
		return []byte(text), nil
	case EncodingUTF8BOM:
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...), nil
	case EncodingUTF16LE:
		return encodeUTF16(text, binary.LittleEndian, []byte{0xff, 0xfe}), nil
	case EncodingUTF16BE:
		return encodeUTF16(text, binary.BigEndian, []byte{0xfe, 0xff}), nil
	default:
		return nil, fmt.Errorf("unsupported text encoding %q", encoding)
	}
}

func AdaptNewlines(text, newline string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if newline == "" || newline == "\n" {
		return normalized
	}
	return strings.ReplaceAll(normalized, "\n", newline)
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	if mode == 0 {
		mode = 0o644
	}
	temporary, err := os.CreateTemp(directory, ".awaken-write-*")
	if err != nil {
		return fmt.Errorf("create same-directory temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return fmt.Errorf("preserve file mode: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}
	cleanup = false
	if directoryHandle, err := os.Open(directory); err == nil {
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

func decodeUTF16(data []byte, order binary.ByteOrder) (string, error) {
	if len(data)%2 != 0 {
		return "", fmt.Errorf("invalid UTF-16 byte length")
	}
	values := make([]uint16, len(data)/2)
	for i := range values {
		values[i] = order.Uint16(data[i*2 : i*2+2])
	}
	return string(utf16.Decode(values)), nil
}

func encodeUTF16(text string, order binary.ByteOrder, bom []byte) []byte {
	values := utf16.Encode([]rune(text))
	result := make([]byte, len(bom)+len(values)*2)
	copy(result, bom)
	for i, value := range values {
		order.PutUint16(result[len(bom)+i*2:len(bom)+i*2+2], value)
	}
	return result
}

func containsNUL(data []byte) bool {
	for _, value := range data {
		if value == 0 {
			return true
		}
	}
	return false
}

func detectNewline(data []byte) string {
	if strings.Contains(string(data), "\r\n") {
		return "\r\n"
	}
	return "\n"
}
