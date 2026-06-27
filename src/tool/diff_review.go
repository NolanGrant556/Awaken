package tool

type DiffReviewMetadata struct {
	AffectedPaths []string              `json:"affectedPaths,omitempty"`
	FileCounts    DiffReviewFileCounts  `json:"fileCounts"`
	LineCounts    DiffReviewLineCounts  `json:"lineCounts"`
	Risk          DiffReviewRiskSummary `json:"risk"`
}

type DiffReviewFileCounts struct {
	Added    int `json:"added,omitempty"`
	Modified int `json:"modified,omitempty"`
	Deleted  int `json:"deleted,omitempty"`
}

type DiffReviewLineCounts struct {
	Added   int `json:"added,omitempty"`
	Deleted int `json:"deleted,omitempty"`
}

type DiffReviewRiskSummary struct {
	Level   string   `json:"level,omitempty"`
	Summary string   `json:"summary,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
}
