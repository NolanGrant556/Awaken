package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"awaken/src/mcpclient"
)

// connectorsJSONSchema is the on-disk shape of ~/.awaken/connectors.json.
type connectorsJSONSchema struct {
	Connectors []localConnectorEntry `json:"connectors"`
}

// localConnectorEntry is a single entry in the local connectors config file.
type localConnectorEntry struct {
	ID          string                  `json:"id"`
	DisplayName string                  `json:"displayName"`
	Description string                  `json:"description,omitempty"`
	Category    Category                `json:"category"`
	Provider    string                  `json:"provider,omitempty"`
	AuthType    AuthType                `json:"authType,omitempty"`
	MCPConfig   mcpclient.McpServerConfig `json:"mcp"`
}

// localSource implements ConnectorSource for user-defined local connectors.
type localSource struct {
	defs  []ConnectorDefinition
	logger Logger
}

// NewLocalSource loads connectors from ~/.awaken/connectors.json. A missing
// file is not an error; it returns an empty source.
func NewLocalSource(logger Logger) (*localSource, error) {
	if logger == nil {
		logger = noopLogger{}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	path := filepath.Join(home, ".awaken", "connectors.json")
	defs, err := loadLocalConnectors(path, logger)
	if err != nil {
		return nil, err
	}
	return &localSource{defs: defs, logger: logger}, nil
}

// List implements ConnectorSource.
func (s *localSource) List(_ context.Context) ([]ConnectorDefinition, error) {
	return s.defs, nil
}

// Origin implements ConnectorSource.
func (s *localSource) Origin() SourceOrigin { return SourceLocal }

// Priority implements ConnectorSource. Local has priority 3 (after builtin=0, team=2).
func (s *localSource) Priority() int { return 3 }

// loadLocalConnectors reads and validates the connectors.json file.
func loadLocalConnectors(path string, logger Logger) ([]ConnectorDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg connectorsJSONSchema
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	var defs []ConnectorDefinition
	for i, entry := range cfg.Connectors {
		def, err := validateLocalEntry(entry, i)
		if err != nil {
			logger.Warnf("local connector entry %d: %v, skipping", i, err)
			continue
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// validateLocalEntry checks a local connector entry and converts it to a
// ConnectorDefinition.
func validateLocalEntry(entry localConnectorEntry, index int) (ConnectorDefinition, error) {
	if entry.ID == "" {
		return ConnectorDefinition{}, fmt.Errorf("entry %d: id is required", index)
	}
	if entry.DisplayName == "" {
		return ConnectorDefinition{}, fmt.Errorf("entry %d (%s): displayName is required", index, entry.ID)
	}
	if entry.Category == "" {
		return ConnectorDefinition{}, fmt.Errorf("entry %d (%s): category is required", index, entry.ID)
	}
	if !entry.MCPConfig.Valid() {
		return ConnectorDefinition{}, fmt.Errorf("entry %d (%s): mcp config is invalid (must declare exactly one transport)", index, entry.ID)
	}

	authType := entry.AuthType
	if authType == "" {
		authType = AuthOAuth2
	}

	raw := entry.MCPConfig
	return ConnectorDefinition{
		ID:            entry.ID,
		DisplayName:   entry.DisplayName,
		Description:   entry.Description,
		Category:      entry.Category,
		Provider:      entry.Provider,
		AuthType:      authType,
		TrustLevel:    TrustUserLocal,
		SourceOrigin:  SourceLocal,
		MCPRawConfig:  &raw,
	}, nil
}