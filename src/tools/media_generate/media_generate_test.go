package mediagenerate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"awaken/src/tool"
)

type mediaStatusObserver struct {
	outputs []tool.CommandOutput
}

type failingRoundTripper struct {
	err error
}

func (transport failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
}

func (observer *mediaStatusObserver) OnCommandOutput(output tool.CommandOutput) {
	observer.outputs = append(observer.outputs, output)
}

func testConfiguration(kind string) runtimeConfiguration {
	capability := "media.image.generate"
	modelID := "doubao-seedream-test"
	if kind == "video" {
		capability = "media.video.generate"
		modelID = "doubao-seedance-test"
	}
	return runtimeConfiguration{
		storedConfiguration: storedConfiguration{
			ID: "test-" + kind, ProviderID: providerID, AccountID: defaultAccount,
			Capability: capability, ModelID: modelID, ModelKind: kind,
			Enabled: true, ConfigurationRevision: 2,
		},
		APIKey: "secret-test-key", CredentialRevision: 3,
	}
}

func TestDefinitionPinsProviderWithoutExposingModelArgument(t *testing.T) {
	media := newMediaTool(testConfiguration("image"))
	if err := tool.ValidateToolWithMode(media, tool.DefinitionValidationStrictBuiltin); err != nil {
		t.Fatalf("strict definition: %v", err)
	}
	definition := media.Definition()
	if _, exists := definition.InputSchema.Properties["generation_model"]; exists {
		t.Fatal("generation_model must not be model-visible")
	}
	if !strings.Contains(string(definition.ProviderID), "doubao-seedream-test") {
		t.Fatalf("provider identity must pin configured model: %q", definition.ProviderID)
	}
	encoded, _ := json.Marshal(definition)
	if strings.Contains(string(encoded), "secret-test-key") {
		t.Fatal("provider definition leaked credential")
	}
	if definition.InputSchema.Properties["mode"].Type != "string" ||
		definition.InputSchema.Properties["max_images"].Minimum == nil ||
		definition.InputSchema.Properties["max_images"].Maximum == nil {
		t.Fatalf("image group schema missing: %+v", definition.InputSchema.Properties)
	}
	for _, field := range []string{"kind", "model_id", "task_id", "paths", "provider"} {
		if _, exists := definition.OutputSchema.Properties[field]; !exists {
			t.Fatalf("media output schema missing %q: %+v", field, definition.OutputSchema.Properties)
		}
	}
	videoDefinition := newMediaTool(testConfiguration("video")).Definition()
	for _, field := range []string{"source_image_path", "source_role", "task_id"} {
		if _, exists := videoDefinition.InputSchema.Properties[field]; !exists {
			t.Fatalf("video input schema missing %q: %+v", field, videoDefinition.InputSchema.Properties)
		}
	}
	for _, mediaDefinition := range []tool.ToolDefinition{definition, videoDefinition} {
		modelDescription := tool.ModelDescription(mediaDefinition)
		if !strings.Contains(modelDescription, "ready 媒体 Artifact") ||
			!strings.Contains(modelDescription, "不得再调用 artifact_publish") {
			t.Fatalf("media tool lost its direct-delivery boundary: %s", modelDescription)
		}
	}
}

func TestImageGenerateDownloadsAndPublishesLocalArtifact(t *testing.T) {
	var server *httptest.Server
	var providerRequest map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/images/generations":
			if request.Header.Get("Authorization") != "Bearer secret-test-key" {
				t.Fatal("missing provider authorization")
			}
			if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
				t.Fatal(err)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"data":[{"url":"` + server.URL + `/result.png"}]}`))
		case "/result.png":
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write([]byte("\x89PNG\r\n\x1a\nimage"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	configuration := testConfiguration("image")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext(
		"scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "",
	)
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	observer := &mediaStatusObserver{}
	outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"product hero","output_format":"png"}`, observer)
	if err != nil || outcome.IsError() {
		t.Fatalf("image outcome err=%v outcome=%+v", err, outcome)
	}
	if len(outcome.Artifacts) != 1 || outcome.Artifacts[0].Role != tool.ArtifactRoleDeliverable {
		t.Fatalf("unexpected artifacts: %+v", outcome.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(outcome.Artifacts[0].Path))); err != nil {
		t.Fatalf("generated image missing: %v", err)
	}
	if providerRequest["model"] != "doubao-seedream-test" ||
		providerRequest["output_format"] != "png" ||
		providerRequest["sequential_image_generation"] != "disabled" {
		t.Fatalf("unexpected image provider request: %+v", providerRequest)
	}
	if _, exists := providerRequest["sequential_image_generation_options"]; exists {
		t.Fatalf("single image request must not include group options: %+v", providerRequest)
	}
	if len(observer.outputs) != 3 || observer.outputs[0].Stream != "media_status" {
		t.Fatalf("unexpected media progress outputs: %+v", observer.outputs)
	}
	if !strings.Contains(observer.outputs[0].Delta, `"stage":"generating"`) ||
		!strings.Contains(observer.outputs[1].Delta, `"stage":"downloading"`) ||
		!strings.Contains(observer.outputs[2].Delta, `"stage":"ready"`) {
		t.Fatalf("unexpected media progress stages: %+v", observer.outputs)
	}
	definition := media.Definition()
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: media.Name(), DeclaredSideEffects: definition.SideEffects,
		OutputSchema: definition.OutputSchema, StrictOutputSchema: true,
	})
	if normalized.IsError() || normalized.ErrorCode == "tool_output_schema_mismatch" {
		t.Fatalf("media outcome violated its strict output schema: %+v", normalized)
	}
}

func TestImageGenerateGroupMapsProviderFieldsAndPublishesEveryResult(t *testing.T) {
	var server *httptest.Server
	var providerRequest map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/images/generations":
			if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
				t.Fatal(err)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"data":[{"url":"` + server.URL + `/result-1.png"},{"url":"` + server.URL + `/result-2.png"},{"url":"` + server.URL + `/result-3.png"}]}`))
		case "/result-1.png", "/result-2.png", "/result-3.png":
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write([]byte("\x89PNG\r\n\x1a\n" + request.URL.Path))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	configuration := testConfiguration("image")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext(
		"scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "",
	)
	observer := &mediaStatusObserver{}
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"three product views","mode":"group","max_images":3,"output_format":"png"}`, observer)
	if err != nil || outcome.IsError() {
		t.Fatalf("group image outcome err=%v outcome=%+v", err, outcome)
	}
	if len(outcome.Artifacts) != 3 || len(outcome.AffectedPaths) != 3 {
		t.Fatalf("expected every group result as an artifact: %+v", outcome)
	}
	for _, artifact := range outcome.Artifacts {
		if artifact.Role != tool.ArtifactRoleDeliverable || artifact.Kind != "image" {
			t.Fatalf("unexpected group artifact: %+v", artifact)
		}
		if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(artifact.Path))); err != nil {
			t.Fatalf("generated group image missing: %v", err)
		}
	}
	options, ok := providerRequest["sequential_image_generation_options"].(map[string]any)
	if providerRequest["sequential_image_generation"] != "auto" || !ok || options["max_images"] != float64(3) {
		t.Fatalf("unexpected group provider request: %+v", providerRequest)
	}
	if !strings.Contains(observer.outputs[len(observer.outputs)-1].Delta, `"message":"已生成 3 张图片"`) {
		t.Fatalf("group ready count missing from progress: %+v", observer.outputs)
	}
}

func TestImageGroupInputBoundsAndCombinations(t *testing.T) {
	media := newMediaTool(testConfiguration("image"))
	for _, input := range []string{
		`{"prompt":"one","mode":"group","max_images":1}`,
		`{"prompt":"fifteen","mode":"group","max_images":15}`,
		`{"prompt":"single","mode":"single"}`,
		`{"prompt":"default single"}`,
	} {
		if err := media.validateInput(input); err != nil {
			t.Fatalf("expected valid image input %s: %v", input, err)
		}
	}
	for _, input := range []string{
		`{"prompt":"missing max","mode":"group"}`,
		`{"prompt":"too many","mode":"group","max_images":16}`,
		`{"prompt":"single with max","mode":"single","max_images":2}`,
		`{"prompt":"unknown","mode":"sequence","max_images":2}`,
	} {
		if err := media.validateInput(input); err == nil {
			t.Fatalf("expected invalid image input: %s", input)
		}
	}
}

func TestInvalidImageGroupCombinationDoesNotCallProvider(t *testing.T) {
	providerCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		providerCalls++
		http.Error(response, "must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()
	configuration := testConfiguration("image")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"invalid","mode":"group","max_images":0}`, nil)
	if err != nil || !outcome.IsError() || outcome.ErrorCode != "media_generate_invalid_input" {
		t.Fatalf("expected validation outcome, got err=%v outcome=%+v", err, outcome)
	}
	if providerCalls != 0 {
		t.Fatalf("invalid input called provider %d times", providerCalls)
	}
}

func TestImageTransportFailureKeepsSafeDiagnosticAndFailedTransport(t *testing.T) {
	configuration := testConfiguration("image")
	configuration.BaseURL = "https://media.invalid"
	media := newMediaTool(configuration)
	media.client = &http.Client{Transport: failingRoundTripper{err: &net.DNSError{Err: "no such host", Name: "media.invalid"}}}

	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"group","mode":"group","max_images":15}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "media_generate_upstream_failed" {
		t.Fatalf("expected upstream failure, got %+v", outcome)
	}
	if outcome.TransportStatus != tool.TransportFailed {
		t.Fatalf("expected failed transport, got %q", outcome.TransportStatus)
	}
	if outcome.Diagnostic["transport_failure_kind"] != "dns" {
		t.Fatalf("expected safe dns diagnostic, got %+v", outcome.Diagnostic)
	}
	normalized := outcome.Normalize(tool.OutcomeNormalizationContext{
		OperationType: media.Name(), DeclaredSideEffects: media.Definition().SideEffects,
	})
	if normalized.TransportStatus != tool.TransportFailed ||
		normalized.Operation == nil || normalized.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("transport failure lost canonical unknown-side-effect semantics: %+v", normalized)
	}
	encoded, _ := json.Marshal(outcome)
	if strings.Contains(string(encoded), "secret-test-key") {
		t.Fatalf("transport diagnostic leaked credential: %s", encoded)
	}
}

func TestImageCreateTimeoutIsUnknownAndNotBlindlyRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(30 * time.Millisecond)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	configuration := testConfiguration("image")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.imageTimeout = 5 * time.Millisecond
	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"slow group","mode":"group","max_images":15}`, nil)
	if err != nil || !outcome.IsError() {
		t.Fatalf("expected typed timeout outcome, err=%v outcome=%+v", err, outcome)
	}
	if outcome.TransportStatus != tool.TransportUnknown || outcome.Retryable || outcome.Diagnostic["transport_failure_kind"] != "timeout" {
		t.Fatalf("image create timeout must be unknown and non-retryable: %+v", outcome)
	}
}

func TestImageGroupPreservesReadyArtifactsWhenSiblingDownloadFails(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/images/generations":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"data":[{"url":"` + server.URL + `/ok.png"},{"url":"` + server.URL + `/missing.png"}]}`))
		case "/ok.png":
			response.Header().Set("Content-Type", "image/png")
			_, _ = response.Write([]byte("\x89PNG\r\n\x1a\nready"))
		case "/missing.png":
			http.Error(response, "missing", http.StatusBadGateway)
		}
	}))
	defer server.Close()
	configuration := testConfiguration("image")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext("scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "")
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"two","mode":"group","max_images":2}`, nil)
	if err != nil || !outcome.IsError() || outcome.ErrorCode != "media_generate_partial" {
		t.Fatalf("expected typed partial outcome, err=%v outcome=%+v", err, outcome)
	}
	if len(outcome.Artifacts) != 1 || len(outcome.AffectedPaths) != 1 || outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectPartial {
		t.Fatalf("partial outcome lost ready artifact: %+v", outcome)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(outcome.Artifacts[0].Path))); err != nil {
		t.Fatalf("ready partial artifact was removed: %v", err)
	}
	if !strings.Contains(outcome.Content, `"generation_status":"partial"`) || !strings.Contains(outcome.Content, `"status":"error"`) {
		t.Fatalf("partial per-item result missing: %s", outcome.Content)
	}
}

func TestRejectsProviderControlFlagsInsidePrompt(t *testing.T) {
	media := newMediaTool(testConfiguration("video"))
	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"make it cinematic --model another-model","ratio":"16:9"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.IsError() || outcome.ErrorCode != "media_generate_invalid_input" {
		t.Fatalf("expected validation failure, got %+v", outcome)
	}
}

func TestMediaDownloadRejectsUnsafeRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "http://192.168.1.20/result.mp4", http.StatusFound)
	}))
	defer server.Close()

	media := newMediaTool(testConfiguration("video"))
	_, _, err := media.download(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "下载失败") {
		t.Fatalf("expected unsafe redirect download failure, got %v", err)
	}
}

func TestVideoGeneratePollsTerminalTaskAndPublishesMP4(t *testing.T) {
	var server *httptest.Server
	var providerRequest map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/contents/generations/tasks":
			if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
				t.Fatal(err)
			}
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"id":"cgt-test"}`))
		case "/contents/generations/tasks/cgt-test":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"status":"succeeded","content":{"video_url":"` + server.URL + `/result.mp4"}}`))
		case "/result.mp4":
			response.Header().Set("Content-Type", "video/mp4")
			_, _ = response.Write([]byte("\x00\x00\x00\x18ftypmp42video-bytes"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.pollInterval = time.Millisecond
	media.pollTimeout = time.Second
	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext(
		"scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "",
	)
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"slow camera move","duration":5,"ratio":"16:9"}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("video outcome err=%v outcome=%+v", err, outcome)
	}
	if len(outcome.Artifacts) != 1 || outcome.Artifacts[0].Kind != "video" {
		t.Fatalf("unexpected artifacts: %+v", outcome.Artifacts)
	}
	if !strings.Contains(outcome.Content, `"task_id":"cgt-test"`) {
		t.Fatalf("missing durable task id: %s", outcome.Content)
	}
	if providerRequest["model"] != "doubao-seedance-test" || providerRequest["ratio"] != "16:9" || providerRequest["duration"] != float64(5) {
		t.Fatalf("unexpected video provider request: %+v", providerRequest)
	}
}

func TestVideoGenerateUsesReadyImageArtifactAsFirstFrame(t *testing.T) {
	var server *httptest.Server
	var providerRequest map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/contents/generations/tasks":
			if err := json.NewDecoder(request.Body).Decode(&providerRequest); err != nil {
				t.Fatal(err)
			}
			_, _ = response.Write([]byte(`{"id":"cgt-image"}`))
		case "/contents/generations/tasks/cgt-image":
			_, _ = response.Write([]byte(`{"status":"succeeded","content":{"video_url":"` + server.URL + `/result.mp4"}}`))
		case "/result.mp4":
			response.Header().Set("Content-Type", "video/mp4")
			_, _ = response.Write([]byte("\x00\x00\x00\x18ftypmp42video"))
		}
	}))
	defer server.Close()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "output"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "output", "source.png"), []byte("\x89PNG\r\n\x1a\nsource"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.pollInterval = time.Millisecond
	permission := tool.NewTaskScopePermissionContext("scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "")
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"make it move","source_image_path":"output/source.png","source_role":"first_frame"}`, nil)
	if err != nil || outcome.IsError() {
		t.Fatalf("image-to-video failed, err=%v outcome=%+v", err, outcome)
	}
	content, ok := providerRequest["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("video request did not include source image: %+v", providerRequest)
	}
	imagePart, _ := content[1].(map[string]any)
	imageURL, _ := imagePart["image_url"].(map[string]any)
	if _, hasRole := imagePart["role"]; hasRole || !strings.HasPrefix(fmt.Sprint(imageURL["url"]), "data:image/png;base64,") {
		t.Fatalf("unexpected image-to-video content: %+v", imagePart)
	}
}

func TestLargeVideoSourceImageIsCompactedBeforeInlineUpload(t *testing.T) {
	workspace := t.TempDir()
	outputDir := filepath.Join(workspace, "output")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	input := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	state := uint32(1)
	for index := range input.Pix {
		state = state*1664525 + 1013904223
		input.Pix[index] = byte(state >> 24)
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, input); err != nil {
		t.Fatal(err)
	}
	if raw.Len() <= maximumInlineVideoImageBytes {
		t.Fatalf("test fixture did not exceed inline threshold: %d", raw.Len())
	}
	if err := os.WriteFile(filepath.Join(outputDir, "large.png"), raw.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	permission := tool.NewTaskScopePermissionContext("scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "")
	dataURL, err := sourceImageDataURL(permission, "output/large.png")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dataURL, "data:image/jpeg;base64,") {
		t.Fatalf("expected compact JPEG data URL, got prefix %q", dataURL[:min(len(dataURL), 40)])
	}
	encoded := strings.TrimPrefix(dataURL, "data:image/jpeg;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) > maximumInlineVideoImageBytes {
		t.Fatalf("compacted video input is still too large: %d", len(decoded))
	}
}

func TestVideoCreateTimeoutRequiresExternalStateConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		time.Sleep(30 * time.Millisecond)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"id":"late-task"}`))
	}))
	defer server.Close()
	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.videoCreateTimeout = 5 * time.Millisecond
	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"slow task"}`, nil)
	if err != nil || !outcome.IsError() {
		t.Fatalf("expected typed create timeout, err=%v outcome=%+v", err, outcome)
	}
	if outcome.ErrorCode != "media_generate_create_unknown" || outcome.TransportStatus != tool.TransportUnknown ||
		outcome.SemanticStatus != tool.SemanticUnknown || outcome.Retryable ||
		outcome.Diagnostic["requires_external_state_confirmation"] != true {
		t.Fatalf("video create timeout must require external confirmation: %+v", outcome)
	}
	if outcome.Operation == nil || outcome.Operation.SideEffectState != tool.OutcomeSideEffectUnknown {
		t.Fatalf("video create timeout lost unknown side effect: %+v", outcome.Operation)
	}
}

func TestVideoResumeUsesExistingTaskAndPollTimeoutReturnsUnknown(t *testing.T) {
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			createCalls++
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()
	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.pollInterval = time.Millisecond
	media.pollTimeout = 5 * time.Millisecond
	observer := &mediaStatusObserver{}
	outcome, err := media.ExecuteOutcome(context.Background(), `{"prompt":"continue","task_id":"cgt-existing"}`, observer)
	if err != nil || !outcome.IsError() || outcome.ErrorCode != "media_generate_task_pending" {
		t.Fatalf("expected pending video outcome, err=%v outcome=%+v", err, outcome)
	}
	if createCalls != 0 || outcome.TransportStatus != tool.TransportUnknown || outcome.Retryable || !strings.Contains(outcome.Content, `"task_id":"cgt-existing"`) {
		t.Fatalf("resume must not create or hide task identity: calls=%d outcome=%+v", createCalls, outcome)
	}
	if len(observer.outputs) == 0 || !strings.Contains(observer.outputs[0].Delta, `"task_id":"cgt-existing"`) {
		t.Fatalf("task identity missing from durable media status: %+v", observer.outputs)
	}
}

func TestVideoSameRequestAfterRestartReusesDurablePendingTask(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	createCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			createCalls++
			_, _ = response.Write([]byte(`{"id":"cgt-durable"}`))
			return
		}
		_, _ = response.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()
	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext("scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "")
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	for attempt := 0; attempt < 2; attempt++ {
		media := newMediaTool(configuration)
		media.client = server.Client()
		media.pollInterval = time.Millisecond
		media.pollTimeout = 4 * time.Millisecond
		outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"same durable task","duration":5}`, nil)
		if err != nil || outcome.ErrorCode != "media_generate_task_pending" || !strings.Contains(outcome.Content, `"task_id":"cgt-durable"`) {
			t.Fatalf("attempt %d did not preserve pending task: err=%v outcome=%+v", attempt, err, outcome)
		}
	}
	if createCalls != 1 {
		t.Fatalf("same request after restart created %d provider tasks; expected one", createCalls)
	}
}

func TestConcurrentSameVideoRequestCreatesOneProviderTask(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	var createCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			createCalls.Add(1)
			time.Sleep(10 * time.Millisecond)
			_, _ = response.Write([]byte(`{"id":"cgt-concurrent"}`))
			return
		}
		_, _ = response.Write([]byte(`{"status":"running"}`))
	}))
	defer server.Close()

	workspace := t.TempDir()
	permission := tool.NewTaskScopePermissionContext("scope", workspace, []string{workspace}, []string{workspace}, []string{workspace}, nil, nil, "")
	ctx := tool.ContextWithPermissionContext(context.Background(), permission)
	configuration := testConfiguration("video")
	configuration.BaseURL = server.URL
	media := newMediaTool(configuration)
	media.client = server.Client()
	media.pollInterval = time.Millisecond
	media.pollTimeout = 4 * time.Millisecond

	type result struct {
		outcome tool.ExecutionOutcome
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			outcome, err := media.ExecuteOutcome(ctx, `{"prompt":"same concurrent task","duration":5}`, nil)
			results <- result{outcome: outcome, err: err}
		}()
	}
	for range 2 {
		got := <-results
		if got.err != nil || got.outcome.ErrorCode != "media_generate_task_pending" || !strings.Contains(got.outcome.Content, `"task_id":"cgt-concurrent"`) {
			t.Fatalf("concurrent request lost resumable task: err=%v outcome=%+v", got.err, got.outcome)
		}
	}
	if got := createCalls.Load(); got != 1 {
		t.Fatalf("concurrent same request created %d provider tasks; expected one", got)
	}
}

func TestLoadRuntimeConfigurationsRequiresEnabledTypedConfigAndCredential(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AWAKEN_HOME", root)
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	account := `{"version":2,"accountId":"volcengine-agent-plan-default","credentialRevision":4,"catalog":[{"id":"doubao-seedream-test","kind":"image"},{"id":"glm-test","kind":"llm"}]}`
	media := `{"version":1,"configurations":[` +
		`{"id":"image","providerId":"volcengine-agent-plan","accountId":"volcengine-agent-plan-default","capability":"media.image.generate","modelId":"doubao-seedream-test","modelKind":"image","enabled":true,"configurationRevision":2},` +
		`{"id":"wrong","providerId":"volcengine-agent-plan","accountId":"volcengine-agent-plan-default","capability":"media.video.generate","modelId":"glm-test","modelKind":"image","enabled":true,"configurationRevision":1}` +
		`]}`
	if err := os.WriteFile(filepath.Join(root, "config", "agent-plan-account.json"), []byte(account), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "agent-plan-media-tools.json"), []byte(media), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("AWAKEN_AGENT_PLAN_API_KEY_volcengine-agent-plan-default=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configurations := loadRuntimeConfigurations()
	if len(configurations) != 1 || configurations[0].ModelKind != "image" || configurations[0].CredentialRevision != 4 {
		t.Fatalf("unexpected runtime configurations: %+v", configurations)
	}
}
