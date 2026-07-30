package mediagenerate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"awaken/src/modelprofile"
)

const (
	providerID      = "volcengine-agent-plan"
	defaultAccount  = "volcengine-agent-plan-default"
	defaultEndpoint = "https://ark.cn-beijing.volces.com/api/plan/v3"
)

type storedConfiguration struct {
	ID                    string `json:"id"`
	ProviderID            string `json:"providerId"`
	AccountID             string `json:"accountId"`
	Capability            string `json:"capability"`
	ModelID               string `json:"modelId"`
	ModelKind             string `json:"modelKind"`
	Enabled               bool   `json:"enabled"`
	ConfigurationRevision uint64 `json:"configurationRevision"`
}

type storedConfigurationFile struct {
	Version        int                   `json:"version"`
	Configurations []storedConfiguration `json:"configurations"`
}

type runtimeConfiguration struct {
	storedConfiguration
	APIKey             string
	CredentialRevision uint64
	BaseURL            string
}

func loadRuntimeConfigurations() []runtimeConfiguration {
	root := strings.TrimSpace(os.Getenv("AWAKEN_HOME"))
	if root == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(root, "config", "agent-plan-media-tools.json"))
	if err != nil {
		return nil
	}
	var file storedConfigurationFile
	if json.Unmarshal(raw, &file) != nil || file.Version != 1 {
		return nil
	}
	result := make([]runtimeConfiguration, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, configuration := range file.Configurations {
		configuration.ID = strings.TrimSpace(configuration.ID)
		configuration.ProviderID = strings.TrimSpace(configuration.ProviderID)
		configuration.AccountID = strings.TrimSpace(configuration.AccountID)
		configuration.Capability = strings.TrimSpace(configuration.Capability)
		configuration.ModelID = strings.TrimSpace(configuration.ModelID)
		configuration.ModelKind = strings.TrimSpace(configuration.ModelKind)
		if !configuration.Enabled || configuration.ID == "" || configuration.ModelID == "" || configuration.ConfigurationRevision == 0 {
			continue
		}
		if configuration.ProviderID != providerID || configuration.AccountID != defaultAccount {
			continue
		}
		expectedKind := ""
		switch configuration.Capability {
		case "media.image.generate":
			expectedKind = "image"
		case "media.video.generate":
			expectedKind = "video"
		default:
			continue
		}
		if configuration.ModelKind != expectedKind {
			continue
		}
		catalogKind, err := modelprofile.ResolveAgentPlanModelKind(root, configuration.AccountID, configuration.ModelID)
		if err != nil || catalogKind != expectedKind {
			continue
		}
		if _, exists := seen[configuration.Capability]; exists {
			continue
		}
		credential, err := modelprofile.ResolveAgentPlanCredential(root, configuration.AccountID)
		if err != nil {
			continue
		}
		seen[configuration.Capability] = struct{}{}
		result = append(result, runtimeConfiguration{
			storedConfiguration: configuration,
			APIKey:              credential.APIKey,
			CredentialRevision:  credential.CredentialRevision,
			BaseURL:             defaultEndpoint,
		})
	}
	return result
}
