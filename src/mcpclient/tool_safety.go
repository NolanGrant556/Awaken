package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"
)

const (
	// ErrorCodeUnsupportedRecoverableDelete is returned before transport starts
	// when a remote delete provider does not declare verifiable recovery facts.
	ErrorCodeUnsupportedRecoverableDelete = "unsupported_recoverable_delete"
	// ErrorCodeRecoverableDeleteResultUnverified reports a provider contract
	// violation after a remote call claimed success without recovery evidence.
	ErrorCodeRecoverableDeleteResultUnverified = "recoverable_delete_result_unverified"
	// ErrorCodeUnverifiedProviderMetadata is returned before transport starts
	// when a non-read-only MCP tool lacks valid Awaken per-tool metadata. MCP
	// annotations are hints and cannot prove that an ordinary-looking alias has
	// no message, delete, or other hidden side effect.
	ErrorCodeUnverifiedProviderMetadata    = "unverified_mcp_provider_metadata"
	ErrorCodeUnobservableMessageRecipients = "unobservable_message_recipients"
	ErrorCodeMassMessageReviewRequired     = "mass_message_review_required"
	ErrorCodeUnsupportedArtifactOutput     = "unsupported_artifact_output_contract"
	ErrorCodeArtifactOutputInvalid         = "artifact_output_invalid"
	ErrorCodeArtifactOutputOversize        = "artifact_output_oversize"
	ErrorCodeArtifactOutputHashMismatch    = "artifact_output_hash_mismatch"
	ErrorCodeArtifactOutputPathDenied      = "artifact_output_path_denied"
	ErrorCodeArtifactOutputContentMismatch = "artifact_output_content_mismatch"
	ErrorCodeArtifactOutputCommitFailed    = "artifact_output_commit_failed"
)

type adapterSafetyDecision struct {
	permission   tool.PermissionDecision
	envelope     *tool.MessageEgressEnvelope
	egress       tool.MessageEgressDecision
	deleteCall   bool
	artifactCall bool
}

func (a *McpToolAdapter) evaluateSafety(ctx context.Context, arguments map[string]any) adapterSafetyDecision {
	if a.requiresExplicitProviderMetadata() && !a.providerMetadataValid {
		return adapterSafetyDecision{permission: tool.UnsupportedPermissionWithCode(
			"non-read-only MCP tool does not declare valid per-tool "+AwakenProviderMetaKey+" metadata",
			ErrorCodeUnverifiedProviderMetadata,
		)}
	}

	capabilities := a.providerManifest.Capabilities
	artifactCall := a.providerMetadata.ArtifactOutput != nil
	if tool.HasCapability(capabilities, tool.CapabilityFileDelete) {
		if reason := a.validateRecoverableDeleteContract(); reason != "" {
			return adapterSafetyDecision{
				permission: tool.UnsupportedPermissionWithCode(reason, ErrorCodeUnsupportedRecoverableDelete),
				deleteCall: true,
			}
		}
	}
	if artifactCall {
		if reason := a.validateArtifactOutputContract(); reason != "" {
			return adapterSafetyDecision{
				permission:   tool.UnsupportedPermissionWithCode(reason, ErrorCodeUnsupportedArtifactOutput),
				artifactCall: true,
			}
		}
	}

	message := tool.HasCapability(capabilities, tool.CapabilityMessageSend) ||
		tool.HasCapability(capabilities, tool.CapabilityMessageMassSend)
	if !message {
		return adapterSafetyDecision{
			permission:   tool.AllowPermission(),
			deleteCall:   tool.HasCapability(capabilities, tool.CapabilityFileDelete),
			artifactCall: artifactCall,
		}
	}
	if a.providerMetadata.MessageEgress == nil {
		return adapterSafetyDecision{permission: tool.DenyPermission("message provider does not declare observable recipient semantics"), artifactCall: artifactCall}
	}
	envelope, err := a.messageEnvelope(arguments, *a.providerMetadata.MessageEgress)
	if err != nil {
		return adapterSafetyDecision{permission: tool.DenyPermission("message recipient semantics are invalid: " + err.Error()), artifactCall: artifactCall}
	}
	if tool.HasCapability(capabilities, tool.CapabilityMessageMassSend) {
		envelope.ExpandsRecipients = true
	}
	gateway := a.messageEgressGateway
	if gateway == nil {
		gateway = tool.DefaultMessageEgressGateway{}
	}
	decision, err := gateway.Decide(ctx, envelope)
	if err != nil || decision.Action == tool.EgressDeny {
		reason := strings.TrimSpace(decision.Reason)
		if reason == "" && err != nil {
			reason = err.Error()
		}
		if reason == "" {
			reason = "message recipient semantics are not observable"
		}
		return adapterSafetyDecision{permission: tool.DenyPermission(reason), envelope: &envelope, egress: decision, artifactCall: artifactCall}
	}
	if decision.Action == tool.EgressNeedsMassReview {
		metadata := tool.MassMessageMetadataFromEnvelope(envelope, a.prefixedName)
		return adapterSafetyDecision{
			permission:   tool.NeedsMassMessageReviewPermission(decision.Reason, tool.DecisionMetadata{MassMessage: &metadata}),
			envelope:     &envelope,
			egress:       decision,
			artifactCall: artifactCall,
		}
	}
	return adapterSafetyDecision{permission: tool.AllowPermission(), envelope: &envelope, egress: decision, artifactCall: artifactCall}
}

func (a *McpToolAdapter) validateArtifactOutputContract() string {
	contract := a.providerMetadata.ArtifactOutput
	if contract == nil {
		return ""
	}
	if !declaresArtifactOutputCapability(a.providerManifest.Capabilities) {
		return "MCP artifact output requires an explicit artifact-writing capability"
	}
	if contract.MaxBytes < 0 {
		return "MCP artifact output max_bytes cannot be negative"
	}
	if contract.MaxBytes > maxMCPArtifactOutputBytes {
		return fmt.Sprintf("MCP artifact output max_bytes cannot exceed the host limit of %d bytes", maxMCPArtifactOutputBytes)
	}
	fields := []struct {
		name string
		path string
	}{
		{name: "bytes_base64_field", path: contract.BytesBase64Field},
		{name: "sha256_field", path: contract.SHA256Field},
		{name: "relative_path_field", path: contract.RelativePathField},
		{name: "content_type_field", path: contract.ContentTypeField},
	}
	if strings.TrimSpace(contract.TitleField) != "" {
		fields = append(fields, struct {
			name string
			path string
		}{name: "title_field", path: contract.TitleField})
	}
	seenPaths := make(map[string]string, len(fields))
	for _, field := range fields {
		field.path = strings.TrimSpace(field.path)
		if field.path == "" {
			return "MCP artifact output " + field.name + " is required"
		}
		if !validArtifactFieldPath(field.path) {
			return fmt.Sprintf("MCP artifact output %s %q is not a valid dotted field path", field.name, field.path)
		}
		if previous, exists := seenPaths[field.path]; exists {
			return fmt.Sprintf("MCP artifact output %s and %s cannot use the same field %q", previous, field.name, field.path)
		}
		seenPaths[field.path] = field.name
		if !schemaDeclaresRequiredStringPath(a.outputSchemaJSON, field.path) {
			return fmt.Sprintf("MCP artifact output %s %q must be a required string in the provider output schema", field.name, field.path)
		}
	}
	return ""
}

func validArtifactFieldPath(value string) bool {
	for _, segment := range strings.Split(value, ".") {
		if strings.TrimSpace(segment) == "" || strings.TrimSpace(segment) != segment {
			return false
		}
	}
	return true
}

func declaresArtifactOutputCapability(capabilities []tool.Capability) bool {
	for _, capability := range capabilities {
		switch capability {
		case tool.CapabilityFileWrite,
			tool.CapabilityFileEdit,
			tool.CapabilityPDFRender,
			tool.CapabilityDocumentCreate,
			tool.CapabilityPresentationCreate,
			tool.CapabilityPresentationEdit,
			tool.CapabilityPresentationRender,
			tool.CapabilitySpreadsheetCreate:
			return true
		}
	}
	return false
}

func (a *McpToolAdapter) requiresExplicitProviderMetadata() bool {
	// A genuinely read-only tool remains usable with standard MCP annotations,
	// including open-world reads such as search. Contradictory destructive hints
	// and every non-read-only tool require the versioned Awaken contract.
	return !a.readOnly || a.destructive
}

func (a *McpToolAdapter) validateRecoverableDeleteContract() string {
	contract := a.providerMetadata.RecoverableDelete
	if contract == nil || !contract.Recoverable {
		return "remote delete provider does not declare recoverable delete semantics"
	}
	confirmation := strings.TrimSpace(contract.ConfirmationField)
	restoreHandle := strings.TrimSpace(contract.RestoreHandleField)
	if confirmation == "" && restoreHandle == "" {
		return "remote delete provider does not declare a recoverable result field"
	}
	if confirmation != "" && !schemaDeclaresPath(a.outputSchemaJSON, confirmation) {
		return "remote delete confirmation field is absent from the provider output schema"
	}
	if restoreHandle != "" && !schemaDeclaresPath(a.outputSchemaJSON, restoreHandle) {
		return "remote delete restore handle is absent from the provider output schema"
	}
	return ""
}

func (a *McpToolAdapter) verifyRecoverableDeleteResult(result any) bool {
	contract := a.providerMetadata.RecoverableDelete
	if contract == nil {
		return false
	}
	value, ok := jsonCompatibleValue(result)
	if !ok {
		return false
	}
	if path := strings.TrimSpace(contract.ConfirmationField); path != "" {
		if confirmed, exists := valueAtPath(value, path); exists && recoverableConfirmation(confirmed) {
			return true
		}
	}
	if path := strings.TrimSpace(contract.RestoreHandleField); path != "" {
		if handle, exists := valueAtPath(value, path); exists && strings.TrimSpace(stringValue(handle)) != "" {
			return true
		}
	}
	return false
}

func recoverableConfirmation(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "recoverable", "trashed", "recycle_bin", "recycle-bin", "soft_deleted", "soft-deleted":
			return true
		}
	}
	return false
}

func unsupportedRecoverableDeleteOutcome(reason string) tool.ExecutionOutcome {
	now := time.Now().UTC()
	return tool.ExecutionOutcome{
		Content:         reason,
		Status:          tool.OutcomeUnsupported,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       ErrorCodeUnsupportedRecoverableDelete,
		ErrorCategory:   tool.ErrorCategoryUnsupported,
		ErrorMessage:    reason,
		Retryable:       false,
		RecoveryOptions: []string{"choose_provider_with_recoverable_delete", "move_resource_to_provider_trash"},
		StartedAt:       now,
		FinishedAt:      now,
	}
}

func unsupportedMCPCallOutcome(decision tool.PermissionDecision) tool.ExecutionOutcome {
	if decision.ErrorCode == ErrorCodeUnsupportedRecoverableDelete {
		return unsupportedRecoverableDeleteOutcome(decision.Reason)
	}
	now := time.Now().UTC()
	code := strings.TrimSpace(decision.ErrorCode)
	if code == "" {
		code = tool.ErrorCodeUnsupported
	}
	return tool.ExecutionOutcome{
		Content:         decision.Reason,
		Status:          tool.OutcomeUnsupported,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       code,
		ErrorCategory:   tool.ErrorCategoryUnsupported,
		ErrorMessage:    decision.Reason,
		Retryable:       false,
		RecoveryOptions: []string{"use_mcp_provider_with_valid_per_tool_metadata", "choose_read_only_provider"},
		StartedAt:       now,
		FinishedAt:      now,
	}
}

func unobservableMessageOutcome(reason string) tool.ExecutionOutcome {
	outcome := tool.DeniedOutcome(reason, ErrorCodeUnobservableMessageRecipients)
	outcome.RecoveryOptions = []string{"choose_message_provider_with_typed_recipients", "supply_explicit_recipient_identity_and_kind"}
	return outcome
}

func massMessageReviewRequiredOutcome(reason string, safety adapterSafetyDecision, grantErr error) tool.ExecutionOutcome {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "mass message send requires an exact approved review grant"
	}
	now := time.Now().UTC()
	outcome := tool.ExecutionOutcome{
		Content:         reason,
		Status:          tool.OutcomeReviewRequired,
		TransportStatus: tool.TransportNotStarted,
		SemanticStatus:  tool.SemanticFailure,
		ProcessStatus:   tool.ProcessNotApplicable,
		ErrorCode:       ErrorCodeMassMessageReviewRequired,
		ErrorCategory:   tool.ErrorCategoryPolicy,
		ErrorMessage:    reason,
		Retryable:       false,
		RecoveryOptions: []string{"complete_exact_mass_message_review", "retry_as_a_new_tool_call"},
		StartedAt:       now,
		FinishedAt:      now,
	}
	outcome.Diagnostic = map[string]any{}
	if safety.envelope != nil {
		outcome.Diagnostic["envelope_digest"] = tool.MessageEnvelopeDigest(*safety.envelope)
		outcome.Diagnostic["body_digest"] = strings.TrimSpace(safety.envelope.BodyDigest)
		outcome.Diagnostic["recipient_identity_digest"] = tool.MessageRecipientIdentityDigest(safety.envelope.Recipients)
	}
	if grantErr != nil {
		outcome.Diagnostic["approval_error"] = grantErr.Error()
	}
	return outcome
}

func (a *McpToolAdapter) massMessageApprovalBinding(ctx context.Context, safety adapterSafetyDecision) (tool.MassMessageApprovalBinding, error) {
	identity, ok := tool.ToolExecutionIdentityFromContext(ctx)
	if !ok || safety.envelope == nil || identity.ToolName != a.prefixedName {
		return tool.MassMessageApprovalBinding{}, tool.ErrMassMessageApprovalMissing
	}
	return tool.MassMessageApprovalBinding{
		RunID:                   identity.RunID,
		ToolCallID:              identity.ToolCallID,
		ToolName:                a.prefixedName,
		ProviderID:              safety.envelope.ProviderID,
		EnvelopeDigest:          tool.MessageEnvelopeDigest(*safety.envelope),
		BodyDigest:              strings.TrimSpace(safety.envelope.BodyDigest),
		RecipientIdentityDigest: tool.MessageRecipientIdentityDigest(safety.envelope.Recipients),
	}, nil
}

func recoverableDeleteResultUnverifiedOutcome() tool.ExecutionOutcome {
	reason := "remote delete provider returned success without declared recovery evidence"
	outcome := tool.FailedOutcome(reason, ErrorCodeRecoverableDeleteResultUnverified, tool.ErrorCategoryDomain, false)
	outcome.TransportStatus = tool.TransportCompleted
	outcome.RecoveryOptions = []string{"inspect_provider_trash", "do_not_retry_delete", "choose_provider_with_verifiable_recovery_result"}
	return outcome
}

func (a *McpToolAdapter) messageEnvelope(arguments map[string]any, contract MCPMessageEgressContract) (tool.MessageEgressEnvelope, error) {
	envelope := tool.MessageEgressEnvelope{
		ProviderID:     a.providerManifest.ProviderID,
		Channel:        firstNonEmpty(strings.TrimSpace(contract.Channel), stringAtPath(arguments, contract.ChannelField)),
		SendingAccount: firstNonEmpty(strings.TrimSpace(contract.SendingAccount), stringAtPath(arguments, contract.SendingAccountField)),
		RecipientScope: stringAtPath(arguments, contract.RecipientScopeField),
		MessageCount:   intAtPath(arguments, contract.MessageCountField),
		Attachments:    stringsAtPath(arguments, contract.AttachmentsField),
	}
	if envelope.MessageCount <= 0 {
		envelope.MessageCount = 1
	}
	envelope.ExpandsRecipients = boolAtPath(arguments, contract.ExpandsField)

	body := valueAtPathOrNil(arguments, contract.BodyField)
	bodyText := stringValue(body)
	if bodyText == "" && body != nil {
		if encoded, err := json.Marshal(body); err == nil {
			bodyText = string(encoded)
		}
	}
	digest := sha256.Sum256([]byte(bodyText))
	envelope.BodyDigest = "sha256:" + hex.EncodeToString(digest[:])
	envelope.BodyPreview = excerpt(bodyText, 500)

	var rawRecipients []any
	if path := strings.TrimSpace(contract.RecipientsField); path != "" {
		value, exists := valueAtPath(arguments, path)
		if !exists {
			return tool.MessageEgressEnvelope{}, fmt.Errorf("recipients field %q is missing", path)
		}
		switch typed := value.(type) {
		case []any:
			rawRecipients = typed
		default:
			rawRecipients = []any{typed}
		}
	} else if path := strings.TrimSpace(contract.RecipientField); path != "" {
		value, exists := valueAtPath(arguments, path)
		if !exists {
			return tool.MessageEgressEnvelope{}, fmt.Errorf("recipient field %q is missing", path)
		}
		rawRecipients = []any{value}
	} else {
		return tool.MessageEgressEnvelope{}, fmt.Errorf("provider did not declare a recipient field")
	}

	for _, rawRecipient := range rawRecipients {
		recipient, err := messageRecipient(rawRecipient, contract)
		if err != nil {
			return tool.MessageEgressEnvelope{}, err
		}
		envelope.Recipients = append(envelope.Recipients, recipient)
	}
	envelope.EnvelopeDigest = tool.MessageEnvelopeDigest(envelope)
	return envelope, nil
}

func messageRecipient(value any, contract MCPMessageEgressContract) (tool.MessageRecipient, error) {
	recipient := tool.MessageRecipient{Kind: contract.RecipientKind}
	if object, ok := value.(map[string]any); ok {
		idField := strings.TrimSpace(contract.RecipientIDField)
		if idField == "" {
			idField = "id"
		}
		recipient.ID = stringAtPath(object, idField)
		if field := strings.TrimSpace(contract.RecipientKindField); field != "" {
			recipient.Kind = normalizeRecipientKind(stringAtPath(object, field))
		}
	} else {
		recipient.ID = strings.TrimSpace(stringValue(value))
	}
	if recipient.Kind == "" {
		recipient.Kind = tool.RecipientUnknown
	}
	if recipient.ID == "" {
		return tool.MessageRecipient{}, fmt.Errorf("recipient identity is empty")
	}
	if recipient.Kind == tool.RecipientUnknown {
		return tool.MessageRecipient{}, fmt.Errorf("recipient %q has unknown kind", recipient.ID)
	}
	return recipient, nil
}

func normalizeRecipientKind(value string) tool.MessageRecipientKind {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "individual", "person", "user", "member", "email", "direct", "dm":
		return tool.RecipientIndividual
	case "group", "chat_group", "chat-group":
		return tool.RecipientGroup
	case "channel", "room":
		return tool.RecipientChannel
	case "broadcast", "distribution_list", "distribution-list", "mailing_list", "mailing-list":
		return tool.RecipientBroadcast
	default:
		return tool.RecipientUnknown
	}
}

func schemaDeclaresPath(raw json.RawMessage, path string) bool {
	if len(raw) == 0 || strings.TrimSpace(path) == "" {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	current := any(schema)
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return false
		}
		properties, ok := object["properties"].(map[string]any)
		if !ok {
			return false
		}
		next, exists := properties[segment]
		if !exists {
			return false
		}
		current = next
	}
	return true
}

func schemaDeclaresRequiredStringPath(raw json.RawMessage, path string) bool {
	if len(raw) == 0 || strings.TrimSpace(path) == "" {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	segments := strings.Split(path, ".")
	current := schema
	for index, segment := range segments {
		properties, ok := current["properties"].(map[string]any)
		if !ok || !schemaRequiredContains(current, segment) {
			return false
		}
		next, exists := properties[segment]
		if !exists {
			return false
		}
		nextSchema, ok := next.(map[string]any)
		if !ok {
			return false
		}
		if index == len(segments)-1 {
			return nextSchema["type"] == "string"
		}
		if value, exists := nextSchema["type"]; exists && value != "object" {
			return false
		}
		current = nextSchema
	}
	return false
}

func schemaRequiredContains(schema map[string]any, field string) bool {
	values, ok := schema["required"].([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if text, ok := value.(string); ok && text == field {
			return true
		}
	}
	return false
}

func valueAtPath(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	current := root
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := object[segment]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

func valueAtPathOrNil(root any, path string) any {
	value, _ := valueAtPath(root, path)
	return value
}

func stringAtPath(root any, path string) string {
	value, ok := valueAtPath(root, path)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue(value))
}

func boolAtPath(root any, path string) bool {
	value, ok := valueAtPath(root, path)
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	}
	return false
}

func intAtPath(root any, path string) int {
	value, ok := valueAtPath(root, path)
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		if typed > float64(math.MaxInt) {
			return math.MaxInt
		}
		return int(typed)
	case int:
		return typed
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	}
	return 0
}

func stringsAtPath(root any, path string) []string {
	value, ok := valueAtPath(root, path)
	if !ok {
		return nil
	}
	var result []string
	switch typed := value.(type) {
	case []any:
		for _, entry := range typed {
			if text := observableString(entry); text != "" {
				result = append(result, text)
			}
		}
	case []string:
		for _, entry := range typed {
			if text := strings.TrimSpace(entry); text != "" {
				result = append(result, text)
			}
		}
	default:
		if text := observableString(value); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func observableString(value any) string {
	if text := strings.TrimSpace(stringValue(value)); text != "" {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) == "null" {
		return ""
	}
	return excerpt(string(encoded), 500)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}
