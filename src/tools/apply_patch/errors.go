package applypatch

import (
	"errors"
	"fmt"
	"strings"
)

// Typed apply_patch error codes for FailureSupervisor recovery mapping.
const (
	ErrorCodeInvalidPatchFormat      = "invalid_patch_format"
	ErrorCodePatchMissingBegin       = "patch_missing_begin"
	ErrorCodePatchMissingEnd         = "patch_missing_end"
	ErrorCodePatchInvalidHunkHeader  = "patch_invalid_hunk_header"
	ErrorCodePatchInvalidLinePrefix  = "patch_invalid_line_prefix"
	ErrorCodePatchContextNotFound    = "patch_context_not_found"
	ErrorCodePatchContextAmbiguous   = "patch_context_ambiguous"
	ErrorCodePatchContextMismatch    = "patch_context_mismatch"
	ErrorCodePatchStaleFile          = "patch_stale_file"
)

// PatchError is a typed apply_patch failure with a stable machine code.
type PatchError struct {
	Code    string
	Message string
}

func (e *PatchError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return e.Code
	}
	return e.Message
}

func patchErrorf(code, format string, args ...any) error {
	return &PatchError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func patchErrorCode(err error) string {
	var pe *PatchError
	if errors.As(err, &pe) && pe != nil && pe.Code != "" {
		return pe.Code
	}
	return ErrorCodeInvalidPatchFormat
}

// classifyLegacyPatchError maps historical free-form parser/matcher errors to
// stable codes when callers still produce plain fmt.Errorf values.
func classifyLegacyPatchError(err error) string {
	if err == nil {
		return ""
	}
	var pe *PatchError
	if errors.As(err, &pe) && pe != nil && pe.Code != "" {
		return pe.Code
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, ErrorCodePatchMissingBegin),
		(strings.Contains(msg, "first line must be") && strings.Contains(msg, beginPatchMarker)),
		(strings.Contains(msg, "Begin Patch") && (strings.Contains(msg, "first line") || strings.Contains(msg, "must contain Begin Patch"))):
		return ErrorCodePatchMissingBegin
	case strings.Contains(msg, ErrorCodePatchMissingEnd),
		(strings.Contains(msg, "last line must be") && strings.Contains(msg, endPatchMarker)):
		return ErrorCodePatchMissingEnd
	case strings.Contains(msg, ErrorCodePatchInvalidHunkHeader),
		strings.Contains(msg, "is not a valid hunk header"):
		return ErrorCodePatchInvalidHunkHeader
	case strings.Contains(msg, ErrorCodePatchInvalidLinePrefix),
		strings.Contains(msg, "must start with +"),
		strings.Contains(msg, "update lines must start with"),
		strings.Contains(msg, "content lines must start with"):
		return ErrorCodePatchInvalidLinePrefix
	case strings.Contains(msg, ErrorCodePatchContextNotFound),
		strings.Contains(msg, "failed to find expected lines"),
		strings.Contains(msg, "failed to find context"):
		return ErrorCodePatchContextNotFound
	case strings.Contains(msg, ErrorCodePatchContextAmbiguous),
		strings.Contains(msg, "ambiguous"):
		return ErrorCodePatchContextAmbiguous
	case strings.Contains(msg, ErrorCodePatchStaleFile),
		strings.Contains(msg, "stale"):
		return ErrorCodePatchStaleFile
	case strings.Contains(msg, ErrorCodePatchContextMismatch):
		return ErrorCodePatchContextMismatch
	case strings.Contains(msg, "End of File requires"):
		return ErrorCodeInvalidPatchFormat
	default:
		return ErrorCodeInvalidPatchFormat
	}
}

// IsInvalidPatchFormat reports whether err is a patch grammar/format failure
// (as opposed to a context mismatch against the current file).
func IsInvalidPatchFormat(err error) bool {
	code := classifyLegacyPatchError(err)
	switch code {
	case ErrorCodeInvalidPatchFormat,
		ErrorCodePatchMissingBegin,
		ErrorCodePatchMissingEnd,
		ErrorCodePatchInvalidHunkHeader,
		ErrorCodePatchInvalidLinePrefix:
		return true
	default:
		return false
	}
}

// IsPatchContextMismatch reports context/file-state failures that should
// recover via read_file + text_edit rather than replaying the same patch.
func IsPatchContextMismatch(err error) bool {
	code := classifyLegacyPatchError(err)
	switch code {
	case ErrorCodePatchContextNotFound,
		ErrorCodePatchContextAmbiguous,
		ErrorCodePatchContextMismatch,
		ErrorCodePatchStaleFile:
		return true
	default:
		return false
	}
}
