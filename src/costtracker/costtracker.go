package costtracker

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const microsPerUSD int64 = 1_000_000

type MoneyMicros int64

type CostStatus string

const (
	CostStatusActual    CostStatus = "actual"
	CostStatusEstimated CostStatus = "estimated"
	CostStatusIncluded  CostStatus = "included"
	CostStatusUnknown   CostStatus = "unknown"
)

type CostSource string

const (
	CostSourceProviderCostAPI       CostSource = "provider_cost_api"
	CostSourceProviderGenerationAPI CostSource = "provider_generation_api"
	CostSourceProviderModelsAPI     CostSource = "provider_models_api"
	CostSourceOfficialDocsSnapshot  CostSource = "official_docs_snapshot"
	CostSourceUserOverride          CostSource = "user_override"
	CostSourceCustomContract        CostSource = "custom_contract"
	CostSourceNone                  CostSource = "none"
)

type Usage struct {
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
	WebSearches      int64 `json:"web_searches,omitempty"`
	RequestCount     int64 `json:"request_count,omitempty"`
}

type Pricing struct {
	InputMicrosPerMillion      *int64     `json:"input_micros_per_million,omitempty"`
	OutputMicrosPerMillion     *int64     `json:"output_micros_per_million,omitempty"`
	CacheReadMicrosPerMillion  *int64     `json:"cache_read_micros_per_million,omitempty"`
	CacheWriteMicrosPerMillion *int64     `json:"cache_write_micros_per_million,omitempty"`
	RequestMicros              *int64     `json:"request_micros,omitempty"`
	Status                     CostStatus `json:"status,omitempty"`
	Source                     CostSource `json:"source,omitempty"`
	PricingVersion             string     `json:"pricing_version,omitempty"`
}

type CostResult struct {
	AmountMicros   MoneyMicros `json:"amount_micros"`
	Status         CostStatus  `json:"status"`
	Source         CostSource  `json:"source"`
	Model          string      `json:"model"`
	CanonicalModel string      `json:"canonical_model"`
	PricingVersion string      `json:"pricing_version,omitempty"`
	Notes          []string    `json:"notes,omitempty"`
}

type ModelUsage struct {
	Model            string      `json:"model"`
	InputTokens      int64       `json:"input_tokens,omitempty"`
	OutputTokens     int64       `json:"output_tokens,omitempty"`
	CacheReadTokens  int64       `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64       `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64       `json:"reasoning_tokens,omitempty"`
	WebSearches      int64       `json:"web_searches,omitempty"`
	RequestCount     int64       `json:"request_count,omitempty"`
	CostMicros       MoneyMicros `json:"cost_micros"`
	HasUnknownCost   bool        `json:"has_unknown_cost,omitempty"`
}

type SessionTotals struct {
	InputTokens         int64         `json:"input_tokens,omitempty"`
	OutputTokens        int64         `json:"output_tokens,omitempty"`
	CacheReadTokens     int64         `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens    int64         `json:"cache_write_tokens,omitempty"`
	ReasoningTokens     int64         `json:"reasoning_tokens,omitempty"`
	WebSearches         int64         `json:"web_searches,omitempty"`
	RequestCount        int64         `json:"request_count,omitempty"`
	CostMicros          MoneyMicros   `json:"cost_micros"`
	APIDuration         time.Duration `json:"api_duration,omitempty"`
	WallDuration        time.Duration `json:"wall_duration,omitempty"`
	LinesAdded          int64         `json:"lines_added,omitempty"`
	LinesRemoved        int64         `json:"lines_removed,omitempty"`
	HasUnknownModelCost bool          `json:"has_unknown_model_cost,omitempty"`
}

type Snapshot struct {
	Totals SessionTotals         `json:"totals"`
	Models map[string]ModelUsage `json:"models,omitempty"`
}

type Tracker struct {
	mu     sync.Mutex
	totals SessionTotals
	models map[string]ModelUsage
}

func NewTracker() *Tracker {
	return &Tracker{models: make(map[string]ModelUsage)}
}

func AddUsage(model string, usage Usage, pricing Pricing) CostResult {
	tracker := NewTracker()
	return tracker.AddUsage(model, usage, pricing)
}

func (t *Tracker) AddUsage(model string, usage Usage, pricing Pricing) CostResult {
	result := CalculateCost(model, usage, pricing)
	canonical := result.CanonicalModel

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.models == nil {
		t.models = make(map[string]ModelUsage)
	}

	t.totals.InputTokens += nonNegative(usage.InputTokens)
	t.totals.OutputTokens += nonNegative(usage.OutputTokens)
	t.totals.CacheReadTokens += nonNegative(usage.CacheReadTokens)
	t.totals.CacheWriteTokens += nonNegative(usage.CacheWriteTokens)
	t.totals.ReasoningTokens += nonNegative(usage.ReasoningTokens)
	t.totals.WebSearches += nonNegative(usage.WebSearches)
	t.totals.RequestCount += nonNegative(usage.RequestCount)
	if result.Status == CostStatusUnknown {
		t.totals.HasUnknownModelCost = true
	} else {
		t.totals.CostMicros += result.AmountMicros
	}

	modelUsage := t.models[canonical]
	if modelUsage.Model == "" {
		modelUsage.Model = canonical
	}
	modelUsage.InputTokens += nonNegative(usage.InputTokens)
	modelUsage.OutputTokens += nonNegative(usage.OutputTokens)
	modelUsage.CacheReadTokens += nonNegative(usage.CacheReadTokens)
	modelUsage.CacheWriteTokens += nonNegative(usage.CacheWriteTokens)
	modelUsage.ReasoningTokens += nonNegative(usage.ReasoningTokens)
	modelUsage.WebSearches += nonNegative(usage.WebSearches)
	modelUsage.RequestCount += nonNegative(usage.RequestCount)
	if result.Status == CostStatusUnknown {
		modelUsage.HasUnknownCost = true
	} else {
		modelUsage.CostMicros += result.AmountMicros
	}
	t.models[canonical] = modelUsage

	return result
}

func CalculateCost(model string, usage Usage, pricing Pricing) CostResult {
	canonical := CanonicalModelName(model)
	result := CostResult{
		Status:         CostStatusEstimated,
		Source:         pricing.Source,
		Model:          strings.TrimSpace(model),
		CanonicalModel: canonical,
		PricingVersion: pricing.PricingVersion,
	}
	if result.Source == "" {
		result.Source = CostSourceNone
	}
	if pricing.Status != "" {
		result.Status = pricing.Status
	}

	if canonical == "unknown" {
		result.Status = CostStatusUnknown
		result.Notes = append(result.Notes, "model name is unknown")
		return result
	}

	if result.Status == CostStatusIncluded {
		result.Source = CostSourceNone
		return result
	}
	if result.Status == CostStatusUnknown {
		result.Notes = append(result.Notes, "pricing is unknown")
		return result
	}
	if result.Status == "" {
		result.Status = CostStatusEstimated
	}

	var amount MoneyMicros
	if usage.InputTokens > 0 {
		if pricing.InputMicrosPerMillion == nil {
			return unknownPricingResult(result, "input pricing unavailable")
		}
		amount += tokenCost(usage.InputTokens, *pricing.InputMicrosPerMillion)
	}
	if usage.OutputTokens > 0 {
		if pricing.OutputMicrosPerMillion == nil {
			return unknownPricingResult(result, "output pricing unavailable")
		}
		amount += tokenCost(usage.OutputTokens, *pricing.OutputMicrosPerMillion)
	}
	if usage.CacheReadTokens > 0 {
		if pricing.CacheReadMicrosPerMillion == nil {
			return unknownPricingResult(result, "cache-read pricing unavailable")
		}
		amount += tokenCost(usage.CacheReadTokens, *pricing.CacheReadMicrosPerMillion)
	}
	if usage.CacheWriteTokens > 0 {
		if pricing.CacheWriteMicrosPerMillion == nil {
			return unknownPricingResult(result, "cache-write pricing unavailable")
		}
		amount += tokenCost(usage.CacheWriteTokens, *pricing.CacheWriteMicrosPerMillion)
	}
	if usage.RequestCount > 0 && pricing.RequestMicros != nil {
		amount += MoneyMicros(usage.RequestCount * *pricing.RequestMicros)
	}
	result.AmountMicros = amount
	return result
}

func (t *Tracker) AddDuration(api, wall time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totals.APIDuration += api
	t.totals.WallDuration += wall
}

func (t *Tracker) AddLinesChanged(added, removed int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totals.LinesAdded += nonNegative(added)
	t.totals.LinesRemoved += nonNegative(removed)
}

func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
		Totals: t.totals,
		Models: copyModels(t.models),
	}
}

func (t *Tracker) Restore(snapshot Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totals = snapshot.Totals
	t.models = copyModels(snapshot.Models)
	if t.models == nil {
		t.models = make(map[string]ModelUsage)
	}
}

func Restore(snapshot Snapshot) *Tracker {
	tracker := NewTracker()
	tracker.Restore(snapshot)
	return tracker
}

func CanonicalModelName(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return "unknown"
	}
	if alias, ok := modelAliases[name]; ok {
		return alias
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 && slash < len(name)-1 {
		withoutProvider := name[slash+1:]
		if alias, ok := modelAliases[withoutProvider]; ok {
			return alias
		}
		return withoutProvider
	}
	return name
}

func FormatCost(amount MoneyMicros) string {
	micros := int64(amount)
	if micros < 0 {
		return "-" + FormatCost(MoneyMicros(-micros))
	}
	if micros >= microsPerUSD/2 {
		return "$" + formatDecimalRounded(micros, 2)
	}
	return "$" + formatDecimalRounded(micros, 4)
}

func FormatSummary(snapshot Snapshot) string {
	lines := []string{
		"Total cost:            " + formatCostWithUnknown(snapshot.Totals),
		"Total duration (API):  " + formatDuration(snapshot.Totals.APIDuration),
		"Total duration (wall): " + formatDuration(snapshot.Totals.WallDuration),
		fmt.Sprintf("Total code changes:    %d %s added, %d %s removed",
			snapshot.Totals.LinesAdded,
			plural(snapshot.Totals.LinesAdded, "line", "lines"),
			snapshot.Totals.LinesRemoved,
			plural(snapshot.Totals.LinesRemoved, "line", "lines"),
		),
	}

	models := make([]string, 0, len(snapshot.Models))
	for model := range snapshot.Models {
		models = append(models, model)
	}
	sort.Strings(models)
	if len(models) == 0 {
		lines = append(lines, "Usage:                 0 input, 0 output, 0 cache read, 0 cache write")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "Usage by model:")
	for _, model := range models {
		usage := snapshot.Models[model]
		cost := FormatCost(usage.CostMicros)
		if usage.HasUnknownCost {
			cost += " (unknown pricing)"
		}
		line := fmt.Sprintf("%s: %d input, %d output, %d cache read, %d cache write",
			usage.Model,
			usage.InputTokens,
			usage.OutputTokens,
			usage.CacheReadTokens,
			usage.CacheWriteTokens,
		)
		if usage.WebSearches > 0 {
			line += fmt.Sprintf(", %d web search", usage.WebSearches)
		}
		line += " (" + cost + ")"
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func Micros(value int64) *int64 {
	return &value
}

var modelAliases = map[string]string{
	"claude-3-5-sonnet-20241022":         "claude-3.5-sonnet",
	"claude-3-5-sonnet-latest":           "claude-3.5-sonnet",
	"claude-sonnet-4-20250514":           "claude-sonnet-4",
	"claude-opus-4-20250514":             "claude-opus-4",
	"gpt-4o-2024-08-06":                  "gpt-4o",
	"gpt-4o-mini-2024-07-18":             "gpt-4o-mini",
	"openai/gpt-4o-2024-08-06":           "gpt-4o",
	"anthropic/claude-sonnet-4-20250514": "claude-sonnet-4",
}

func unknownPricingResult(result CostResult, note string) CostResult {
	result.AmountMicros = 0
	result.Status = CostStatusUnknown
	result.Notes = append(result.Notes, note)
	return result
}

func tokenCost(tokens, microsPerMillion int64) MoneyMicros {
	if tokens <= 0 || microsPerMillion == 0 {
		return 0
	}
	product := tokens * microsPerMillion
	if product >= 0 {
		return MoneyMicros((product + 500_000) / 1_000_000)
	}
	return MoneyMicros((product - 500_000) / 1_000_000)
}

func formatCostWithUnknown(totals SessionTotals) string {
	formatted := FormatCost(totals.CostMicros)
	if totals.HasUnknownModelCost {
		return formatted + " (costs may be inaccurate due to unknown pricing)"
	}
	return formatted
}

func formatDecimalRounded(micros int64, places int) string {
	unit := int64(1)
	for i := 0; i < 6-places; i++ {
		unit *= 10
	}
	rounded := (micros + unit/2) / unit * unit
	whole := rounded / microsPerUSD
	fraction := rounded % microsPerUSD
	width := 6
	digits := fmt.Sprintf("%0*d", width, fraction)
	return fmt.Sprintf("%d.%s", whole, digits[:places])
}

func formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int64(duration.Round(time.Second)/time.Second))
	}
	if duration < time.Hour {
		return fmt.Sprintf("%dm", int64(duration.Round(time.Minute)/time.Minute))
	}
	hours := int64(duration / time.Hour)
	minutes := int64((duration % time.Hour) / time.Minute)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func plural(value int64, singular, plural string) string {
	if value == 1 {
		return singular
	}
	return plural
}

func copyModels(source map[string]ModelUsage) map[string]ModelUsage {
	if source == nil {
		return nil
	}
	copied := make(map[string]ModelUsage, len(source))
	for model, usage := range source {
		copied[model] = usage
	}
	return copied
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
