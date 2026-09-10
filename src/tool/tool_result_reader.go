package tool

import "context"

const ToolResultReadMaxChars = 2048

type ToolResultReadRequest struct {
	StorageKey string `json:"storage_key"`
	Offset     int    `json:"offset,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type ToolResultPage struct {
	Content    string
	Offset     int
	NextOffset int
	TotalChars int
	ByteSize   int
	SHA256     string
	HasMore    bool
}

// ToolResultReader accepts thread/Agent identity only from the host context.
// Storage keys are opaque receipts, never arbitrary filesystem paths.
type ToolResultReader interface {
	ReadToolResult(context.Context, string, string, ToolResultReadRequest) (ToolResultPage, error)
}

type ToolResultReadError struct{ Code, Message string }

func (e *ToolResultReadError) Error() string { return e.Code + ": " + e.Message }
