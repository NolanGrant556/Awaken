package compactengine

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type CompactionReason string

const (
	CompactionReasonManual CompactionReason = "manual"
	CompactionReasonAuto   CompactionReason = "auto"
)

type SkipReason string

const (
	SkipReasonNone                 SkipReason = ""
	SkipReasonUnderBudget          SkipReason = "under_budget"
	SkipReasonMissingBudget        SkipReason = "missing_budget"
	SkipReasonNoCompressibleRegion SkipReason = "no_compressible_region"
	SkipReasonNoTokenSavings       SkipReason = "no_token_savings"
)

type MediaKind string

const (
	MediaImage    MediaKind = "image"
	MediaDocument MediaKind = "document"
	MediaAudio    MediaKind = "audio"
	MediaVideo    MediaKind = "video"
	MediaUnknown  MediaKind = "media"
)

type ToolCallMarker struct {
	ID            string
	Name          string
	Arguments     string
	TokenEstimate int
}

type ToolResultMarker struct {
	ID            string
	Name          string
	IsError       bool
	TokenEstimate int
}

type AttachmentMarker struct {
	ID            string
	Kind          string
	Name          string
	TokenEstimate int
}

type MediaMarker struct {
	ID          string
	Kind        MediaKind
	Description string
	Payload     string
}

type CompactBoundaryMetadata struct {
	ID                string
	Reason            CompactionReason
	StartIndex        int
	EndIndex          int
	SourceIDs         []string
	PreCompactTokens  int
	SelectedTokens    int
	SummaryTokens     int
	PostCompactTokens int
	ProtectedIndices  []int
}

type Message struct {
	ID               string
	SourceID         string
	Role             Role
	Content          string
	TokenEstimate    int
	ToolCalls        []ToolCallMarker
	ToolResult       *ToolResultMarker
	Attachments      []AttachmentMarker
	Media            []MediaMarker
	CompactBoundary  *CompactBoundaryMetadata
	IsCompactSummary bool
}

type Config struct {
	ContextWindowTokens   int
	TokenBudget           int
	SummaryTokenEstimate  int
	BoundaryTokenEstimate int
	LastTurnsToKeep       int
}

func DefaultConfig() Config {
	return Config{
		SummaryTokenEstimate:  750,
		BoundaryTokenEstimate: 1,
		LastTurnsToKeep:       4,
	}
}

type CompactionPlan struct {
	ShouldCompact         bool
	SkipReason            SkipReason
	Reason                CompactionReason
	StartIndex            int
	EndIndex              int
	OriginalTokens        int
	TargetTokenBudget     int
	ContextWindowTokens   int
	TokensToSave          int
	SelectedTokens        int
	SummaryTokenEstimate  int
	BoundaryTokenEstimate int
	EstimatedPostTokens   int
	StillOverLimit        bool
	ProtectedIndices      []int
	ProtectedSourceIDs    []string
	SelectedSourceIDs     []string
}

type CompactionMetrics struct {
	OriginalTokens      int
	PostCompactTokens   int
	TokensSaved         int
	SummaryTokens       int
	OriginalMessages    int
	PostCompactMessages int
	CompactedMessages   int
	CompressionRatio    float64
	WasCompacted        bool
	StillOverLimit      bool
}

type CompactionResult struct {
	Plan           CompactionPlan
	Messages       []Message
	Boundary       CompactBoundaryMetadata
	SummaryMessage Message
	Metrics        CompactionMetrics
	StillOverLimit bool
}

type SummaryInput struct {
	Plan     CompactionPlan
	Messages []Message
	Material string
}

type Summarizer interface {
	Summarize(input SummaryInput) (string, error)
}

const FallbackSummaryPrefix = "[CONTEXT SUMMARY]:"
