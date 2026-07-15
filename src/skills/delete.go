package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/tool/trash"
)

var (
	ErrSkillDeleteNotFound          = errors.New("skill package not found")
	ErrSkillDeleteSourceUnsupported = errors.New("skill source does not support deletion")
	ErrSkillDeleteTrashUnavailable  = errors.New("skill package Trash unavailable")
)

type DeleteRequest struct {
	ID               string
	Source           SourceOrigin
	ProjectRoot      string
	DefaultWorkspace string
	TrashAdapter     trash.TrashAdapter
}

type DeleteResult struct {
	ID           string
	Source       SourceOrigin
	OriginalPath string
	Trash        trash.TrashResult
}

// DeleteSkillPackage synchronously removes one project/user package from its
// .awaken/skills source and moves the complete package directory to the
// platform Trash. Other discovery sources are not owned by this lifecycle and
// fail closed.
func DeleteSkillPackage(req DeleteRequest) (DeleteResult, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return DeleteResult{}, fmt.Errorf("skill id is invalid")
	}

	scope := ""
	switch req.Source {
	case SourceProject:
		scope = string(SourceProject)
	case SourceUser:
		scope = "global"
	default:
		return DeleteResult{}, fmt.Errorf("%w: %s", ErrSkillDeleteSourceUnsupported, req.Source)
	}

	_, _, installDir, source, err := ResolveInstallTarget(InstallTargetRequest{
		Scope:            scope,
		ProjectRoot:      req.ProjectRoot,
		DefaultWorkspace: req.DefaultWorkspace,
	})
	if err != nil {
		return DeleteResult{}, err
	}
	if source != req.Source {
		return DeleteResult{}, fmt.Errorf("%w: resolved %s for requested %s", ErrSkillDeleteSourceUnsupported, source, req.Source)
	}
	if info, statErr := os.Stat(installDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return DeleteResult{}, fmt.Errorf("%w: %s", ErrSkillDeleteNotFound, id)
		}
		return DeleteResult{}, fmt.Errorf("stat skill source: %w", statErr)
	} else if !info.IsDir() {
		return DeleteResult{}, fmt.Errorf("skill source is not a directory: %s", installDir)
	}

	root, err := openRootedSkillInstallDir(installDir)
	if err != nil {
		return DeleteResult{}, err
	}
	defer root.Close()

	var target *SkillDefinition
	for _, def := range LoadDir(installDir, source) {
		if def != nil && def.ID == id && def.Source == source {
			target = def
			break
		}
	}
	if target == nil {
		return DeleteResult{}, fmt.Errorf("%w: %s", ErrSkillDeleteNotFound, id)
	}

	packageName, err := directSkillPackageEntry(installDir, target.RootDir)
	if err != nil {
		return DeleteResult{}, err
	}
	info, err := root.Lstat(packageName)
	if err != nil {
		if os.IsNotExist(err) {
			return DeleteResult{}, fmt.Errorf("%w: %s", ErrSkillDeleteNotFound, id)
		}
		return DeleteResult{}, fmt.Errorf("stat skill package: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return DeleteResult{}, fmt.Errorf("skill package must be a real directory: %s", target.RootDir)
	}

	adapter := req.TrashAdapter
	if adapter == nil {
		adapter = trash.DefaultAdapter()
	}
	if adapter == nil {
		return DeleteResult{}, fmt.Errorf("%w: adapter unavailable", ErrSkillDeleteTrashUnavailable)
	}
	originalPath := filepath.Join(installDir, packageName)
	trashed, err := adapter.TrashPath(originalPath)
	if err != nil {
		return DeleteResult{}, fmt.Errorf("%w: %v", ErrSkillDeleteTrashUnavailable, err)
	}
	if _, err := root.Lstat(packageName); err == nil || !os.IsNotExist(err) {
		return DeleteResult{}, fmt.Errorf("%w: Trash operation did not remove %s", ErrSkillDeleteTrashUnavailable, originalPath)
	}
	return DeleteResult{
		ID:           id,
		Source:       source,
		OriginalPath: originalPath,
		Trash:        trashed,
	}, nil
}

func directSkillPackageEntry(installDir, packageDir string) (string, error) {
	installDir = filepath.Clean(installDir)
	packageDir = filepath.Clean(packageDir)
	rel, err := filepath.Rel(installDir, packageDir)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || filepath.Dir(rel) != "." {
		return "", fmt.Errorf("skill package is not a direct child of its source root")
	}
	if strings.TrimSpace(rel) == "" || strings.HasPrefix(rel, ".") {
		return "", fmt.Errorf("skill package entry is invalid")
	}
	return rel, nil
}
