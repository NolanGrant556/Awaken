package researchruntime

import (
	"encoding/json"
	"fmt"
	"strings"
)

type PlannerOutput struct {
	Questions []Question `json:"questions"`
}

type Question struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type ResearcherOutput struct {
	Claims        []RawClaim `json:"claims"`
	Uncertainties []string   `json:"uncertainties"`
}

type RawClaim struct {
	ID         string        `json:"id"`
	QuestionID string        `json:"question_id"`
	Statement  string        `json:"statement"`
	Confidence string        `json:"confidence"`
	Evidence   []RawEvidence `json:"evidence"`
}

type RawEvidence struct {
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Locator    string `json:"locator"`
	SourceType string `json:"source_type"`
	ReceiptID  string `json:"receipt_id,omitempty"`
}

type VerifierOutput struct {
	Verdicts []RawVerdict `json:"verdicts"`
}

type RawVerdict struct {
	ClaimID string `json:"claim_id"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

type SynthOutput struct {
	Answer      string   `json:"answer"`
	ClaimIDs    []string `json:"claim_ids"`
	Limitations []string `json:"limitations"`
}

func parseJSON[T any](raw string, dest *T) error {
	raw = extractJSONObject(raw)
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("empty json")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	return nil
}

func extractJSONObject(raw string) string {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ""
	}
	return raw[start : end+1]
}
