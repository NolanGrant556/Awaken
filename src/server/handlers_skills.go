package server

import (
	"net/http"

	"awaken/src/skills"
)

// skillHandlers wires the /v1/skills HTTP surface to a skills.SkillRegistry.
type skillHandlers struct {
	registry *skills.SkillRegistry
}

// skillInfo is the wire shape for entries returned by GET /v1/skills.
type skillInfo struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	DisplayName       string `json:"displayName,omitempty"`
	Description       string `json:"description"`
	WhenToUse         string `json:"whenToUse,omitempty"`
	State             string `json:"state"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	SourceOrigin      string `json:"sourceOrigin"`
	Context           string `json:"context"`
	ShadowedBy        string `json:"shadowedBy,omitempty"`
}

func (h skillHandlers) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "SKILLS_UNAVAILABLE", "skill registry not configured")
		return
	}
	all := h.registry.ListAll()
	out := make([]skillInfo, 0, len(all))
	for _, def := range all {
		out = append(out, skillInfo{
			ID:                def.ID,
			Name:              def.Name,
			DisplayName:       def.DisplayName,
			Description:       def.Description,
			WhenToUse:         def.WhenToUse,
			State:             string(def.State),
			UnavailableReason: def.UnavailableReason,
			SourceOrigin:      string(def.Source),
			Context:           string(def.Context),
			ShadowedBy:        def.ShadowedBy,
		})
	}
	writeJSON(w, http.StatusOK, out)
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

	// For now, return the skill definition as a preview. Full execution
	// integration with the runtime will come in a follow-up change.
	writeJSON(w, http.StatusOK, skillInfo{
		ID:                def.ID,
		Name:              def.Name,
		DisplayName:       def.DisplayName,
		Description:       def.Description,
		WhenToUse:         def.WhenToUse,
		State:             string(def.State),
		UnavailableReason: def.UnavailableReason,
		SourceOrigin:      string(def.Source),
		Context:           string(def.Context),
	})
}