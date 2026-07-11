package skills

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxSkillPackageFiles = 1000
	MaxSkillPackageBytes = 50 << 20
)

var (
	ErrSkillAlreadyExists = errors.New("skill already exists")
)

type SkillPackage struct {
	rawSkill     []byte
	fallbackName string
	files        []SkillPackageFile
}

type SkillPackageFile struct {
	relPath string
	data    []byte
	mode    os.FileMode
}

type InstallRequest struct {
	Package          SkillPackage
	Scope            string
	ProjectRoot      string
	DefaultWorkspace string
	OverwritePolicy  string
}

type InstallResult struct {
	Skill   *SkillDefinition
	Path    string
	Scope   string
	Message string
}

func PackageFromBytes(fileName string, data []byte) (SkillPackage, error) {
	lowerName := strings.ToLower(strings.TrimSpace(fileName))
	if strings.HasSuffix(lowerName, ".zip") {
		return extractZipSkillPackage(fileName, data)
	}
	if !strings.HasSuffix(lowerName, ".md") {
		return SkillPackage{}, fmt.Errorf("skill upload must be a SKILL.md file or a zip package")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return SkillPackage{}, fmt.Errorf("uploaded file is empty")
	}
	return SkillPackage{
		rawSkill:     data,
		fallbackName: strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)),
		files: []SkillPackageFile{{
			relPath: "SKILL.md",
			data:    data,
			mode:    0o600,
		}},
	}, nil
}

func PackageFromPath(sourcePath string) (SkillPackage, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return SkillPackage{}, fmt.Errorf("source path is required")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return SkillPackage{}, fmt.Errorf("stat skill source: %w", err)
	}
	if !info.IsDir() {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return SkillPackage{}, fmt.Errorf("read skill source: %w", err)
		}
		return PackageFromBytes(filepath.Base(sourcePath), data)
	}
	return packageFromDirectory(sourcePath)
}

func InstallSkillPackage(req InstallRequest) (InstallResult, error) {
	pkg := req.Package
	if len(bytes.TrimSpace(pkg.rawSkill)) == 0 {
		return InstallResult{}, fmt.Errorf("SKILL.md is empty")
	}

	def, err := ParseFrontmatter(string(pkg.rawSkill))
	if err != nil {
		return InstallResult{}, err
	}
	def.ID = SkillIDFromNameOrFallback(pkg.fallbackName, def.Name)
	if strings.TrimSpace(def.Name) == "" {
		def.Name = def.ID
	}
	if err := ValidateNameReserved(def.Name); err != nil {
		return InstallResult{}, err
	}
	if err := ValidateSchema(def); err != nil {
		return InstallResult{}, err
	}
	if def.Context == "" {
		def.Context = ContextInline
	}

	scope, projectRoot, dir, source, err := ResolveInstallTarget(InstallTargetRequest{
		Scope:            req.Scope,
		ProjectRoot:      req.ProjectRoot,
		DefaultWorkspace: req.DefaultWorkspace,
	})
	if err != nil {
		return InstallResult{}, err
	}

	policy, err := normalizeOverwritePolicy(req.OverwritePolicy)
	if err != nil {
		return InstallResult{}, err
	}
	rootDir := filepath.Join(dir, def.ID)
	installRoot, err := openRootedSkillInstallDir(dir)
	if err != nil {
		return InstallResult{}, err
	}
	defer installRoot.Close()
	targetExists := false
	if _, err := installRoot.Stat(def.ID); err == nil {
		targetExists = true
	} else if !os.IsNotExist(err) {
		return InstallResult{}, fmt.Errorf("stat existing skill package: %w", err)
	}
	if targetExists && policy == "abort" {
		return InstallResult{}, fmt.Errorf("%w: %s", ErrSkillAlreadyExists, rootDir)
	}
	if err := writeSkillPackage(installRoot, def.ID, pkg, targetExists); err != nil {
		return InstallResult{}, err
	}

	skillPath := filepath.Join(rootDir, "SKILL.md")
	def.Source = source
	def.Scope = source
	def.RootDir = rootDir
	if resolvedRoot, resolveErr := filepath.EvalSymlinks(rootDir); resolveErr == nil {
		def.ResolvedRootDir = resolvedRoot
	}
	def.SkillFilePath = skillPath
	def.FilePath = skillPath
	if source == SourceProject {
		def.ProjectPath = projectRoot
	}
	def.State = StateAvailable

	return InstallResult{
		Skill:   def,
		Path:    rootDir,
		Scope:   scope,
		Message: fmt.Sprintf("%s skill installed", scope),
	}, nil
}

type InstallTargetRequest struct {
	Scope            string
	ProjectRoot      string
	DefaultWorkspace string
}

func ResolveInstallTarget(req InstallTargetRequest) (scope string, projectRoot string, dir string, source SourceOrigin, err error) {
	scope = strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = string(SourceProject)
	}
	switch scope {
	case string(SourceProject):
		projectRoot, err = resolveSkillProjectRoot(firstNonEmpty(req.ProjectRoot, req.DefaultWorkspace))
		if err != nil {
			return "", "", "", "", err
		}
		return scope, projectRoot, filepath.Join(projectRoot, ".awaken", "skills"), SourceProject, nil
	case "global", string(SourceUser):
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || strings.TrimSpace(home) == "" {
			return "", "", "", "", fmt.Errorf("user home directory unavailable")
		}
		projectRoot, _ = resolveOptionalSkillProjectRoot(firstNonEmpty(req.ProjectRoot, req.DefaultWorkspace))
		return "global", projectRoot, filepath.Join(home, ".awaken", "skills"), SourceUser, nil
	default:
		return "", "", "", "", fmt.Errorf("unsupported skill scope %q", scope)
	}
}

func LocalSourceDirs(projectRoot string) map[SourceOrigin]string {
	dirs := make(map[SourceOrigin]string)
	if root := strings.TrimSpace(projectRoot); root != "" && filepath.IsAbs(root) {
		if stat, err := os.Stat(root); err == nil && stat.IsDir() {
			dirs[SourceProject] = filepath.Join(root, ".awaken", "skills")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs[SourceUser] = filepath.Join(home, ".awaken", "skills")
	}
	return dirs
}

func FormatRuntimeSkillSummary(defs []*SkillDefinition) string {
	if len(defs) == 0 {
		return "none"
	}
	var b strings.Builder
	for _, def := range defs {
		if def == nil {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- id: ")
		b.WriteString(def.ID)
		b.WriteString("; name: ")
		b.WriteString(def.Name)
		b.WriteString("; scope: ")
		b.WriteString(publicScopeForSource(def.Source))
		b.WriteString("; state: ")
		b.WriteString(string(def.State))
		if strings.TrimSpace(def.RootDir) != "" {
			b.WriteString("; rootDir: ")
			b.WriteString(def.RootDir)
		}
		if strings.TrimSpace(def.SkillFilePath) != "" {
			b.WriteString("; skillFilePath: ")
			b.WriteString(def.SkillFilePath)
		}
		if strings.TrimSpace(def.ProjectPath) != "" {
			b.WriteString("; projectPath: ")
			b.WriteString(def.ProjectPath)
		}
		if strings.TrimSpace(def.ShadowedBy) != "" {
			b.WriteString("; shadowedBy: ")
			b.WriteString(def.ShadowedBy)
		}
		if strings.TrimSpace(def.WhenToUse) != "" {
			b.WriteString("; whenToUse: ")
			b.WriteString(def.WhenToUse)
		} else if strings.TrimSpace(def.Description) != "" {
			b.WriteString("; description: ")
			b.WriteString(def.Description)
		}
	}
	if b.Len() == 0 {
		return "none"
	}
	return b.String()
}

func publicScopeForSource(source SourceOrigin) string {
	switch source {
	case SourceUser:
		return "global"
	case SourceProject:
		return "project"
	case SourceMcp:
		return "mcp"
	default:
		return string(source)
	}
}

func packageFromDirectory(sourceDir string) (SkillPackage, error) {
	root, err := resolvePackageRoot(sourceDir)
	if err != nil {
		return SkillPackage{}, err
	}

	var files []SkillPackageFile
	var rawSkill []byte
	var totalBytes int64
	count := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package cannot contain symlinks: %s", path)
		}
		count++
		if count > MaxSkillPackageFiles {
			return fmt.Errorf("skill package contains too many files")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		totalBytes += int64(len(data))
		if totalBytes > MaxSkillPackageBytes {
			return fmt.Errorf("skill package extracted content exceeds %d bytes", MaxSkillPackageBytes)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.EqualFold(rel, "SKILL.md") {
			rawSkill = data
			rel = "SKILL.md"
		}
		files = append(files, SkillPackageFile{
			relPath: rel,
			data:    data,
			mode:    normalizedPackageFileMode(info.Mode()),
		})
		return nil
	})
	if err != nil {
		return SkillPackage{}, err
	}
	if len(bytes.TrimSpace(rawSkill)) == 0 {
		return SkillPackage{}, fmt.Errorf("SKILL.md is empty")
	}
	return SkillPackage{rawSkill: rawSkill, fallbackName: filepath.Base(root), files: files}, nil
}

func resolvePackageRoot(sourceDir string) (string, error) {
	clean := filepath.Clean(sourceDir)
	rootSkill := filepath.Join(clean, "SKILL.md")
	if info, err := os.Stat(rootSkill); err == nil && !info.IsDir() {
		return clean, nil
	}

	var matches []string
	err := filepath.WalkDir(clean, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(entry.Name(), "SKILL.md") {
			matches = append(matches, path)
		}
		if len(matches) > 1 {
			return fmt.Errorf("source directory contains multiple SKILL.md files; provide a specific skill subpath")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("source directory must contain SKILL.md")
	}
	return filepath.Dir(matches[0]), nil
}

func extractZipSkillPackage(fileName string, data []byte) (SkillPackage, error) {
	if len(data) > MaxSkillPackageBytes {
		return SkillPackage{}, fmt.Errorf("skill package exceeds %d bytes", MaxSkillPackageBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return SkillPackage{}, fmt.Errorf("open zip: %w", err)
	}
	if len(reader.File) > MaxSkillPackageFiles {
		return SkillPackage{}, fmt.Errorf("skill package contains too many files")
	}

	var skillFiles []*zip.File
	cleanNames := make(map[*zip.File]string, len(reader.File))
	for _, file := range reader.File {
		clean, err := cleanZipPath(file.Name)
		if err != nil {
			return SkillPackage{}, err
		}
		cleanNames[file] = clean
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return SkillPackage{}, fmt.Errorf("skill package cannot contain symlinks: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(path.Base(clean), "SKILL.md") {
			skillFiles = append(skillFiles, file)
		}
	}
	if len(skillFiles) == 0 {
		return SkillPackage{}, fmt.Errorf("zip must contain exactly one SKILL.md")
	}
	if len(skillFiles) > 1 {
		return SkillPackage{}, fmt.Errorf("zip contains multiple SKILL.md files")
	}

	skillFile := skillFiles[0]
	skillPath := cleanNames[skillFile]
	packageRoot := path.Dir(skillPath)
	if packageRoot == "." {
		packageRoot = ""
	}
	fallbackName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if packageRoot != "" {
		fallbackName = path.Base(packageRoot)
	}

	var files []SkillPackageFile
	var rawSkill []byte
	var totalBytes int64
	for _, file := range reader.File {
		clean := cleanNames[file]
		if !isInZipPackageRoot(clean, packageRoot) {
			continue
		}
		rel := clean
		if packageRoot != "" {
			rel = strings.TrimPrefix(clean, packageRoot+"/")
		}
		if rel == "" || rel == "." || file.FileInfo().IsDir() {
			continue
		}

		content, err := readZipFile(file)
		if err != nil {
			return SkillPackage{}, err
		}
		totalBytes += int64(len(content))
		if totalBytes > MaxSkillPackageBytes {
			return SkillPackage{}, fmt.Errorf("skill package extracted content exceeds %d bytes", MaxSkillPackageBytes)
		}
		mode := normalizedPackageFileMode(file.FileInfo().Mode())
		files = append(files, SkillPackageFile{relPath: rel, data: content, mode: mode})
		if strings.EqualFold(rel, "SKILL.md") {
			rawSkill = content
		}
	}
	if len(bytes.TrimSpace(rawSkill)) == 0 {
		return SkillPackage{}, fmt.Errorf("SKILL.md is empty")
	}
	return SkillPackage{rawSkill: rawSkill, fallbackName: fallbackName, files: files}, nil
}

func cleanZipPath(name string) (string, error) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.TrimSpace(normalized) == "" {
		return "", fmt.Errorf("skill package contains empty path")
	}
	if path.IsAbs(normalized) {
		return "", fmt.Errorf("skill package contains absolute path: %s", name)
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("skill package contains unsafe path: %s", name)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("skill package contains unsafe path: %s", name)
		}
	}
	return clean, nil
}

func isInZipPackageRoot(clean string, packageRoot string) bool {
	if packageRoot == "" {
		return true
	}
	return clean == packageRoot || strings.HasPrefix(clean, packageRoot+"/")
}

func readZipFile(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, MaxSkillPackageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Name, err)
	}
	if len(data) > MaxSkillPackageBytes {
		return nil, fmt.Errorf("read %s: file exceeds %d bytes", file.Name, MaxSkillPackageBytes)
	}
	return data, nil
}

func normalizedPackageFileMode(mode os.FileMode) os.FileMode {
	if mode&0o111 != 0 {
		return 0o700
	}
	return 0o600
}

func openRootedSkillInstallDir(installDir string) (*os.Root, error) {
	baseDir := filepath.Dir(filepath.Dir(installDir))
	rel, err := filepath.Rel(baseDir, installDir)
	if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("resolve rooted skill install directory")
	}
	baseRoot, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, fmt.Errorf("open skill install base: %w", err)
	}
	defer baseRoot.Close()
	if err := baseRoot.MkdirAll(rel, 0o700); err != nil {
		return nil, fmt.Errorf("create rooted skill install directory: %w", err)
	}
	root, err := baseRoot.OpenRoot(rel)
	if err != nil {
		return nil, fmt.Errorf("open rooted skill install directory: %w", err)
	}
	return root, nil
}

func writeSkillPackage(root *os.Root, targetName string, pkg SkillPackage, targetExists bool) error {
	tempName, err := createRootTempDir(root, "."+targetName+".tmp-")
	if err != nil {
		return fmt.Errorf("create temp package dir: %w", err)
	}
	tempPresent := true
	defer func() {
		if tempPresent {
			_ = root.RemoveAll(tempName)
		}
	}()

	for _, file := range pkg.files {
		rel, err := cleanPackageRelPath(file.relPath)
		if err != nil {
			return err
		}
		target := filepath.Join(tempName, filepath.FromSlash(rel))
		if err := root.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create package directory: %w", err)
		}
		if err := root.WriteFile(target, file.data, file.mode); err != nil {
			return fmt.Errorf("write package file %s: %w", rel, err)
		}
	}

	if !targetExists {
		if err := root.Rename(tempName, targetName); err != nil {
			return fmt.Errorf("install skill package: %w", err)
		}
		tempPresent = false
		return nil
	}

	backupName, err := uniqueRootEntryName(root, "."+targetName+".backup-")
	if err != nil {
		return fmt.Errorf("prepare skill replacement backup: %w", err)
	}
	if err := root.Rename(targetName, backupName); err != nil {
		return fmt.Errorf("backup existing skill package: %w", err)
	}
	if err := root.Rename(tempName, targetName); err != nil {
		rollbackErr := root.Rename(backupName, targetName)
		if rollbackErr != nil {
			return fmt.Errorf("install replacement: %v; restore previous package: %w", err, rollbackErr)
		}
		return fmt.Errorf("install replacement: %w", err)
	}
	tempPresent = false
	if err := root.RemoveAll(backupName); err != nil {
		return fmt.Errorf("replacement installed but previous package cleanup failed: %w", err)
	}
	return nil
}

func createRootTempDir(root *os.Root, prefix string) (string, error) {
	for i := 0; i < 32; i++ {
		name, err := uniqueRootEntryName(root, prefix)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		if err := root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate a unique package directory")
}

func uniqueRootEntryName(root *os.Root, prefix string) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name := prefix + hex.EncodeToString(random[:])
	if _, err := root.Lstat(name); err == nil {
		return "", os.ErrExist
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return name, nil
}

func cleanPackageRelPath(rel string) (string, error) {
	clean, err := cleanZipPath(rel)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(clean, "SKILL.md") {
		return "SKILL.md", nil
	}
	return clean, nil
}

func resolveSkillProjectRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		return "", fmt.Errorf("project root is required")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("project root must be an absolute path")
	}
	clean := filepath.Clean(root)
	stat, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("project root is not accessible: %w", err)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("project root is not a directory")
	}
	return clean, nil
}

func resolveOptionalSkillProjectRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		return "", nil
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("project root must be an absolute path")
	}
	clean := filepath.Clean(root)
	stat, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("project root is not accessible: %w", err)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("project root is not a directory")
	}
	return clean, nil
}

func normalizeOverwritePolicy(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", "abort":
		return "abort", nil
	case "replace":
		return "replace", nil
	default:
		return "", fmt.Errorf("unsupported overwrite policy %q", raw)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
