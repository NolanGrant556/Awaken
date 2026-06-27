package toolsummary

import "time"

type ToolStatus string

const (
	StatusUnknown  ToolStatus = ""
	StatusSuccess  ToolStatus = "success"
	StatusError    ToolStatus = "error"
	StatusPartial  ToolStatus = "partial"
	StatusCanceled ToolStatus = "canceled"
)

type ToolUse struct {
	Name     string
	Input    any
	Output   any
	Status   ToolStatus
	Error    string
	Duration time.Duration
}

type Batch struct {
	Tools   []ToolUse
	Options Options
}

type Options struct {
	MaxLabelChars    int
	MaxValueChars    int
	WorkspaceRoot    string
	DisableRedaction bool
}

type Summary struct {
	Label        string
	Status       ToolStatus
	ToolCount    int
	ErrorCount   int
	PartialCount int
}

type Summarizer interface {
	Summarize(batch Batch) Summary
}

type DeterministicSummarizer struct{}

func Summarize(tools []ToolUse, opts Options) Summary {
	return DeterministicSummarizer{}.Summarize(Batch{
		Tools:   tools,
		Options: opts,
	})
}

func (opts Options) normalized() Options {
	if opts.MaxLabelChars <= 0 {
		opts.MaxLabelChars = 64
	}
	if opts.MaxValueChars <= 0 {
		opts.MaxValueChars = 300
	}
	return opts
}
