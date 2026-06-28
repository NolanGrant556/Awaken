package terminaltransform

import (
	"errors"
	"reflect"
	"testing"
)

func TestApplyFirstValidStringWins(t *testing.T) {
	ctx := Context{
		Command:    "go test ./...",
		Output:     "original output",
		ReturnCode: 0,
		TaskID:     "task-123",
		EnvType:    "local",
	}
	calls := []int{}
	seenOutputs := []string{}

	result := Apply(ctx,
		func(ctx Context) (string, bool, error) {
			calls = append(calls, 0)
			seenOutputs = append(seenOutputs, ctx.Output)
			return "ignored", false, nil
		},
		func(ctx Context) (string, bool, error) {
			calls = append(calls, 1)
			seenOutputs = append(seenOutputs, ctx.Output)
			return "rewritten output", true, nil
		},
		func(ctx Context) (string, bool, error) {
			calls = append(calls, 2)
			return "should not run", true, nil
		},
	)

	if result.Output != "rewritten output" {
		t.Fatalf("Output = %q, want %q", result.Output, "rewritten output")
	}
	if !result.Transformed {
		t.Fatal("Transformed = false, want true")
	}
	if result.TransformerIndex != 1 {
		t.Fatalf("TransformerIndex = %d, want 1", result.TransformerIndex)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", result.Errors)
	}
	if !reflect.DeepEqual(calls, []int{0, 1}) {
		t.Fatalf("calls = %v, want [0 1]", calls)
	}
	if !reflect.DeepEqual(seenOutputs, []string{"original output", "original output"}) {
		t.Fatalf("seenOutputs = %v, want original output for each transformer", seenOutputs)
	}
}

func TestApplyErrorsFailOpen(t *testing.T) {
	result := Apply(Context{Output: "original"},
		func(Context) (string, bool, error) {
			return "bad replacement", true, errors.New("transform failed")
		},
		func(Context) (string, bool, error) {
			return "fallback replacement", true, nil
		},
	)

	if result.Output != "fallback replacement" {
		t.Fatalf("Output = %q, want %q", result.Output, "fallback replacement")
	}
	if !result.Transformed {
		t.Fatal("Transformed = false, want true")
	}
	if result.TransformerIndex != 1 {
		t.Fatalf("TransformerIndex = %d, want 1", result.TransformerIndex)
	}
	if !reflect.DeepEqual(result.Errors, []string{"transform failed"}) {
		t.Fatalf("Errors = %v, want [transform failed]", result.Errors)
	}
}

func TestApplyAllInvalidReturnsOriginalOutput(t *testing.T) {
	result := Apply(Context{Output: "original"},
		func(Context) (string, bool, error) {
			return "ignored", false, nil
		},
		func(Context) (string, bool, error) {
			return "", false, nil
		},
	)

	if result.Output != "original" {
		t.Fatalf("Output = %q, want original", result.Output)
	}
	if result.Transformed {
		t.Fatal("Transformed = true, want false")
	}
	if result.TransformerIndex != -1 {
		t.Fatalf("TransformerIndex = %d, want -1", result.TransformerIndex)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", result.Errors)
	}
}

func TestApplyEmptyStringIsValidReplacement(t *testing.T) {
	result := Apply(Context{Output: "noisy output"},
		func(Context) (string, bool, error) {
			return "", true, nil
		},
	)

	if result.Output != "" {
		t.Fatalf("Output = %q, want empty string", result.Output)
	}
	if !result.Transformed {
		t.Fatal("Transformed = false, want true")
	}
	if result.TransformerIndex != 0 {
		t.Fatalf("TransformerIndex = %d, want 0", result.TransformerIndex)
	}
}
