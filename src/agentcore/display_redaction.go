package agentcore

import "awaken/src/redaction"

func DisplaySafeEvent(event AwakenEvent) AwakenEvent {
	event.Delta = redaction.RedactString(event.Delta)
	event.Content = redaction.RedactString(event.Content)
	event.Arguments = redaction.RedactString(event.Arguments)
	event.Progress = redaction.RedactString(event.Progress)
	event.Patch = redaction.RedactString(event.Patch)
	event.PatchSummary = redaction.RedactString(event.PatchSummary)
	event.Diff = redaction.RedactString(event.Diff)
	event.Error = redaction.RedactString(event.Error)
	event.ApprovalTitle = redaction.RedactString(event.ApprovalTitle)
	event.ApprovalDescription = redaction.RedactString(event.ApprovalDescription)
	event.ApprovalPatchSummary = redaction.RedactString(event.ApprovalPatchSummary)
	event.ApprovalAffectedPaths = redactDisplayStrings(event.ApprovalAffectedPaths)
	event.ArtifactTitle = redaction.RedactString(event.ArtifactTitle)
	event.ArtifactDescription = redaction.RedactString(event.ArtifactDescription)
	event.ArtifactPath = redaction.RedactString(event.ArtifactPath)
	event.ArtifactContent = redaction.RedactString(event.ArtifactContent)
	return event
}

func DisplaySafeApprovalRequest(request ApprovalRequest) ApprovalRequest {
	request.Title = redaction.RedactString(request.Title)
	request.Description = redaction.RedactString(request.Description)
	request.PatchSummary = redaction.RedactString(request.PatchSummary)
	request.AffectedPaths = redactDisplayStrings(request.AffectedPaths)
	return request
}

func redactDisplayStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = redaction.RedactString(value)
	}
	return out
}
