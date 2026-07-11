package server

import (
	"net/http"

	"awaken/src/tool"
)

// toolInfo is the wire shape for entries returned by GET /v1/tools.
type toolInfo struct {
	SchemaVersion      int                       `json:"schemaVersion"`
	Name               string                    `json:"name"`
	DisplayName        string                    `json:"displayName,omitempty"`
	Source             string                    `json:"source"`
	DefinitionQuality  string                    `json:"definitionQuality"`
	DefinitionWarnings []string                  `json:"definitionWarnings,omitempty"`
	Description        string                    `json:"description"`
	ModelDescription   string                    `json:"modelDescription"`
	Purpose            string                    `json:"purpose"`
	WhenToUse          []string                  `json:"whenToUse"`
	WhenNotToUse       []string                  `json:"whenNotToUse"`
	InputSchema        tool.ToolParametersSchema `json:"inputSchema"`
	OutputSchema       tool.ToolParam            `json:"outputSchema"`
	SideEffects        []tool.ToolSideEffect     `json:"sideEffects"`
	Permissions        tool.ToolPermissionSpec   `json:"permissions"`
	Examples           []tool.ToolExample        `json:"examples"`
	FailureModes       []tool.ToolFailureMode    `json:"failureModes"`
	ReadOnly           bool                      `json:"readOnly"`
	Destructive        bool                      `json:"destructive"`
	OpenWorld          bool                      `json:"openWorld"`
	RequiresAuth       bool                      `json:"requiresAuth"`
	ServerName         string                    `json:"serverName,omitempty"`
	ToolName           string                    `json:"toolName,omitempty"`
	CapabilityClass    string                    `json:"capabilityClass,omitempty"`
	TrustedOrigin      string                    `json:"trustedOrigin,omitempty"`
	FileBridgeScope    string                    `json:"fileBridgeScope,omitempty"`
}

func handleListTools(w http.ResponseWriter, r *http.Request) {
	registered := tool.ListRegisteredTools()
	out := make([]toolInfo, 0, len(registered))
	for _, entry := range registered {
		def := entry.Definition
		info := toolInfo{
			SchemaVersion:      def.SchemaVersion,
			Name:               def.Name,
			DisplayName:        def.DisplayName,
			Source:             string(entry.Origin),
			DefinitionQuality:  string(entry.Quality),
			DefinitionWarnings: append([]string(nil), entry.Warnings...),
			Description:        def.Description,
			ModelDescription:   tool.ModelDescription(def),
			Purpose:            def.Purpose,
			WhenToUse:          append([]string(nil), def.WhenToUse...),
			WhenNotToUse:       append([]string(nil), def.WhenNotToUse...),
			InputSchema:        def.InputSchema,
			OutputSchema:       def.OutputSchema,
			SideEffects:        def.SideEffects,
			Permissions:        def.Permissions,
			Examples:           append([]tool.ToolExample{}, def.Examples...),
			FailureModes:       append([]tool.ToolFailureMode{}, def.FailureModes...),
			ReadOnly:           def.Annotations.ReadOnly,
			Destructive:        def.Annotations.Destructive,
			OpenWorld:          def.Annotations.OpenWorld,
			RequiresAuth:       def.Annotations.RequiresAuth,
			ServerName:         def.Annotations.ServerName,
			ToolName:           def.Annotations.ToolName,
			CapabilityClass:    string(def.Annotations.CapabilityClass),
			TrustedOrigin:      string(def.Annotations.TrustedOrigin),
			FileBridgeScope:    string(def.Annotations.FileBridgeScope),
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}
