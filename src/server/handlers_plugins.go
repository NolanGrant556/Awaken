package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"awaken/src/mcpclient"
	"awaken/src/plugins"
	"awaken/src/skills"
)

type pluginHandlers struct {
	store         *plugins.Store
	manager       *mcpclient.Manager
	skillRegistry *skills.SkillRegistry
}

type pluginInfo struct {
	Name        string               `json:"name"`
	Version     string               `json:"version,omitempty"`
	Description string               `json:"description,omitempty"`
	Author      *plugins.Author      `json:"author,omitempty"`
	Homepage    string               `json:"homepage,omitempty"`
	Repository  string               `json:"repository,omitempty"`
	License     string               `json:"license,omitempty"`
	Keywords    []string             `json:"keywords,omitempty"`
	Enabled     bool                 `json:"enabled"`
	Source      plugins.Source       `json:"source"`
	Digest      string               `json:"digest,omitempty"`
	InstalledAt string               `json:"installedAt,omitempty"`
	SkillRoot   string               `json:"skillRoot,omitempty"`
	Skills      []skillInfo          `json:"skills,omitempty"`
	Servers     []pluginServerInfo   `json:"servers,omitempty"`
	Diagnostics []plugins.Diagnostic `json:"diagnostics,omitempty"`
	SkillCount  int                  `json:"skillCount"`
	ServerCount int                  `json:"serverCount"`
}

type pluginServerInfo struct {
	ShortName   string              `json:"shortName"`
	Identity    string              `json:"identity"`
	Transport   string              `json:"transport,omitempty"`
	State       string              `json:"state,omitempty"`
	Enabled     bool                `json:"enabled"`
	ErrorReason string              `json:"errorReason,omitempty"`
	Invalid     *plugins.Diagnostic `json:"invalid,omitempty"`
}

type installPluginRequest struct {
	Source  plugins.Source `json:"source"`
	Replace bool           `json:"replace"`
}

type installPluginResponse struct {
	Plugin  pluginInfo `json:"plugin"`
	Message string     `json:"message"`
}

func (h pluginHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	listed, err := h.store.List()
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	out := make([]pluginInfo, 0, len(listed))
	for _, plugin := range listed {
		out = append(out, h.toPluginInfo(plugin, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h pluginHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	plugin, err := h.store.Get(name)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.toPluginInfo(plugin, true))
}

func (h pluginHandlers) handleInstall(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	var req installPluginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	result, err := h.store.Install(r.Context(), plugins.InstallRequest{Source: req.Source, Replace: req.Replace})
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, installPluginResponse{
		Plugin:  h.toPluginInfo(result.Plugin, true),
		Message: result.Message,
	})
}

func (h pluginHandlers) handleEnable(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, true)
}

func (h pluginHandlers) handleDisable(w http.ResponseWriter, r *http.Request) {
	h.setEnabled(w, r, false)
}

func (h pluginHandlers) setEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !h.ensureStore(w) {
		return
	}
	plugin, err := h.store.SetEnabled(strings.TrimSpace(r.PathValue("name")), enabled)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, h.toPluginInfo(plugin, true))
}

func (h pluginHandlers) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	result, err := h.store.Upgrade(r.Context(), strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, installPluginResponse{
		Plugin:  h.toPluginInfo(result.Plugin, true),
		Message: result.Message,
	})
}

func (h pluginHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	trashed, err := h.store.Uninstall(strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, map[string]string{
		"trashedPath": trashed,
		"message":     "plugin moved to Trash",
	})
}

func (h pluginHandlers) handleServerEnable(w http.ResponseWriter, r *http.Request) {
	h.setServerEnabled(w, r, true)
}

func (h pluginHandlers) handleServerDisable(w http.ResponseWriter, r *http.Request) {
	h.setServerEnabled(w, r, false)
}

func (h pluginHandlers) setServerEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !h.ensureStore(w) {
		return
	}
	plugin, err := h.store.SetServerEnabled(strings.TrimSpace(r.PathValue("name")), strings.TrimSpace(r.PathValue("short")), enabled)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, h.toPluginInfo(plugin, true))
}

func (h pluginHandlers) ensureStore(w http.ResponseWriter) bool {
	if h.store == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "PLUGINS_UNAVAILABLE", "plugin store not configured")
		return false
	}
	if err := h.store.Available(); err != nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "PLUGIN_STATE_CORRUPT", err.Error())
		return false
	}
	return true
}

func (h pluginHandlers) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, plugins.ErrNotFound):
		writeErrorJSON(w, http.StatusNotFound, "PLUGIN_NOT_FOUND", err.Error())
	case errors.Is(err, plugins.ErrAlreadyExists):
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_ALREADY_EXISTS", err.Error())
	case errors.Is(err, plugins.ErrDigestChanged):
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_DIGEST_CHANGED", err.Error())
	case errors.Is(err, plugins.ErrReplaceTrashUnavailable):
		writeErrorJSON(w, http.StatusConflict, "PLUGIN_TRASH_UNAVAILABLE", err.Error())
	case errors.Is(err, plugins.ErrUnsupportedSchema), errors.Is(err, plugins.ErrManifestInvalid), errors.Is(err, plugins.ErrMCPInvalid):
		writeErrorJSON(w, http.StatusBadRequest, "PLUGIN_INVALID", err.Error())
	case errors.Is(err, plugins.ErrStateCorrupt):
		writeErrorJSON(w, http.StatusServiceUnavailable, "PLUGIN_STATE_CORRUPT", err.Error())
	default:
		writeErrorJSON(w, http.StatusBadRequest, "PLUGIN_REQUEST_FAILED", err.Error())
	}
}

func (h pluginHandlers) refreshRuntime() {
	if h.store == nil || h.store.Available() != nil {
		skills.SetPluginSkillRoots(nil)
		return
	}
	skills.SetPluginSkillRoots(h.store.EnabledSkillRoots())
	if h.manager == nil {
		return
	}
	desired := h.store.PluginMCPServers()
	desiredNames := make(map[string]mcpclient.ScopedMcpServerConfig, len(desired))
	for _, scoped := range desired {
		desiredNames[scoped.Name] = scoped
	}
	for _, snap := range h.manager.Snapshot() {
		if plugins.IsPluginServerIdentity(snap.Name) {
			if _, ok := desiredNames[snap.Name]; !ok {
				_ = h.manager.Remove(snap.Name)
			}
		}
	}
	ctx := context.Background()
	for _, scoped := range desired {
		_ = h.manager.RestartWithConfig(ctx, scoped)
	}
}

func (h pluginHandlers) toPluginInfo(plugin *plugins.InstalledPlugin, includeDetails bool) pluginInfo {
	info := pluginInfo{
		Name:        plugin.Name,
		Version:     plugin.Manifest.Version,
		Description: plugin.Manifest.Description,
		Author:      plugin.Manifest.Author,
		Homepage:    plugin.Manifest.Homepage,
		Repository:  plugin.Manifest.Repository,
		License:     plugin.Manifest.License,
		Keywords:    plugin.Manifest.Keywords,
		Enabled:     plugin.Enabled,
		Source:      plugin.Source,
		Digest:      plugin.Digest,
		SkillRoot:   plugin.SkillRoot,
		Diagnostics: plugin.Diagnostics,
		ServerCount: len(plugin.MCPServers),
	}
	if !plugin.InstalledAt.IsZero() {
		info.InstalledAt = plugin.InstalledAt.UTC().Format(time.RFC3339)
	}
	if includeDetails {
		info.Skills = h.pluginSkills(plugin)
		info.SkillCount = len(info.Skills)
		info.Servers = h.pluginServers(plugin)
	} else {
		info.SkillCount = h.pluginSkillCount(plugin)
		info.Servers = h.pluginServers(plugin)
	}
	return info
}

func (h pluginHandlers) pluginSkillCount(plugin *plugins.InstalledPlugin) int {
	return len(h.pluginSkills(plugin))
}

func (h pluginHandlers) pluginSkills(plugin *plugins.InstalledPlugin) []skillInfo {
	if plugin == nil || plugin.SkillRoot == "" {
		return nil
	}
	defs := skills.LoadDir(plugin.SkillRoot, skills.SourcePlugin)
	out := make([]skillInfo, 0, len(defs))
	for _, def := range defs {
		out = append(out, skillInfoFromDefinition(def))
	}
	return out
}

func (h pluginHandlers) pluginServers(plugin *plugins.InstalledPlugin) []pluginServerInfo {
	out := make([]pluginServerInfo, 0, len(plugin.MCPServers))
	for _, server := range plugin.MCPServers {
		item := pluginServerInfo{
			ShortName: server.ShortName,
			Identity:  server.Identity,
			Enabled:   server.Enabled,
			Invalid:   server.Invalid,
			Transport: string(server.Config.TransportKind()),
		}
		if h.manager != nil {
			if snap, err := h.manager.SnapshotServer(server.Identity); err == nil {
				item.State = string(snap.State)
				item.ErrorReason = snap.ErrorReason
				if snap.Transport != "" {
					item.Transport = string(snap.Transport)
				}
			}
		}
		out = append(out, item)
	}
	return out
}
