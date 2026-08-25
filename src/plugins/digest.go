package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"awaken/src/pluginidentity"
)

const pluginDigestV2Prefix = "sha256-v2:"

func digestTree(root string) (string, error) {
	return digestTreeVersion(root, true)
}

func digestTreeLegacy(root string) (string, error) {
	return digestTreeVersion(root, false)
}

func digestTreeVersion(root string, includeExecutable bool) (string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if err := rejectSymlink(full); err != nil {
			return err
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		posix := path.Clean(filepath.ToSlash(rel))
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		digestEntry := posix + " " + hex.EncodeToString(hasher.Sum(nil))
		if includeExecutable {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			executable := "0"
			if info.Mode().Perm()&0o111 != 0 {
				executable = "1"
			}
			digestEntry = posix + " " + executable + " " + hex.EncodeToString(hasher.Sum(nil))
		}
		entries = append(entries, digestEntry)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	hasher := sha256.New()
	for _, entry := range entries {
		if _, err := io.WriteString(hasher, entry); err != nil {
			return "", err
		}
		if _, err := io.WriteString(hasher, "\n"); err != nil {
			return "", err
		}
	}
	prefix := "sha256:"
	if includeExecutable {
		prefix = pluginDigestV2Prefix
	}
	return prefix + hex.EncodeToString(hasher.Sum(nil)), nil
}

func verifyTreeDigest(root, expected string) (string, bool, error) {
	var (
		actual string
		err    error
	)
	if strings.HasPrefix(expected, "sha256:") {
		actual, err = digestTreeLegacy(root)
	} else {
		actual, err = digestTree(root)
	}
	if err != nil {
		return "", false, err
	}
	return actual, actual == expected, nil
}

func SkillCatalogID(pluginName, skillID string) string {
	return pluginidentity.SkillCatalogID(pluginName, skillID)
}

func PluginNameFromSkillsRoot(skillsRoot string) string {
	return pluginidentity.PluginNameFromSkillsRoot(skillsRoot)
}
