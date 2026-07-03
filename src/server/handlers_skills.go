package server

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
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
	Context            string          `json:"context"`
	ShadowedBy         string          `json:"shadowedBy,omitempty"`
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
	all := h.registry.ListAll()
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
			Context:            string(def.Context),
			ShadowedBy:         def.ShadowedBy,
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
	if strings.TrimSpace(h.workspaceRoot) == "" {
		writeErrorJSON(w, http.StatusServiceUnavailable, "WORKSPACE_UNAVAILABLE", "workspace root not configured")
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
	raw, sourceName, err := extractSkillMarkdown(req.FileName, data)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL_PACKAGE", err.Error())
		return
	}

	def, err := skills.ParseFrontmatter(string(raw))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_SKILL", err.Error())
		return
	}
	def.ID = skillIDFromUploadName(sourceName, def.Name)
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

	dir := filepath.Join(h.workspaceRoot, ".awaken", "skills")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "SKILL_INSTALL_FAILED", err.Error())
		return
	}
	path := filepath.Join(dir, def.ID+".md")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "SKILL_INSTALL_FAILED", err.Error())
		return
	}

	def.Source = skills.SourceProject
	def.FilePath = path
	def.State = skills.StateAvailable
	h.registry.RegisterSkill(def)

	writeJSON(w, http.StatusOK, installSkillResponse{
		Skill:   skillInfoFromDefinition(def),
		Path:    path,
		Message: "skill installed",
	})
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
		Context:            string(def.Context),
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

func extractSkillMarkdown(fileName string, data []byte) ([]byte, string, error) {
	lowerName := strings.ToLower(fileName)
	if strings.HasSuffix(lowerName, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, "", fmt.Errorf("open zip: %w", err)
		}
		var fallback *zip.File
		for _, file := range reader.File {
			clean := filepath.ToSlash(file.Name)
			if strings.EqualFold(clean, "SKILL.md") {
				fallback = file
				break
			}
			if strings.HasSuffix(strings.ToLower(clean), "/skill.md") && fallback == nil {
				fallback = file
			}
		}
		if fallback == nil {
			return nil, "", fmt.Errorf("zip must contain SKILL.md")
		}
		rc, err := fallback.Open()
		if err != nil {
			return nil, "", fmt.Errorf("open %s: %w", fallback.Name, err)
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", fallback.Name, err)
		}
		return raw, fileName, nil
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, "", fmt.Errorf("uploaded file is empty")
	}
	return data, fileName, nil
}

func skillIDFromUploadName(fileName string, skillName string) string {
	base := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if strings.EqualFold(base, "skill") || strings.TrimSpace(base) == "" {
		base = skillName
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(base)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "custom-skill"
	}
	return id
}
