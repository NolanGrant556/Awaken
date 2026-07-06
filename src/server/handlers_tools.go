package server

import (
	"net/http"

	"awaken/src/tool"
)

// toolInfo is the wire shape for entries returned by GET /v1/tools.
type toolInfo struct {
	Name            string `json:"name"`
	DisplayName     string `json:"displayName,omitempty"`
	Source          string `json:"source"`
	Description     string `json:"description"`
	ReadOnly        bool   `json:"readOnly"`
	Destructive     bool   `json:"destructive"`
	OpenWorld       bool   `json:"openWorld"`
	RequiresAuth    bool   `json:"requiresAuth"`
	ServerName      string `json:"serverName,omitempty"`
	ToolName        string `json:"toolName,omitempty"`
	CapabilityClass string `json:"capabilityClass,omitempty"`
	TrustedOrigin   string `json:"trustedOrigin,omitempty"`
	FileBridgeScope string `json:"fileBridgeScope,omitempty"`
}

func handleListTools(w http.ResponseWriter, r *http.Request) {
	tools := tool.ListTools()
	out := make([]toolInfo, 0, len(tools))
	for _, t := range tools {
		def := t.Definition()
		info := toolInfo{
			Name:            def.Name,
			DisplayName:     def.DisplayName,
			Source:          string(def.Source),
			Description:     def.Description,
			ReadOnly:        def.Annotations.ReadOnly,
			Destructive:     def.Annotations.Destructive,
			OpenWorld:       def.Annotations.OpenWorld,
			RequiresAuth:    def.Annotations.RequiresAuth,
			ServerName:      def.Annotations.ServerName,
			ToolName:        def.Annotations.ToolName,
			CapabilityClass: string(def.Annotations.CapabilityClass),
			TrustedOrigin:   string(def.Annotations.TrustedOrigin),
			FileBridgeScope: string(def.Annotations.FileBridgeScope),
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}
