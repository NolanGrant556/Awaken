package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
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

func (h skillHandlers) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	projectRoot := strings.TrimSpace(h.workspaceRoot)
	if rawProjectRoot := strings.TrimSpace(r.URL.Query().Get("projectRoot")); rawProjectRoot != "" {
		_, resolved, _, _, err := skills.ResolveInstallTarget(skills.InstallTargetRequest{
			Scope:       string(skills.SourceProject),
			ProjectRoot: rawProjectRoot,
		})
		if err != nil {
			writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT_ROOT", err.Error())
			return
		}
		projectRoot = resolved
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

func (h skillHandlers) skillSourceDirs(projectRoot string) map[skills.SourceOrigin]string {
	return skills.LocalSourceDirs(projectRoot)
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
