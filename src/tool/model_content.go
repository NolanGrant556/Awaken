package tool

import (
	"context"
	"strings"
)

// ModelCapabilities is the host-owned capability contract used by tools and
// providers. It must come from the selected model profile, never from tool
// arguments supplied by the model.
type ModelCapabilities struct {
	SupportsVision    bool `json:"supports_vision"`
	SupportsNativePDF bool `json:"supports_native_pdf"`
	// SupportsTools remains a protocol compatibility field. Current Desktop
	// profiles normalize it to "ready"; non-Desktop callers may still use an
	// explicit non-ready value to suppress model-visible tool schemas.
	SupportsTools string `json:"supports_tools,omitempty"`
	// SupportsComputerUse indicates the model profile is allowed to use the
	// computer_observe/act/verify lane. It is host-owned and does not by itself
	// guarantee a CUA Host is available.
	SupportsComputerUse bool `json:"supports_computer_use"`
}

// ModelContentPart describes non-text content returned by a tool. DataBase64
// is deliberately memory-only: journals, AG-UI events, audits and logs retain
// StorageKey but never inline the blob.
type ModelContentPart struct {
	Type        string `json:"type"`
	MimeType    string `json:"mime_type"`
	DisplayName string `json:"display_name,omitempty"`
	ByteSize    int64  `json:"byte_size,omitempty"`
	Page        int    `json:"page,omitempty"`
	StorageKey  string `json:"storage_key,omitempty"`
	DataBase64  string `json:"-"`
}

func CloneModelContent(parts []ModelContentPart) []ModelContentPart {
	if len(parts) == 0 {
		return nil
	}
	result := make([]ModelContentPart, len(parts))
	copy(result, parts)
	return result
}

// ModelContentStore persists and rehydrates runtime-owned tool result blobs.
// Implementations must validate storage keys beneath their owned root.
type ModelContentStore interface {
	PersistToolResult(context.Context, string, ToolExecutionIdentity, []ModelContentPart) ([]ModelContentPart, error)
	HydrateToolResult(context.Context, string, []ModelContentPart) ([]ModelContentPart, []OutcomeNotice)
}

type modelCapabilitiesContextKey struct{}
type modelContentStoreContextKey struct{}
type toolThreadIDContextKey struct{}

func ContextWithModelCapabilities(ctx context.Context, capabilities ModelCapabilities) context.Context {
	return context.WithValue(ctx, modelCapabilitiesContextKey{}, capabilities)
}

func ModelCapabilitiesFromContext(ctx context.Context) ModelCapabilities {
	if ctx == nil {
		return ModelCapabilities{}
	}
	capabilities, _ := ctx.Value(modelCapabilitiesContextKey{}).(ModelCapabilities)
	return capabilities
}

func ContextWithModelContentStore(ctx context.Context, store ModelContentStore) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, modelContentStoreContextKey{}, store)
}

func ModelContentStoreFromContext(ctx context.Context) (ModelContentStore, bool) {
	if ctx == nil {
		return nil, false
	}
	store, ok := ctx.Value(modelContentStoreContextKey{}).(ModelContentStore)
	return store, ok && store != nil
}

func ContextWithToolThreadID(ctx context.Context, threadID string) context.Context {
	if strings.TrimSpace(threadID) == "" {
		return ctx
	}
	return context.WithValue(ctx, toolThreadIDContextKey{}, strings.TrimSpace(threadID))
}

func ToolThreadIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	threadID, _ := ctx.Value(toolThreadIDContextKey{}).(string)
	return strings.TrimSpace(threadID)
}
