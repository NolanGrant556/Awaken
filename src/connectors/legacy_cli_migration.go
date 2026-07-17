package connectors

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"awaken/src/filestore"
	"awaken/src/mcpclient"
	"awaken/src/tool"
)

const LegacyCLIReviewRequired = "legacy_mcp_review_required"

type LegacyCLIMigrationReport struct {
	Removed        []string
	ReviewRequired []string
	BackupPath     string
}

// MigrateLegacyBuiltinCLIConfigs removes only records carrying the exact
// provider identity and transport signature emitted by the old built-in CLI
// MCP factories. Same-name manual records are preserved for user review.
func MigrateLegacyBuiltinCLIConfigs() (LegacyCLIMigrationReport, error) {
	report := LegacyCLIMigrationReport{}
	err := mcpclient.MutateUserConfig(func(cfg *mcpclient.ConfigFile) (bool, error) {
		for _, candidate := range legacyCLICandidates() {
			actual, exists := cfg.Servers[candidate.key]
			if !exists {
				continue
			}
			if legacyCLIConfigMatches(actual, candidate) {
				report.Removed = append(report.Removed, candidate.key)
				continue
			}
			report.ReviewRequired = append(report.ReviewRequired, candidate.key)
		}
		if len(report.Removed) == 0 {
			return false, nil
		}

		before, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return false, fmt.Errorf("serialize legacy CLI migration backup: %w", err)
		}
		backupPath, err := writeLegacyCLIBackup(append(before, '\n'))
		if err != nil {
			return false, err
		}
		report.BackupPath = backupPath
		for _, key := range report.Removed {
			delete(cfg.Servers, key)
		}
		return true, nil
	})
	sort.Strings(report.Removed)
	sort.Strings(report.ReviewRequired)
	return report, err
}

type legacyCLICandidate struct {
	key         string
	displayName string
	command     string
	args        []string
	providerID  tool.ProviderID
}

func legacyCLICandidates() []legacyCLICandidate {
	return []legacyCLICandidate{
		{key: "dingtalk-workspace", displayName: "钉钉CLI", command: "dingtalk-workspace-cli", args: []string{"mcp"}, providerID: "connector:dingtalk-workspace"},
		{key: "lark", displayName: "飞书CLI", command: "lark-cli", args: []string{"mcp"}, providerID: "connector:lark-cli"},
	}
}

func legacyCLIConfigMatches(actual mcpclient.McpServerConfig, candidate legacyCLICandidate) bool {
	if actual.Stdio == nil || actual.HTTP != nil || actual.SSE != nil || actual.WS != nil || actual.InProc != nil {
		return false
	}
	if actual.DisplayName != candidate.displayName || actual.Stdio.Command != candidate.command || !reflect.DeepEqual(actual.Stdio.Args, candidate.args) {
		return false
	}
	if actual.Stdio.Cwd != "" || len(actual.Stdio.Env) != 0 || actual.Provider == nil {
		return false
	}
	return actual.Provider.ProviderID == candidate.providerID && actual.Provider.Source == tool.SourceConnector
}

func writeLegacyCLIBackup(data []byte) (string, error) {
	configPath := mcpclient.UserConfigPath()
	awakenHome := filepath.Dir(filepath.Dir(configPath))
	backupDir := filepath.Join(awakenHome, "backups", "migrations", "cli-connectors-v1")
	if err := os.MkdirAll(backupDir, filestore.DirPerm); err != nil {
		return "", fmt.Errorf("create legacy CLI migration backup directory: %w", err)
	}
	if err := os.Chmod(backupDir, filestore.DirPerm); err != nil {
		return "", fmt.Errorf("protect legacy CLI migration backup directory: %w", err)
	}
	digest := sha256.Sum256(data)
	backupPath := filepath.Join(backupDir, "mcp-servers."+hex.EncodeToString(digest[:])+".json")
	if _, err := os.Stat(backupPath); err == nil {
		return backupPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect legacy CLI migration backup: %w", err)
	}
	if err := filestore.WriteFileAtomic(backupPath, data); err != nil {
		return "", fmt.Errorf("write legacy CLI migration backup: %w", err)
	}
	return backupPath, nil
}
