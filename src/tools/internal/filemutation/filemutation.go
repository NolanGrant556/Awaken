package filemutation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"awaken/src/tool"
	"awaken/src/tools/internal/textfile"
)

func Snapshot(path string, data []byte, info os.FileInfo, mimeType string) tool.FileReadState {
	return tool.FileReadState{
		CanonicalPath: filepath.Clean(path),
		SHA256:        textfile.Digest(data),
		Size:          int64(len(data)),
		ModTime:       info.ModTime().UTC(),
		Mode:          uint32(info.Mode()),
		MimeType:      mimeType,
	}
}

func SameSnapshot(left, right tool.FileReadState) bool {
	return left.CanonicalPath == right.CanonicalPath && left.SHA256 == right.SHA256 && left.Size == right.Size && left.ModTime.Equal(right.ModTime) && left.Mode == right.Mode && left.MimeType == right.MimeType
}

func LoadExisting(path string, expected tool.FileReadState) ([]byte, os.FileInfo, tool.FileReadState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, tool.FileReadState{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, tool.FileReadState{}, fmt.Errorf("target is not a regular non-symlink file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, tool.FileReadState{}, err
	}
	current := Snapshot(path, data, info, expected.MimeType)
	if !SameSnapshot(expected, current) {
		return nil, nil, current, fmt.Errorf("file_changed_since_read")
	}
	return data, info, current, nil
}

func RefreshedSnapshot(path, mimeType string) (tool.FileReadState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tool.FileReadState{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return tool.FileReadState{}, err
	}
	return Snapshot(path, data, info, mimeType), nil
}

func Details(snapshot tool.FileReadState) map[string]any {
	return map[string]any{
		"file_path": snapshot.CanonicalPath,
		"sha256":    snapshot.SHA256,
		"size":      snapshot.Size,
		"mtime":     snapshot.ModTime.Format(time.RFC3339Nano),
		"mode":      snapshot.Mode,
		"mime_type": snapshot.MimeType,
	}
}
