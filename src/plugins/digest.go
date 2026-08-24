package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"

	"awaken/src/pluginidentity"
)

func digestTree(root string) (string, error) {
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
		entries = append(entries, posix+" "+hex.EncodeToString(hasher.Sum(nil)))
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
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func SkillCatalogID(pluginName, skillID string) string {
	return pluginidentity.SkillCatalogID(pluginName, skillID)
}

func PluginNameFromSkillsRoot(skillsRoot string) string {
	return pluginidentity.PluginNameFromSkillsRoot(skillsRoot)
}
