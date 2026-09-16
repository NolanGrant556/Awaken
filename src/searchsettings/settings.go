// Package searchsettings reads the host-owned search configuration at run boundaries.
package searchsettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Snapshot is a value: a running task cannot observe later settings writes.
type Snapshot struct {
	Exa    bool   `json:"exa"`
	Tavily bool   `json:"tavily"`
	Error  string `json:"error,omitempty"`
}

func (s Snapshot) Providers() string {
	names := []string{}
	if s.Exa {
		names = append(names, "exa")
	}
	if s.Tavily {
		names = append(names, "tavily")
	}
	return strings.Join(names, ",")
}

// Desktop and ACP use the same identity-scoped file. Environment configuration
// is only the fallback for hosts without a userData identity.
func Load() Snapshot {
	if root := strings.TrimSpace(os.Getenv("AWAKEN_USER_DATA_DIR")); root != "" {
		data, err := os.ReadFile(filepath.Join(root, "config", "search.json"))
		if os.IsNotExist(err) {
			return Snapshot{Exa: true}
		}
		if err != nil {
			return Snapshot{Error: "search settings could not be read"}
		}
		s, err := Parse(data)
		if err != nil {
			return Snapshot{Error: err.Error()}
		}
		return s
	}
	value, configured := os.LookupEnv("AWAKEN_WEB_SEARCH_PROVIDERS")
	if !configured {
		return Snapshot{Exa: true}
	}
	var s Snapshot
	for _, name := range strings.Split(value, ",") {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "exa":
			s.Exa = true
		case "tavily":
			s.Tavily = true
		}
	}
	return s
}

func Parse(data []byte) (Snapshot, error) {
	var file struct {
		Version   int `json:"version"`
		Providers map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return Snapshot{}, fmt.Errorf("search settings format is invalid")
	}
	if file.Version != 1 {
		return Snapshot{}, fmt.Errorf("search settings version is unsupported")
	}
	s := Snapshot{Exa: true}
	if exa, ok := file.Providers["exa"]; ok {
		s.Exa = exa.Enabled
	}
	s.Tavily = file.Providers["tavily"].Enabled
	return s, nil
}
