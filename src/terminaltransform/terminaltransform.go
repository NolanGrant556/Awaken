package terminaltransform

// Context is the terminal command output payload offered to each transformer.
type Context struct {
	Command    string
	Output     string
	ReturnCode int
	TaskID     string
	EnvType    string
}

// Transformer may return a replacement output. The bool reports whether the
// returned string is a valid replacement; an empty string is valid when true.
type Transformer func(Context) (string, bool, error)

// Result describes the output selected by Apply and any fail-open errors.
type Result struct {
	Output      string
	Transformed bool
	// TransformerIndex is -1 when no transformer provided a replacement.
	TransformerIndex int
	Errors           []string
}

// Apply runs transformers in order against the original context output.
// The first transformer to return ok=true wins. Transformer errors are recorded
// and skipped so terminal output remains fail-open.
func Apply(ctx Context, transformers ...Transformer) Result {
	result := Result{
		Output:           ctx.Output,
		TransformerIndex: -1,
	}

	for i, transformer := range transformers {
		output, ok, err := transformer(ctx)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}
		if !ok {
			continue
		}

		result.Output = output
		result.Transformed = true
		result.TransformerIndex = i
		return result
	}

	return result
}
