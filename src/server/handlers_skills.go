package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool/trash"
)

// skillHandlers wires the /v1/skills HTTP surface to a skills.SkillRegistry.
type skillHandlers struct {
	registry      *skills.SkillRegistry
	workspaceRoot string
	trashAdapter  trash.TrashAdapter
}

// skillInfo is the wire shape for entries returned by GET /v1/skills.
type skillInfo struct {
	ID                 string                `json:"id"`
	CatalogEntryID     string                `json:"catalogEntryId,omitempty"`
	Name               string                `json:"name"`
	DisplayName        string                `json:"displayName,omitempty"`
	Description        string                `json:"description"`
	WhenToUse          string                `json:"whenToUse,omitempty"`
	State              string                `json:"state"`
	UnavailableReason  string                `json:"unavailableReason,omitempty"`
	SourceOrigin       string                `json:"sourceOrigin"`
	Scope              string                `json:"scope"`
	RootDir            string                `json:"rootDir,omitempty"`
	SkillFilePath      string                `json:"skillFilePath,omitempty"`
	ProjectPath        string                `json:"projectPath,omitempty"`
	Context            string                `json:"context"`
	ShadowedBy         string                `json:"shadowedBy,omitempty"`
	AllowedTools       []string              `json:"allowedTools,omitempty"`
	McpBinding         *mcpBindingInfo       `json:"mcpBinding,omitempty"`
	RequiredMcpServers []string              `json:"requiredMcpServers,omitempty"`
	UserInvocable      bool                  `json:"userInvocable"`
	ModelInvocable     bool                  `json:"modelInvocable"`
	Diagnostics        []skillDiagnosticInfo `json:"diagnostics,omitempty"`
	LastInvokedAt      string                `json:"lastInvokedAt,omitempty"`
}

type skillDiagnosticInfo struct {
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// mcpBindingInfo is the wire shape for the MCP binding field.
type mcpBindingInfo struct {
	ServerName string `json:"serverName"`
	PromptName string `json:"promptName"`
	Kind       string `json:"kind"`
}

type installSkillRequest struct {
	FileName        string `json:"fileName"`
	Content         string `json:"content,omitempty"`
	ContentBase64   string `json:"contentBase64,omitempty"`
	Scope           string `json:"scope,omitempty"`
	ProjectRoot     string `json:"projectRoot,omitempty"`
	OverwritePolicy string `json:"overwritePolicy,omitempty"`
}

type installSkillResponse struct {
	Skill   skillInfo `json:"skill"`
	Path    string    `json:"path"`
	Message string    `json:"message"`
}

type deleteSkillRequest struct {
	SourceOrigin string `json:"sourceOrigin"`
	ProjectRoot  string `json:"projectRoot,omitempty"`
}

type deleteSkillResponse struct {
	ID           string `json:"id"`
	SourceOrigin string `json:"sourceOrigin"`
	OriginalPath string `json:"originalPath"`
	TrashedPath  string `json:"trashedPath,omitempty"`
	Message      string `json:"message"`
}

type previewSkillRequest struct {
	CatalogEntryID string `json:"catalogEntryId"`
	ProjectRoot    string `json:"projectRoot,omitempty"`
}

type previewSkillResponse struct {
	FrontmatterRaw string `json:"frontmatterRaw"`
	BodyMarkdown   string `json:"bodyMarkdown"`
}

type skillStateResponse struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (h skillHandlers) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	projectRoots, err := resolveSkillPreviewProjectRoots(r.URL.Query()["projectRoot"], h.workspaceRoot)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT_ROOT", err.Error())
		return
	}

	all := h.registry.PreviewLocalSources(h.skillSourceDirsForProjectRoots(projectRoots))
	out := make([]skillInfo, 0, len(all))
	for _, def := range all {
		out = append(out, skillInfoFromDefinition(def))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h skillHandlers) handlePreviewSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	var req previewSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	entryID := strings.TrimSpace(req.CatalogEntryID)
	if entryID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "catalogEntryId is required")
		return
	}
	projectRoot, err := resolveSkillPreviewProjectRoot(req.ProjectRoot, h.workspaceRoot)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT_ROOT", err.Error())
		return
	}

	var selected *skills.SkillDefinition
	for _, def := range h.registry.PreviewLocalSources(h.skillSourceDirs(projectRoot)) {
		if skills.CatalogEntryID(def) == entryID {
			selected = def
			break
		}
	}
	if selected == nil {
		writeErrorJSON(w, http.StatusNotFound, "SKILL_PREVIEW_NOT_FOUND", "skill preview entry is no longer available; refresh the Skills list")
		return
	}
	preview, err := skills.ReadSkillPreview(selected)
	if err != nil {
		switch {
		case errors.Is(err, skills.ErrSkillPreviewTooLarge):
			writeErrorJSON(w, http.StatusRequestEntityTooLarge, "SKILL_PREVIEW_TOO_LARGE", err.Error())
		case errors.Is(err, skills.ErrSkillPreviewNotUTF8):
			writeErrorJSON(w, http.StatusUnprocessableEntity, "SKILL_PREVIEW_NOT_UTF8", err.Error())
		case errors.Is(err, skills.ErrSkillPreviewPathDenied):
			writeErrorJSON(w, http.StatusForbidden, "SKILL_PREVIEW_PATH_DENIED", err.Error())
		default:
			writeErrorJSON(w, http.StatusConflict, "SKILL_PREVIEW_UNAVAILABLE", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, previewSkillResponse{
		FrontmatterRaw: preview.FrontmatterRaw,
		BodyMarkdown:   preview.BodyMarkdown,
	})
}

func resolveSkillPreviewProjectRoot(rawProjectRoot, fallback string) (string, error) {
	projectRoots, err := resolveSkillPreviewProjectRoots([]string{rawProjectRoot}, fallback)
	if err != nil || len(projectRoots) == 0 {
		return "", err
	}
	return projectRoots[0], nil
}

func resolveSkillPreviewProjectRoots(rawProjectRoots []string, fallback string) ([]string, error) {
	hasExplicitRoot := false
	for _, rawProjectRoot := range rawProjectRoots {
		if strings.TrimSpace(rawProjectRoot) != "" {
			hasExplicitRoot = true
			break
		}
	}
	if !hasExplicitRoot {
		rawProjectRoots = []string{fallback}
	}
	projectRoots := make([]string, 0, len(rawProjectRoots))
	seen := make(map[string]struct{}, len(rawProjectRoots))
	for _, rawProjectRoot := range rawProjectRoots {
		rawProjectRoot = strings.TrimSpace(rawProjectRoot)
		if rawProjectRoot == "" {
			continue
		}
		_, resolved, _, _, err := skills.ResolveInstallTarget(skills.InstallTargetRequest{
			Scope:       string(skills.SourceProject),
			ProjectRoot: rawProjectRoot,
		})
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[resolved]; duplicate {
			continue
		}
		seen[resolved] = struct{}{}
		projectRoots = append(projectRoots, resolved)
	}
	return projectRoots, nil
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
	pkg, err := skills.PackageFromBytes(req.FileName, data)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL_PACKAGE", err.Error())
		return
	}
	result, err := skills.InstallSkillPackage(skills.InstallRequest{
		Package:          pkg,
		Scope:            req.Scope,
		ProjectRoot:      req.ProjectRoot,
		DefaultWorkspace: h.workspaceRoot,
		OverwritePolicy:  req.OverwritePolicy,
	})
	if err != nil {
		if errors.Is(err, skills.ErrSkillAlreadyExists) {
			writeErrorJSON(w, http.StatusConflict, "SKILL_ALREADY_EXISTS", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusBadRequest, "SKILL_INSTALL_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, installSkillResponse{
		Skill:   skillInfoFromDefinition(result.Skill),
		Path:    result.Path,
		Message: result.Message,
	})
}

func (h skillHandlers) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "skill id is required")
		return
	}
	var req deleteSkillRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	result, err := skills.DeleteSkillPackage(skills.DeleteRequest{
		ID:               id,
		Source:           skills.SourceOrigin(strings.TrimSpace(req.SourceOrigin)),
		ProjectRoot:      req.ProjectRoot,
		DefaultWorkspace: h.workspaceRoot,
		TrashAdapter:     h.trashAdapter,
	})
	if err != nil {
		switch {
		case errors.Is(err, skills.ErrSkillDeleteNotFound):
			writeErrorJSON(w, http.StatusNotFound, "SKILL_NOT_FOUND", err.Error())
		case errors.Is(err, skills.ErrSkillDeleteSourceUnsupported):
			writeErrorJSON(w, http.StatusBadRequest, "SKILL_DELETE_SOURCE_UNSUPPORTED", err.Error())
		case errors.Is(err, skills.ErrSkillDeleteTrashUnavailable):
			writeErrorJSON(w, http.StatusConflict, "SKILL_DELETE_TRASH_UNAVAILABLE", err.Error())
		default:
			writeErrorJSON(w, http.StatusBadRequest, "SKILL_DELETE_FAILED", err.Error())
		}
		return
	}

	// Keep the live Catalog synchronized before the Desktop receives success.
	// Project previews for other roots remain read-only and are refreshed from
	// disk by the caller's follow-up GET.
	h.registry.ReloadLocalSources(h.skillSourceDirs(h.workspaceRoot))
	writeJSON(w, http.StatusOK, deleteSkillResponse{
		ID:           result.ID,
		SourceOrigin: string(result.Source),
		OriginalPath: result.OriginalPath,
		TrashedPath:  result.Trash.TrashedPath,
		Message:      fmt.Sprintf("%s skill moved to Trash", result.ID),
	})
}

func (h skillHandlers) skillSourceDirs(projectRoot string) map[skills.SourceOrigin]string {
	return skills.LocalSourceDirs(projectRoot)
}

func (h skillHandlers) skillSourceDirsForProjectRoots(projectRoots []string) map[skills.SourceOrigin]string {
	return skills.LocalSourceDirsForProjectRoots(projectRoots)
}

// handleExecuteSkill triggers direct execution of a skill by ID.
// POST /v1/skills/{id}/execute with JSON body of arguments.
func (h skillHandlers) handleEnableSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "skill id is required")
		return
	}
	h.registry.SetDisabledByPolicy(id, false, "")
	h.registry.ReloadLocalSources(h.skillSourceDirs(h.workspaceRoot))
	writeJSON(w, http.StatusOK, skillStateResponse{ID: id, State: string(skills.StateLoaded)})
}

func (h skillHandlers) handleDisableSkill(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "skill id is required")
		return
	}
	h.registry.SetDisabledByPolicy(id, true, "disabled by user")
	h.registry.ReloadLocalSources(h.skillSourceDirs(h.workspaceRoot))
	writeJSON(w, http.StatusOK, skillStateResponse{ID: id, State: string(skills.StateDisabledByPolicy)})
}

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

	if def.State != skills.StateLoaded {
		writeErrorJSON(w, http.StatusConflict, "SKILL_UNAVAILABLE", def.UnavailableReason)
		return
	}

	writeJSON(w, http.StatusOK, skillInfoFromDefinition(def))
}

func skillInfoFromDefinition(def *skills.SkillDefinition) skillInfo {
	info := skillInfo{
		ID:                 def.ID,
		CatalogEntryID:     skills.CatalogEntryID(def),
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
		UserInvocable:      def.UserInvocable,
		ModelInvocable:     !def.DisableModelInvocation,
		Diagnostics:        skillDiagnostics(def),
	}
	if def.LastInvokedAt != nil && !def.LastInvokedAt.IsZero() {
		info.LastInvokedAt = def.LastInvokedAt.UTC().Format(time.RFC3339Nano)
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

func skillDiagnostics(def *skills.SkillDefinition) []skillDiagnosticInfo {
	if def == nil {
		return nil
	}
	result := make([]skillDiagnosticInfo, 0, len(def.Diagnostics)+len(def.DeprecatedWarnings)+1)
	seen := map[string]bool{}
	appendDiagnostic := func(value skillDiagnosticInfo) {
		key := value.Code + "\x00" + value.Message + "\x00" + value.Severity
		if strings.TrimSpace(value.Message) == "" || seen[key] {
			return
		}
		// The disabled-by-policy state is already represented by the card's
		// toggle switch; don't repeat it as a red diagnostic line.
		if value.Code == "skill_disabled_by_policy" {
			return
		}
		seen[key] = true
		result = append(result, value)
	}
	for _, diagnostic := range def.Diagnostics {
		appendDiagnostic(skillDiagnosticInfo{Code: diagnostic.Code, Message: diagnostic.Message, Severity: diagnostic.Severity})
	}
	if reason := strings.TrimSpace(def.UnavailableReason); reason != "" {
		code := strings.TrimSpace(def.UnavailableCode)
		if code == "" {
			code = "skill_" + string(def.State)
		}
		if code != "skill_disabled_by_policy" {
			appendDiagnostic(skillDiagnosticInfo{Code: code, Message: reason, Severity: "error"})
		}
	}
	for _, warning := range def.DeprecatedWarnings {
		if warning = strings.TrimSpace(warning); warning != "" {
			appendDiagnostic(skillDiagnosticInfo{Code: "skill_compatibility_note", Message: warning, Severity: "warning"})
		}
	}
	return result
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
	case skills.SourceAgents:
		return "agents"
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
