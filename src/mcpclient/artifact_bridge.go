package mcpclient

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxMCPArtifactOutputBytes int64 = 32 << 20
const hostArtifactCompletionTimeout = 30 * time.Second

type validatedMCPArtifactOutput struct {
	bytes        []byte
	digest       string
	relativePath string
	absolutePath string
	contentType  string
	title        string
	kind         string
	structured   any
}

type mcpArtifactOutputError struct {
	code       string
	category   tool.ErrorCategory
	status     tool.OutcomeStatus
	message    string
	recovery   []string
	diagnostic map[string]any
}

func (e *mcpArtifactOutputError) Error() string { return e.message }

func (a *McpToolAdapter) publishArtifactOutput(ctx context.Context, result *mcp.CallToolResult) tool.ExecutionOutcome {
	validated, err := a.validateArtifactOutput(ctx, result)
	if err != nil {
		return a.artifactOutputFailure(err, result)
	}

	permissionContext := tool.PermissionContextFromContext(ctx)
	broker, configured := tool.ExecutionBrokerFromContext(ctx)
	if !configured {
		broker = tool.NewWorkspaceExecutionBroker()
	}
	if broker == nil {
		return a.artifactOutputFailure(&mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputCommitFailed, category: tool.ErrorCategoryPersistence,
			message:  "host artifact publication requires an available workspace execution broker",
			recovery: []string{"retry_with_workspace_execution_broker"},
		}, result)
	}

	brokerScope, err := tool.CanonicalBrokerWorkspaceScope(permissionContext)
	if err != nil {
		return a.artifactOutputFailure(&mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputPathDenied, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:  "cannot project artifact TaskScope into the host broker: " + err.Error(),
			recovery: []string{"choose_output_path_in_task_scope"},
		}, result)
	}
	identity, _ := tool.ToolExecutionIdentityFromContext(ctx)
	capability := artifactPublishCapability(a.providerManifest.Capabilities)
	brokerRelativePath := filepath.Clean(filepath.FromSlash(validated.relativePath))
	writableRoot := filepath.Dir(brokerRelativePath)
	authorization, authorizeErr := broker.Authorize(ctx, tool.ExecutionBrokerRequest{
		RunID: identity.RunID, ToolCallID: identity.ToolCallID,
		ProviderID: a.providerManifest.ProviderID, Capability: capability,
		Operation:        tool.BrokerOperationArtifactPublish,
		WorkingDirectory: permissionContext.WorkspaceRoot, WorkspaceRoot: permissionContext.WorkspaceRoot,
		WorkspaceScopeDeclared: true,
		WritableRoots:          []string{writableRoot},
		ProtectedRoots:         brokerScope.ProtectedRoots,
		ReferencedPaths:        []string{brokerRelativePath},
		ArtifactPublish: &tool.BrokerArtifactPublishPlan{
			RelativePath: brokerRelativePath, SHA256: validated.digest, SizeBytes: int64(len(validated.bytes)),
			ContentType: validated.contentType, Kind: validated.kind,
		},
	})
	if authorizeErr != nil || authorization.Action != tool.BrokerAllow || authorization.RequiresAppliedSandbox || strings.TrimSpace(authorization.ExecutionWorkspaceRoot) == "" {
		if authorizeErr == nil {
			authorizeErr = tool.ErrExecutionBrokerDenied
		}
		if authorization.Action == tool.BrokerAllow && strings.TrimSpace(authorization.ExecutionWorkspaceRoot) != "" {
			_, _, _ = completeHostArtifactBroker(ctx, broker, authorization, false, "invalid artifact broker authorization")
		}
		return a.artifactOutputFailure(&mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputCommitFailed, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:    "host broker rejected artifact publication: " + authorizeErr.Error(),
			recovery:   []string{"choose_output_path_in_task_scope", "retry_with_workspace_execution_broker"},
			diagnostic: map[string]any{"broker_authorization": authorization},
		}, result)
	}

	stagePath := filepath.Join(authorization.ExecutionWorkspaceRoot, brokerRelativePath)
	if ctx.Err() != nil {
		receipt, _, _ := completeHostArtifactBroker(ctx, broker, authorization, false, "artifact call cancelled before stage publication")
		outcome := a.artifactOutputFailure(&mcpArtifactOutputError{
			code: tool.ErrorCodeToolCancelled, category: tool.ErrorCategoryCancelled, status: tool.OutcomeCancelled,
			message:  "MCP artifact publication was cancelled before host commit",
			recovery: []string{"retry_artifact_generation"}, diagnostic: map[string]any{"broker_receipt": receipt},
		}, result)
		applyBrokerReceiptToOutcome(&outcome, receipt)
		return outcome
	}
	if err := writeHostArtifactStage(authorization.ExecutionWorkspaceRoot, stagePath, validated.bytes); err != nil {
		receipt, _, _ := completeHostArtifactBroker(ctx, broker, authorization, false, "host artifact stage write failed")
		return a.artifactOutputFailure(&mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputCommitFailed, category: tool.ErrorCategoryPersistence,
			message:    "host could not stage validated artifact bytes: " + err.Error(),
			recovery:   []string{"verify_workspace_is_writable", "retry_artifact_generation"},
			diagnostic: map[string]any{"broker_receipt": receipt},
		}, result)
	}

	receipt, completeErr, cancelledBeforeCommit := completeHostArtifactBroker(ctx, broker, authorization, true, "")
	if cancelledBeforeCommit {
		outcome := a.artifactOutputFailure(&mcpArtifactOutputError{
			code: tool.ErrorCodeToolCancelled, category: tool.ErrorCategoryCancelled, status: tool.OutcomeCancelled,
			message:  "MCP artifact publication was cancelled before host commit",
			recovery: []string{"retry_artifact_generation"}, diagnostic: map[string]any{"broker_receipt": receipt},
		}, result)
		applyBrokerReceiptToOutcome(&outcome, receipt)
		return outcome
	}
	if completeErr != nil || (receipt.CommitStatus != "committed" && receipt.CommitStatus != "no_workspace_changes") {
		if completeErr == nil {
			completeErr = fmt.Errorf("unexpected broker commit status %q", receipt.CommitStatus)
		}
		failure := &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputCommitFailed, category: tool.ErrorCategoryPersistence,
			message:    "host could not commit validated artifact bytes: " + completeErr.Error(),
			recovery:   []string{"inspect_broker_receipt", "recover_replaced_preimage", "retry_artifact_generation"},
			diagnostic: map[string]any{"broker_receipt": receipt},
		}
		outcome := a.artifactOutputFailure(failure, result)
		applyBrokerReceiptToOutcome(&outcome, receipt)
		return outcome
	}

	verificationCtx, cancelVerification := context.WithTimeout(context.Background(), hostArtifactCompletionTimeout)
	verifyErr := verifyCommittedArtifact(verificationCtx, validated.absolutePath, validated.digest, int64(len(validated.bytes)))
	cancelVerification()
	if verifyErr != nil {
		outcome := a.artifactOutputFailure(&mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputCommitFailed, category: tool.ErrorCategoryPersistence,
			message:    "host could not verify the committed artifact: " + verifyErr.Error(),
			recovery:   []string{"inspect_broker_receipt", "recover_replaced_preimage", "do_not_publish_unverified_file"},
			diagnostic: map[string]any{"broker_receipt": receipt},
		}, result)
		applyBrokerReceiptToOutcome(&outcome, receipt)
		return outcome
	}

	return a.artifactOutputSuccess(validated, result, receipt)
}

// completeHostArtifactBroker never asks the broker to publish after the caller
// has already been cancelled. Cleanup and an already-started atomic completion
// run on a bounded host context so cancellation cannot strand a private stage
// or broker session.
func completeHostArtifactBroker(ctx context.Context, broker tool.ExecutionBroker, authorization tool.BrokerAuthorization, publish bool, discardReason string) (tool.BrokerExecutionReceipt, error, bool) {
	cancelledBeforeCommit := publish && ctx.Err() != nil
	if cancelledBeforeCommit {
		publish = false
		discardReason = "artifact call cancelled before host commit"
	}
	result := tool.BrokerExecutionResult{
		TransportStatus: tool.TransportCompleted, ProcessStatus: tool.ProcessNotApplicable,
		SideEffectState: tool.OutcomeSideEffectUnknown,
	}
	if !publish {
		result.DiscardWorkspace = true
		result.DiscardReason = discardReason
		result.SideEffectState = tool.OutcomeSideEffectNone
	}
	completionCtx, cancel := context.WithTimeout(context.Background(), hostArtifactCompletionTimeout)
	defer cancel()
	receipt, err := broker.Complete(completionCtx, authorization, result)
	return receipt, err, cancelledBeforeCommit
}

func (a *McpToolAdapter) validateArtifactOutput(ctx context.Context, result *mcp.CallToolResult) (validatedMCPArtifactOutput, error) {
	if result == nil || result.IsError {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
			"MCP provider did not return a successful artifact result")
	}
	contract := a.providerMetadata.ArtifactOutput
	if contract == nil {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryTool,
			"MCP provider has no artifact output contract")
	}
	structured, ok := jsonCompatibleValue(result.StructuredContent)
	if !ok {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
			"MCP provider returned no JSON-compatible structured artifact output")
	}
	requireString := func(fieldPath string) (string, error) {
		value, exists := valueAtPath(structured, fieldPath)
		text, isString := value.(string)
		text = strings.TrimSpace(text)
		if !exists || !isString || text == "" {
			return "", artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
				fmt.Sprintf("MCP provider artifact field %q is missing or not a non-empty string", fieldPath))
		}
		return text, nil
	}

	encoded, err := requireString(strings.TrimSpace(contract.BytesBase64Field))
	if err != nil {
		return validatedMCPArtifactOutput{}, err
	}
	maxBytes := maxMCPArtifactOutputBytes
	if contract.MaxBytes > 0 && contract.MaxBytes < maxBytes {
		maxBytes = contract.MaxBytes
	}
	if int64(len(encoded)) > int64(base64.StdEncoding.EncodedLen(int(maxBytes)))+4 {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputOversize, tool.ErrorCategoryPolicy,
			fmt.Sprintf("MCP artifact output exceeds the host limit of %d bytes", maxBytes))
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
			"MCP artifact bytes are not strict base64: "+err.Error())
	}
	if len(decoded) == 0 {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
			"MCP artifact output is empty")
	}
	if int64(len(decoded)) > maxBytes {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputOversize, tool.ErrorCategoryPolicy,
			fmt.Sprintf("MCP artifact output exceeds the host limit of %d bytes", maxBytes))
	}

	expectedDigest, err := requireString(strings.TrimSpace(contract.SHA256Field))
	if err != nil {
		return validatedMCPArtifactOutput{}, err
	}
	expectedDigestBytes, decodeErr := hex.DecodeString(expectedDigest)
	if decodeErr != nil || len(expectedDigestBytes) != sha256.Size || strings.ToLower(expectedDigest) != expectedDigest {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputInvalid, tool.ErrorCategoryDomain,
			"MCP artifact sha256 must be exactly 64 lowercase hexadecimal characters")
	}
	actualDigest := sha256.Sum256(decoded)
	if subtle.ConstantTimeCompare(expectedDigestBytes, actualDigest[:]) != 1 {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputHashMismatch, tool.ErrorCategoryDomain,
			"MCP artifact bytes do not match the declared sha256")
	}

	relativePath, err := requireString(strings.TrimSpace(contract.RelativePathField))
	if err != nil {
		return validatedMCPArtifactOutput{}, err
	}
	if err := validatePortableRelativeArtifactPath(relativePath); err != nil {
		return validatedMCPArtifactOutput{}, &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputPathDenied, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:  "MCP artifact output path is denied: " + err.Error(),
			recovery: []string{"choose_relative_path_in_task_output_roots"},
		}
	}
	if !tool.HasPermissionContext(ctx) {
		return validatedMCPArtifactOutput{}, &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputPathDenied, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:  "MCP artifact publication requires an explicit TaskScope permission context",
			recovery: []string{"execute_through_tool_runtime_with_task_scope"},
		}
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	mutationPath, mutationErr := permissionContext.ResolveMutationPathChecked(filepath.FromSlash(relativePath))
	outputPath, outputErr := permissionContext.ResolveOutputMutationPathChecked(filepath.FromSlash(relativePath))
	realWorkspaceAllowed := artifactPathWithinRealWorkspace(permissionContext.WorkspaceRoot, outputPath)
	if mutationErr != nil || outputErr != nil || filepath.Clean(mutationPath) != filepath.Clean(outputPath) || !permissionContext.IsWithinWorkspace(outputPath) || !realWorkspaceAllowed {
		reason := firstNonNilError(mutationErr, outputErr)
		if reason == nil {
			reason = fmt.Errorf("path must be inside the workspace writable and output roots")
		}
		return validatedMCPArtifactOutput{}, &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputPathDenied, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:  "MCP artifact output path is outside TaskScope: " + reason.Error(),
			recovery: []string{"choose_relative_path_in_writable_output_roots"},
		}
	}
	if err := rejectSymlinkArtifactPath(permissionContext.WorkspaceRoot, outputPath); err != nil {
		return validatedMCPArtifactOutput{}, &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputPathDenied, category: tool.ErrorCategoryPolicy, status: tool.OutcomeDenied,
			message:  "MCP artifact output path is denied: " + err.Error(),
			recovery: []string{"choose_non_symlink_output_path"},
		}
	}

	contentType, err := requireString(strings.TrimSpace(contract.ContentTypeField))
	if err != nil {
		return validatedMCPArtifactOutput{}, err
	}
	kind, validationErr := validateArtifactContent(relativePath, contentType, decoded)
	if validationErr != nil {
		return validatedMCPArtifactOutput{}, artifactOutputError(ErrorCodeArtifactOutputContentMismatch, tool.ErrorCategoryDomain,
			"MCP artifact extension, content type, kind, or bytes do not agree: "+validationErr.Error())
	}
	title := filepath.Base(outputPath)
	if titleField := strings.TrimSpace(contract.TitleField); titleField != "" {
		title, err = requireString(titleField)
		if err != nil {
			return validatedMCPArtifactOutput{}, err
		}
		title = sanitizeArtifactTitle(title, filepath.Base(outputPath))
	}

	sanitized := map[string]any{}
	setValueAtPathCreating(sanitized, strings.TrimSpace(contract.BytesBase64Field), fmt.Sprintf("[host-consumed %d base64 characters]", len(encoded)))
	setValueAtPathCreating(sanitized, strings.TrimSpace(contract.SHA256Field), hex.EncodeToString(actualDigest[:]))
	setValueAtPathCreating(sanitized, strings.TrimSpace(contract.RelativePathField), relativePath)
	setValueAtPathCreating(sanitized, strings.TrimSpace(contract.ContentTypeField), contentType)
	if titleField := strings.TrimSpace(contract.TitleField); titleField != "" {
		setValueAtPathCreating(sanitized, titleField, title)
	}
	return validatedMCPArtifactOutput{
		bytes: decoded, digest: hex.EncodeToString(actualDigest[:]), relativePath: relativePath,
		absolutePath: outputPath, contentType: contentType, title: title, kind: kind, structured: sanitized,
	}, nil
}

func sanitizeArtifactTitle(value string, fallback string) string {
	value = strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return ' '
		}
		return character
	}, value)
	value = strings.TrimSpace(excerpt(value, 256))
	if value == "" {
		return fallback
	}
	return value
}

func (a *McpToolAdapter) artifactOutputSuccess(validated validatedMCPArtifactOutput, result *mcp.CallToolResult, receipt tool.BrokerExecutionReceipt) tool.ExecutionOutcome {
	action := tool.ArtifactActionPublished
	operationState := tool.OutcomeSideEffectNone
	for _, change := range receipt.WorkspaceChanges {
		changeRelative, relativeErr := filepath.Rel(receipt.WorkspaceRoot, change.Path)
		if relativeErr != nil || filepath.Clean(changeRelative) != filepath.Clean(filepath.FromSlash(validated.relativePath)) {
			continue
		}
		switch change.Action {
		case "created":
			action = tool.ArtifactActionCreated
		case "updated":
			action = tool.ArtifactActionUpdated
		}
	}
	if receipt.CommitStatus == "committed" {
		operationState = tool.OutcomeSideEffectApplied
	}
	displayPath := filepath.ToSlash(validated.relativePath)
	summary := "Published MCP artifact " + displayPath
	outcome := tool.SucceededOutcome(summary)
	outcome.Summary = summary
	outcome.TransportStatus = tool.TransportCompleted
	outcome.ProcessStatus = tool.ProcessNotApplicable
	outcome.Operation = &tool.OperationDescriptor{Type: "mcp_artifact_publish", Target: displayPath, SideEffectState: operationState}
	outcome.Artifacts = []tool.ArtifactDescriptor{{
		ContractVersion: tool.ArtifactContractVersion, Role: tool.ArtifactRoleDeliverable,
		Producer: a.prefixedName, Action: action, ID: a.prefixedName + ":" + displayPath,
		Title: validated.title, Description: "Host-validated MCP artifact " + displayPath,
		Kind: validated.kind, Path: displayPath, Status: "ready",
	}}
	outcome.Diagnostic = a.artifactDiagnostic(result)
	outcome.Diagnostic["provider_id"] = a.providerManifest.ProviderID
	outcome.Diagnostic["artifact_sha256"] = validated.digest
	outcome.Diagnostic["artifact_size_bytes"] = len(validated.bytes)
	outcome.Diagnostic["artifact_content_type"] = validated.contentType
	outcome.Diagnostic["broker_receipt"] = receipt
	if details, ok := a.structuredDetails(validated.structured); ok {
		outcome.Details = details
	}
	applyBrokerReceiptToOutcome(&outcome, receipt)
	return outcome
}

func (a *McpToolAdapter) artifactOutputFailure(err error, result *mcp.CallToolResult) tool.ExecutionOutcome {
	var artifactErr *mcpArtifactOutputError
	if !errors.As(err, &artifactErr) {
		artifactErr = &mcpArtifactOutputError{
			code: ErrorCodeArtifactOutputInvalid, category: tool.ErrorCategoryTool,
			message: err.Error(), recovery: []string{"inspect_mcp_artifact_output"},
		}
	}
	outcome := tool.FailedOutcome(artifactErr.message, artifactErr.code, artifactErr.category, false)
	if artifactErr.status != "" {
		outcome.Status = artifactErr.status
	}
	outcome.TransportStatus = tool.TransportCompleted
	outcome.ProcessStatus = tool.ProcessNotApplicable
	outcome.RecoveryOptions = append([]string(nil), artifactErr.recovery...)
	if len(outcome.RecoveryOptions) == 0 {
		outcome.RecoveryOptions = []string{"inspect_mcp_artifact_output", "do_not_publish_unverified_file"}
	}
	outcome.Diagnostic = a.artifactDiagnostic(result)
	outcome.Diagnostic["provider_id"] = a.providerManifest.ProviderID
	for key, value := range artifactErr.diagnostic {
		outcome.Diagnostic[key] = value
	}
	outcome.Operation = &tool.OperationDescriptor{
		Type: "mcp_artifact_publish", SideEffectState: tool.OutcomeSideEffectNotStarted,
	}
	return outcome
}

func (a *McpToolAdapter) artifactDiagnostic(result *mcp.CallToolResult) map[string]any {
	diagnostic := a.mcpDiagnostic(nil)
	if result != nil {
		diagnostic["mcp_is_error"] = result.IsError
		diagnostic["content_items"] = len(result.Content)
	}
	diagnostic["provider_bytes_exposed"] = false
	return diagnostic
}

func artifactOutputError(code string, category tool.ErrorCategory, message string) error {
	return &mcpArtifactOutputError{code: code, category: category, message: message}
}

func artifactPublishCapability(capabilities []tool.Capability) tool.Capability {
	for _, candidate := range []tool.Capability{
		tool.CapabilityPresentationCreate, tool.CapabilityPresentationEdit, tool.CapabilityPresentationRender,
		tool.CapabilityDocumentCreate, tool.CapabilitySpreadsheetCreate, tool.CapabilityPDFRender,
		tool.CapabilityFileWrite, tool.CapabilityFileEdit,
	} {
		if tool.HasCapability(capabilities, candidate) {
			return candidate
		}
	}
	return tool.CapabilityFileWrite
}

func validatePortableRelativeArtifactPath(value string) error {
	if value == "" || strings.ContainsRune(value, 0) || strings.Contains(value, "\\") || strings.ContainsAny(value, `:*?"<>|`) {
		return fmt.Errorf("path must be a non-empty portable forward-slash path")
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return fmt.Errorf("path cannot contain control characters")
		}
	}
	if strings.HasPrefix(value, "/") {
		return fmt.Errorf("absolute and volume-qualified paths are not accepted")
	}
	cleaned := pathpkg.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path must be canonical and cannot traverse outside TaskScope")
	}
	if !tool.IsPublishableArtifactPath(cleaned) {
		return fmt.Errorf("file extension is not a publishable artifact type")
	}
	return nil
}

func rejectSymlinkArtifactPath(workspaceRoot string, target string) error {
	root := filepath.Clean(workspaceRoot)
	relative, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("path escapes the workspace")
	}
	cursor := root
	segments := strings.Split(relative, string(filepath.Separator))
	for index, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		cursor = filepath.Join(cursor, segment)
		info, statErr := os.Lstat(cursor)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic-link component %q is not allowed", cursor)
		}
		if index < len(segments)-1 && !info.IsDir() {
			return fmt.Errorf("parent component %q is not a directory", cursor)
		}
		if index == len(segments)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("existing artifact target is not a regular file")
		}
	}
	return nil
}

func artifactPathWithinRealWorkspace(workspaceRoot string, target string) bool {
	realRoot, err := filepath.EvalSymlinks(filepath.Clean(workspaceRoot))
	if err != nil {
		return false
	}
	realTarget, err := resolveArtifactPathWithExistingAncestor(target)
	if err != nil {
		return false
	}
	return tool.PathWithinRoot(realTarget, realRoot)
}

func resolveArtifactPathWithExistingAncestor(value string) (string, error) {
	cursor := filepath.Clean(value)
	missing := make([]string, 0, 2)
	for {
		if _, err := os.Lstat(cursor); err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(cursor)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", fmt.Errorf("no existing ancestor for %q", value)
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}
}

func validateArtifactContent(relativePath string, contentType string, data []byte) (string, error) {
	contentType = strings.TrimSpace(contentType)
	if contentType != strings.ToLower(contentType) {
		return "", fmt.Errorf("content type must use its exact lowercase canonical value")
	}
	extension := strings.ToLower(filepath.Ext(relativePath))
	expectedTypes := map[string]string{
		".md": "text/markdown", ".pdf": "application/pdf", ".doc": "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".csv":  "text/csv", ".tsv": "text/tab-separated-values", ".ppt": "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}
	if expectedTypes[extension] == "" || contentType != expectedTypes[extension] {
		return "", fmt.Errorf("content type %q does not match extension %q", contentType, extension)
	}
	kind := tool.ArtifactKindForPath(relativePath)
	if kind == "file" {
		return "", fmt.Errorf("artifact kind is unsupported")
	}
	switch extension {
	case ".md", ".csv", ".tsv":
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", fmt.Errorf("text artifact must be UTF-8 without NUL bytes")
		}
	case ".pdf":
		if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.Contains(data, []byte("%%EOF")) {
			return "", fmt.Errorf("PDF signature or EOF marker is missing")
		}
	case ".docx", ".xlsx", ".pptx":
		if err := validateOOXMLContainer(extension, data); err != nil {
			return "", err
		}
	case ".doc", ".xls", ".ppt":
		oleMagic := []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}
		if !bytes.HasPrefix(data, oleMagic) {
			return "", fmt.Errorf("legacy Office compound-file signature is missing")
		}
	}
	return kind, nil
}

func validateOOXMLContainer(extension string, data []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("OOXML bytes are not a valid ZIP container: %w", err)
	}
	wantedPrefix := map[string]string{".docx": "word/", ".xlsx": "xl/", ".pptx": "ppt/"}[extension]
	hasContentTypes := false
	hasKindDirectory := false
	if len(reader.File) > 10000 {
		return fmt.Errorf("OOXML container has too many entries")
	}
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if name == "[Content_Types].xml" {
			hasContentTypes = true
		}
		if strings.HasPrefix(name, wantedPrefix) {
			hasKindDirectory = true
		}
	}
	if !hasContentTypes || !hasKindDirectory {
		return fmt.Errorf("OOXML container does not match %s", extension)
	}
	return nil
}

func writeHostArtifactStage(stageRoot string, target string, data []byte) error {
	stageRoot = filepath.Clean(stageRoot)
	target = filepath.Clean(target)
	if !tool.PathWithinRoot(target, stageRoot) {
		return fmt.Errorf("stage target escapes broker workspace")
	}
	parent := filepath.Dir(target)
	relativeParent, err := filepath.Rel(stageRoot, parent)
	if err != nil {
		return err
	}
	cursor := stageRoot
	for _, segment := range strings.Split(relativeParent, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		cursor = filepath.Join(cursor, segment)
		info, statErr := os.Lstat(cursor)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(cursor, 0o700); err != nil {
				return err
			}
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("stage parent is not a real directory: %s", cursor)
		}
	}
	targetMode := os.FileMode(0o644)
	targetExists := false
	if info, statErr := os.Lstat(target); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("stage target is not a regular file")
		}
		targetMode = info.Mode().Perm()
		targetExists = true
		if info.Size() == int64(len(data)) {
			existingDigest, hashErr := hashArtifactFile(context.Background(), target)
			newDigest := sha256.Sum256(data)
			if hashErr == nil && existingDigest == hex.EncodeToString(newDigest[:]) {
				return nil
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	temporary, err := os.CreateTemp(parent, ".awaken-artifact-stage-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(targetMode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if targetExists {
		if err := os.Remove(target); err != nil {
			return err
		}
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func verifyCommittedArtifact(ctx context.Context, path string, expectedDigest string, expectedSize int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != expectedSize {
		return fmt.Errorf("committed artifact is not the expected regular file and size")
	}
	digest, err := hashArtifactFile(ctx, path)
	if err != nil {
		return err
	}
	if digest != expectedDigest {
		return fmt.Errorf("committed artifact digest mismatch")
	}
	return nil
}

func hashArtifactFile(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			_, _ = hash.Write(buffer[:read])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func setValueAtPathCreating(root map[string]any, fieldPath string, value any) bool {
	segments := strings.Split(strings.TrimSpace(fieldPath), ".")
	if len(segments) == 0 || root == nil {
		return false
	}
	current := root
	for _, segment := range segments[:len(segments)-1] {
		next, exists := current[segment]
		if !exists {
			created := map[string]any{}
			current[segment] = created
			current = created
			continue
		}
		nextObject, ok := next.(map[string]any)
		if !ok {
			return false
		}
		current = nextObject
	}
	current[segments[len(segments)-1]] = value
	return true
}

func firstNonNilError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func applyBrokerReceiptToOutcome(outcome *tool.ExecutionOutcome, receipt tool.BrokerExecutionReceipt) {
	if outcome == nil {
		return
	}
	if outcome.Operation == nil {
		outcome.Operation = &tool.OperationDescriptor{Type: "mcp_artifact_publish"}
	}
	switch receipt.CommitStatus {
	case "committed":
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectApplied
	case "partial":
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectPartial
	case "no_workspace_changes", "discarded_by_typed_executor", "discarded_not_completed":
		outcome.Operation.SideEffectState = tool.OutcomeSideEffectNone
	default:
		if receipt.SideEffectState != "" {
			outcome.Operation.SideEffectState = receipt.SideEffectState
		} else if len(receipt.WorkspaceChanges) > 0 {
			outcome.Operation.SideEffectState = tool.OutcomeSideEffectPartial
		}
	}
	for _, change := range receipt.WorkspaceChanges {
		outcome.AffectedPaths = append(outcome.AffectedPaths, change.Path)
		details := map[string]any{"broker_id": receipt.BrokerID, "commit_status": receipt.CommitStatus}
		reversibility := tool.ChangeConditional
		if strings.TrimSpace(change.RecoveryPath) != "" {
			details["recovery_path"] = change.RecoveryPath
			details["recovery_method"] = change.RecoveryMethod
			reversibility = tool.ChangeReversible
		}
		outcome.Changes = append(outcome.Changes, tool.ResourceChange{
			Kind: "file", Action: change.Action, Resource: change.Path,
			Summary:   "Host broker " + change.Action + " MCP artifact output",
			Certainty: tool.ChangeObserved, Reversibility: reversibility, Details: details,
		})
	}
}
