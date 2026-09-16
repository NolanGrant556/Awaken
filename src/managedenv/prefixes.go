package managedenv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type prefixFixup struct {
	Path        string `json:"path"`
	Root        string `json:"root,omitempty"`
	Placeholder string `json:"placeholder"`
	Mode        string `json:"mode"`
}

// Native bundles may carry fixed-width prefixes from their upstream binary
// packages. Only declared files inside this component are rewritten, before
// validation/publication; the immutable download cache is never modified.
func (m *Manager) relocatePrefixes(ctx context.Context, root string) error {
	var fixups []prefixFixup
	file, err := os.Open(filepath.Join(root, "awaken-prefixes.json"))
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return failure("environment_prefix_invalid", "invalid prefix manifest")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) > 8<<20 || json.Unmarshal(data, &fixups) != nil {
		return failure("environment_prefix_invalid", "invalid or oversized prefix manifest")
	}
	if len(fixups) > 10000 {
		return failure("environment_prefix_invalid", "too many prefix entries")
	}
	for _, fixup := range fixups {
		if err := ctx.Err(); err != nil {
			return err
		}
		if fixup.Placeholder == "" || len(fixup.Placeholder) > 4096 || strings.ContainsRune(fixup.Placeholder, 0) {
			return failure("environment_prefix_invalid", "invalid source prefix")
		}
		path := filepath.Join(root, filepath.FromSlash(fixup.Path))
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !inside(root, resolved) {
			return failure("environment_prefix_invalid", "prefix file escapes component")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 512<<20 {
			return failure("environment_prefix_invalid", "invalid prefix file")
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return err
		}
		var replaced []byte
		replacementRoot := root
		if fixup.Root != "" {
			var err error
			replacementRoot, err = bundlePath(root, fixup.Root, true)
			if err != nil {
				return err
			}
		}
		replacement := filepath.ToSlash(replacementRoot)
		switch fixup.Mode {
		case "text":
			replaced = bytes.ReplaceAll(data, []byte(fixup.Placeholder), []byte(replacement))
		case "binary":
			replaced, err = replaceBinaryPrefix(data, []byte(fixup.Placeholder), []byte(replacement))
			if err != nil {
				return err
			}
		default:
			return failure("environment_prefix_invalid", "unknown prefix replacement mode")
		}
		if bytes.Equal(data, replaced) {
			continue
		}
		if err = os.WriteFile(resolved, replaced, info.Mode().Perm()); err != nil {
			return err
		}
		if strings.HasPrefix(m.target, "darwin-") && isMachO(replaced) {
			if _, err = m.run(ctx, root, Launch{Executable: "/usr/bin/codesign"}, "--force", "--sign", "-", resolved); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceBinaryPrefix(data, old, new []byte) ([]byte, error) {
	if len(new) > len(old) {
		return nil, failure("environment_path_too_long", "native component prefix cannot fit this user directory")
	}
	result := append([]byte(nil), data...)
	for offset := 0; offset < len(result); {
		at := bytes.Index(result[offset:], old)
		if at < 0 {
			break
		}
		at += offset
		end := bytes.IndexByte(result[at:], 0)
		if end < 0 {
			return nil, failure("environment_prefix_invalid", "binary prefix has no string terminator")
		}
		end += at
		segment := bytes.ReplaceAll(result[at:end], old, new)
		copy(result[at:end], segment)
		clear(result[at+len(segment) : end])
		offset = end + 1
	}
	return result, nil
}

func isMachO(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch string(data[:4]) {
	case "\xcf\xfa\xed\xfe", "\xfe\xed\xfa\xcf", "\xca\xfe\xba\xbe", "\xbe\xba\xfe\xca", "\xca\xfe\xba\xbf", "\xbf\xba\xfe\xca":
		return true
	}
	return false
}
