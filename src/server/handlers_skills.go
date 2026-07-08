package server

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"awaken/src/skills"
)

// skillHandlers wires the /v1/skills HTTP surface to a skills.SkillRegistry.
type skillHandlers struct {
	registry      *skills.SkillRegistry
	workspaceRoot string
}

// skillInfo is the wire shape for entries returned by GET /v1/skills.
type skillInfo struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	DisplayName        string          `json:"displayName,omitempty"`
	Description        string          `json:"description"`
	WhenToUse          string          `json:"whenToUse,omitempty"`
	State              string          `json:"state"`
	UnavailableReason  string          `json:"unavailableReason,omitempty"`
	SourceOrigin       string          `json:"sourceOrigin"`
	Scope              string          `json:"scope"`
	RootDir            string          `json:"rootDir,omitempty"`
	SkillFilePath      string          `json:"skillFilePath,omitempty"`
	ProjectPath        string          `json:"projectPath,omitempty"`
	Context            string          `json:"context"`
	ShadowedBy         string          `json:"shadowedBy,omitempty"`
	AllowedTools       []string        `json:"allowedTools,omitempty"`
	McpBinding         *mcpBindingInfo `json:"mcpBinding,omitempty"`
	RequiredMcpServers []string        `json:"requiredMcpServers,omitempty"`
}

// mcpBindingInfo is the wire shape for the MCP binding field.
type mcpBindingInfo struct {
	ServerName string `json:"serverName"`
	PromptName string `json:"promptName"`
	Kind       string `json:"kind"`
}

type installSkillRequest struct {
	FileName      string `json:"fileName"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"contentBase64,omitempty"`
	Scope         string `json:"scope,omitempty"`
	ProjectRoot   string `json:"projectRoot,omitempty"`
}

type installSkillResponse struct {
	Skill   skillInfo `json:"skill"`
	Path    string    `json:"path"`
	Message string    `json:"message"`
}

const (
	maxSkillPackageFiles = 1000
	maxSkillPackageBytes = 50 << 20
)

type skillPackage struct {
	rawSkill     []byte
	fallbackName string
	files        []skillPackageFile
}

type skillPackageFile struct {
	relPath string
	data    []byte
	mode    os.FileMode
}

func (h skillHandlers) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	projectRoot, err := h.resolveOptionalSkillProjectRoot(r.URL.Query().Get("projectRoot"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT_ROOT", err.Error())
		return
	}

	all := h.registry.PreviewLocalSources(h.skillSourceDirs(projectRoot))
	out := make([]skillInfo, 0, len(all))
	for _, def := range all {
		info := skillInfo{
			ID:                 def.ID,
			Name:               def.Name,
			DisplayName:        def.DisplayName,
			Description:        def.Description,
			WhenToUse:          def.WhenToUse,
			State:              string(def.State),
			UnavailableReason:  def.UnavailableReason,
			SourceOrigin:       string(def.Source),
			Scope:              publicSkillScope(def),
			RootDir:            def.RootDir,
			SkillFilePath:      def.SkillFilePath,
			ProjectPath:        def.ProjectPath,
			Context:            string(def.Context),
			ShadowedBy:         def.ShadowedBy,
			AllowedTools:       def.AllowedTools,
			RequiredMcpServers: def.RequiredMcpServers,
		}
		if def.McpBinding != nil {
			info.McpBinding = &mcpBindingInfo{
				ServerName: def.McpBinding.ServerName,
				PromptName: def.McpBinding.PromptName,
				Kind:       def.McpBinding.Kind,
			}
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h skillHandlers) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}

	var req installSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	data, err := skillUploadBytes(req)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL_UPLOAD", err.Error())
		return
	}
	pkg, err := extractSkillPackage(req.FileName, data)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL_PACKAGE", err.Error())
		return
	}

	def, err := skills.ParseFrontmatter(string(pkg.rawSkill))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL", err.Error())
		return
	}
	def.ID = skills.SkillIDFromNameOrFallback(pkg.fallbackName, def.Name)
	if strings.TrimSpace(def.Name) == "" {
		def.Name = def.ID
	}
	if err := skills.ValidateNameReserved(def.Name); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL", err.Error())
		return
	}
	if err := skills.ValidateSchema(def); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL", err.Error())
		return
	}
	if def.Context == "" {
		def.Context = skills.ContextInline
	}

	scope, projectRoot, dir, source, err := h.resolveSkillInstallTarget(req)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL_TARGET", err.Error())
		return
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "SKILL_INSTALL_FAILED", err.Error())
		return
	}
	rootDir := filepath.Join(dir, def.ID)
	if err := writeSkillPackage(rootDir, pkg); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "SKILL_INSTALL_FAILED", err.Error())
		return
	}
	path := filepath.Join(rootDir, "SKILL.md")

	def.Source = source
	def.Scope = source
	def.RootDir = rootDir
	def.SkillFilePath = path
	def.FilePath = path
	if source == skills.SourceProject {
		def.ProjectPath = projectRoot
	}
	def.State = skills.StateAvailable

	writeJSON(w, http.StatusOK, installSkillResponse{
		Skill:   skillInfoFromDefinition(def),
		Path:    rootDir,
		Message: fmt.Sprintf("%s skill installed", scope),
	})
}

func (h skillHandlers) resolveSkillInstallTarget(req installSkillRequest) (scope string, projectRoot string, dir string, source skills.SourceOrigin, err error) {
	scope = strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = string(skills.SourceProject)
	}
	switch scope {
	case string(skills.SourceProject):
		projectRoot, err = h.resolveSkillProjectRoot(req.ProjectRoot)
		if err != nil {
			return "", "", "", "", err
		}
		return scope, projectRoot, filepath.Join(projectRoot, ".awaken", "skills"), skills.SourceProject, nil
	case "global", string(skills.SourceUser):
		home, homeErr := os.UserHomeDir()
		if homeErr != nil || strings.TrimSpace(home) == "" {
			return "", "", "", "", fmt.Errorf("user home directory unavailable")
		}
		projectRoot, _ = h.resolveOptionalSkillProjectRoot(req.ProjectRoot)
		return scope, projectRoot, filepath.Join(home, ".awaken", "skills"), skills.SourceUser, nil
	default:
		return "", "", "", "", fmt.Errorf("unsupported skill scope %q", scope)
	}
}

func (h skillHandlers) resolveSkillProjectRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" {
		root = strings.TrimSpace(h.workspaceRoot)
	}
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

func (h skillHandlers) resolveOptionalSkillProjectRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	explicit := root != ""
	if root == "" {
		root = strings.TrimSpace(h.workspaceRoot)
	}
	if root == "" {
		return "", nil
	}
	if !filepath.IsAbs(root) {
		if explicit {
			return "", fmt.Errorf("project root must be an absolute path")
		}
		return "", nil
	}
	clean := filepath.Clean(root)
	stat, err := os.Stat(clean)
	if err != nil {
		if explicit {
			return "", fmt.Errorf("project root is not accessible: %w", err)
		}
		return "", nil
	}
	if !stat.IsDir() {
		if explicit {
			return "", fmt.Errorf("project root is not a directory")
		}
		return "", nil
	}
	return clean, nil
}

func (h skillHandlers) skillSourceDirs(projectRoot string) map[skills.SourceOrigin]string {
	dirs := make(map[skills.SourceOrigin]string)
	if strings.TrimSpace(projectRoot) != "" {
		dirs[skills.SourceProject] = filepath.Join(projectRoot, ".awaken", "skills")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		dirs[skills.SourceUser] = filepath.Join(home, ".awaken", "skills")
	}
	return dirs
}

// handleExecuteSkill triggers direct execution of a skill by ID.
// POST /v1/skills/{id}/execute with JSON body of arguments.
func (h skillHandlers) handleExecuteSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}

	skillID := r.PathValue("id")
	if skillID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "skill id is required")
		return
	}

	def := h.registry.Get(skillID)
	if def == nil {
		writeErrorJSON(w, http.StatusNotFound, "SKILL_NOT_FOUND", "skill not found")
		return
	}

	if def.State != skills.StateAvailable {
		writeErrorJSON(w, http.StatusConflict, "SKILL_UNAVAILABLE", def.UnavailableReason)
		return
	}

	writeJSON(w, http.StatusOK, skillInfoFromDefinition(def))
}

func skillInfoFromDefinition(def *skills.SkillDefinition) skillInfo {
	info := skillInfo{
		ID:                 def.ID,
		Name:               def.Name,
		DisplayName:        def.DisplayName,
		Description:        def.Description,
		WhenToUse:          def.WhenToUse,
		State:              string(def.State),
		UnavailableReason:  def.UnavailableReason,
		SourceOrigin:       string(def.Source),
		Scope:              publicSkillScope(def),
		RootDir:            def.RootDir,
		SkillFilePath:      def.SkillFilePath,
		ProjectPath:        def.ProjectPath,
		Context:            string(def.Context),
		AllowedTools:       def.AllowedTools,
		RequiredMcpServers: def.RequiredMcpServers,
	}
	if def.McpBinding != nil {
		info.McpBinding = &mcpBindingInfo{
			ServerName: def.McpBinding.ServerName,
			PromptName: def.McpBinding.PromptName,
			Kind:       def.McpBinding.Kind,
		}
	}
	return info
}

func publicSkillScope(def *skills.SkillDefinition) string {
	if def == nil {
		return ""
	}
	switch def.Source {
	case skills.SourceUser:
		return "global"
	case skills.SourceProject:
		return "project"
	case skills.SourceMcp:
		return "mcp"
	default:
		if def.Scope == skills.SourceUser {
			return "global"
		}
		return string(def.Scope)
	}
}

func skillUploadBytes(req installSkillRequest) ([]byte, error) {
	if strings.TrimSpace(req.FileName) == "" {
		return nil, fmt.Errorf("fileName is required")
	}
	if req.ContentBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decode base64 content: %w", err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("uploaded file is empty")
		}
		return data, nil
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	return []byte(req.Content), nil
}

func extractSkillPackage(fileName string, data []byte) (skillPackage, error) {
	lowerName := strings.ToLower(fileName)
	if strings.HasSuffix(lowerName, ".zip") {
		return extractZipSkillPackage(fileName, data)
	}
	if !strings.HasSuffix(lowerName, ".md") {
		return skillPackage{}, fmt.Errorf("skill upload must be a SKILL.md file or a zip package")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return skillPackage{}, fmt.Errorf("uploaded file is empty")
	}
	return skillPackage{
		rawSkill:     data,
		fallbackName: strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)),
		files: []skillPackageFile{{
			relPath: "SKILL.md",
			data:    data,
			mode:    0o600,
		}},
	}, nil
}

func extractZipSkillPackage(fileName string, data []byte) (skillPackage, error) {
	if len(data) > maxSkillPackageBytes {
		return skillPackage{}, fmt.Errorf("skill package exceeds %d bytes", maxSkillPackageBytes)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return skillPackage{}, fmt.Errorf("open zip: %w", err)
	}
	if len(reader.File) > maxSkillPackageFiles {
		return skillPackage{}, fmt.Errorf("skill package contains too many files")
	}

	var skillFiles []*zip.File
	cleanNames := make(map[*zip.File]string, len(reader.File))
	for _, file := range reader.File {
		clean, err := cleanZipPath(file.Name)
		if err != nil {
			return skillPackage{}, err
		}
		cleanNames[file] = clean
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return skillPackage{}, fmt.Errorf("skill package cannot contain symlinks: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.EqualFold(path.Base(clean), "SKILL.md") {
			skillFiles = append(skillFiles, file)
		}
	}
	if len(skillFiles) == 0 {
		return skillPackage{}, fmt.Errorf("zip must contain exactly one SKILL.md")
	}
	if len(skillFiles) > 1 {
		return skillPackage{}, fmt.Errorf("zip contains multiple SKILL.md files")
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

	var files []skillPackageFile
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
			return skillPackage{}, err
		}
		totalBytes += int64(len(content))
		if totalBytes > maxSkillPackageBytes {
			return skillPackage{}, fmt.Errorf("skill package extracted content exceeds %d bytes", maxSkillPackageBytes)
		}
		mode := normalizedPackageFileMode(file.FileInfo().Mode())
		files = append(files, skillPackageFile{relPath: rel, data: content, mode: mode})
		if strings.EqualFold(rel, "SKILL.md") {
			rawSkill = content
		}
	}
	if len(bytes.TrimSpace(rawSkill)) == 0 {
		return skillPackage{}, fmt.Errorf("SKILL.md is empty")
	}
	return skillPackage{rawSkill: rawSkill, fallbackName: fallbackName, files: files}, nil
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
	data, err := io.ReadAll(io.LimitReader(rc, maxSkillPackageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Name, err)
	}
	if len(data) > maxSkillPackageBytes {
		return nil, fmt.Errorf("read %s: file exceeds %d bytes", file.Name, maxSkillPackageBytes)
	}
	return data, nil
}

func normalizedPackageFileMode(mode os.FileMode) os.FileMode {
	if mode&0o111 != 0 {
		return 0o700
	}
	return 0o600
}

func writeSkillPackage(rootDir string, pkg skillPackage) error {
	parent := filepath.Dir(rootDir)
	tempDir, err := os.MkdirTemp(parent, "."+filepath.Base(rootDir)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp package dir: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()

	for _, file := range pkg.files {
		rel, err := cleanPackageRelPath(file.relPath)
		if err != nil {
			return err
		}
		target := filepath.Join(tempDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create package directory: %w", err)
		}
		if err := os.WriteFile(target, file.data, file.mode); err != nil {
			return fmt.Errorf("write package file %s: %w", rel, err)
		}
	}

	if err := os.RemoveAll(rootDir); err != nil {
		return fmt.Errorf("replace existing skill package: %w", err)
	}
	if err := os.Rename(tempDir, rootDir); err != nil {
		return fmt.Errorf("install skill package: %w", err)
	}
	cleanup = false
	return nil
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
