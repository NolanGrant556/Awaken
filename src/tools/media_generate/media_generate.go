package mediagenerate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"awaken/src/tool"

	"github.com/google/uuid"
)

const (
	imageToolName          = "image_generate"
	videoToolName          = "video_generate"
	maximumJSONBytes       = 4 * 1024 * 1024
	maximumMediaBytes      = 128 * 1024 * 1024
	maximumInputImageBytes = 30 * 1024 * 1024
	// Keep inline video reference images comfortably below typical gateway
	// body limits after Base64 expansion. The original Artifact is untouched.
	maximumInlineVideoImageBytes = 2 * 1024 * 1024
	maximumVideoImageBytes       = 4 * 1024 * 1024
)

var providerControlFlag = regexp.MustCompile(`(?i)(?:^|\s)--(?:ratio|dur|duration|resolution|generate_audio|watermark|model)(?:\s|=|$)`)

var errUnsafeMediaRedirect = errors.New("unsafe media redirect")

var videoTaskSubmissionMu sync.Mutex

type mediaTool struct {
	configuration      runtimeConfiguration
	client             *http.Client
	requestTimeout     time.Duration
	videoCreateTimeout time.Duration
	imageTimeout       time.Duration
	downloadTimeout    time.Duration
	pollInterval       time.Duration
	pollTimeout        time.Duration
}

type imageInput struct {
	Prompt       string `json:"prompt"`
	Mode         string `json:"mode,omitempty"`
	MaxImages    int    `json:"max_images,omitempty"`
	Size         string `json:"size,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	Watermark    *bool  `json:"watermark,omitempty"`
}

type videoInput struct {
	Prompt          string `json:"prompt"`
	SourceImagePath string `json:"source_image_path,omitempty"`
	SourceRole      string `json:"source_role,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	Ratio           string `json:"ratio,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	GenerateAudio   *bool  `json:"generate_audio,omitempty"`
	Watermark       *bool  `json:"watermark,omitempty"`
}

type generatedItem struct {
	Index  int    `json:"index"`
	Status string `json:"status"`
	Path   string `json:"path,omitempty"`
}

type generatedResult struct {
	Status         string          `json:"generation_status"`
	Kind           string          `json:"kind"`
	ModelID        string          `json:"model_id"`
	TaskID         string          `json:"task_id,omitempty"`
	Paths          []string        `json:"paths"`
	Items          []generatedItem `json:"items,omitempty"`
	RequestedCount int             `json:"requested_count,omitempty"`
	FailedCount    int             `json:"failed_count,omitempty"`
	Provider       string          `json:"provider"`
}

type videoTaskRecord struct {
	Version     int       `json:"version"`
	Fingerprint string    `json:"fingerprint"`
	TaskID      string    `json:"task_id"`
	AccountID   string    `json:"account_id"`
	ModelID     string    `json:"model_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newMediaTool(configuration runtimeConfiguration) *mediaTool {
	return &mediaTool{
		configuration: configuration,
		client: &http.Client{
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many media redirects")
				}
				if !isAllowedRemoteURL(request.URL.String()) {
					return errUnsafeMediaRedirect
				}
				return nil
			},
		},
		requestTimeout:     45 * time.Second,
		videoCreateTimeout: 45 * time.Second,
		imageTimeout:       10 * time.Minute,
		downloadTimeout:    2 * time.Minute,
		pollInterval:       3 * time.Second,
		pollTimeout:        15 * time.Minute,
	}
}

func (t *mediaTool) Name() string {
	if t.configuration.ModelKind == "image" {
		return imageToolName
	}
	return videoToolName
}

func (t *mediaTool) Description() string {
	if t.configuration.ModelKind == "image" {
		return "根据用户目标生成图片并保存为本地交付物；生成模型已由设置固定，调用参数不得覆盖模型或凭据"
	}
	return "根据用户目标创建视频任务并保存本地 MP4；用户要求让已生成图片动起来时，必须把该 ready 图片 Artifact 的路径传入 source_image_path；任务已有 task_id 时必须续查原任务，不能重复创建计费任务"
}

func (t *mediaTool) Parameters() []tool.ToolParameter {
	parameters := []tool.ToolParameter{{Name: "prompt", Description: "完整的生成提示词和创作要求", Type: "string", Required: true}}
	if t.configuration.ModelKind == "image" {
		return append(parameters,
			tool.ToolParameter{Name: "mode", Description: "单图 single 或组图 group", Type: "string"},
			tool.ToolParameter{Name: "max_images", Description: "组图最多返回的图片数，1 到 15；仅 mode=group 时设置", Type: "integer"},
			tool.ToolParameter{Name: "size", Description: "官方支持的图片尺寸或分辨率，例如 2K；不确定时应省略并使用模型默认值", Type: "string"},
			tool.ToolParameter{Name: "output_format", Description: "期望的图片输出格式", Type: "string"},
			tool.ToolParameter{Name: "watermark", Description: "是否包含水印", Type: "boolean"},
		)
	}
	return append(parameters,
		tool.ToolParameter{Name: "source_image_path", Description: "可选；要作为视频首帧或参考图的 ready 图片 Artifact 路径", Type: "string"},
		tool.ToolParameter{Name: "source_role", Description: "图片作用：first_frame 或 reference_image", Type: "string"},
		tool.ToolParameter{Name: "task_id", Description: "可选；续查已经提交的视频任务，设置后不会创建新任务", Type: "string"},
		tool.ToolParameter{Name: "ratio", Description: "视频宽高比", Type: "string"},
		tool.ToolParameter{Name: "resolution", Description: "视频分辨率", Type: "string"},
		tool.ToolParameter{Name: "duration", Description: "视频时长（秒）", Type: "integer"},
		tool.ToolParameter{Name: "generate_audio", Description: "是否生成同步音轨", Type: "boolean"},
		tool.ToolParameter{Name: "watermark", Description: "是否包含水印", Type: "boolean"},
	)
}

func (t *mediaTool) InputSchema() tool.ToolParametersSchema {
	properties := map[string]tool.ToolParam{
		"prompt": {Type: "string", Description: "结合用户目标和项目上下文编写的完整生成提示词"},
	}
	if t.configuration.ModelKind == "image" {
		properties["mode"] = tool.ToolParam{Type: "string", Description: "生成单图或一组内容关联的图片", Enum: []any{"single", "group"}}
		properties["max_images"] = tool.ToolParam{Type: "integer", Description: "组图返回数量上限，不保证精确返回该数量；仅 mode=group 时设置", Minimum: tool.FloatPtr(1), Maximum: tool.FloatPtr(15)}
		properties["size"] = tool.ToolParam{Type: "string", Description: "官方支持的图片尺寸或分辨率，例如 2K；不确定时应省略并使用模型默认值"}
		properties["output_format"] = tool.ToolParam{Type: "string", Description: "图片输出格式", Enum: []any{"png", "jpeg"}}
		properties["watermark"] = tool.ToolParam{Type: "boolean", Description: "是否包含水印"}
	} else {
		properties["source_image_path"] = tool.ToolParam{Type: "string", Description: "ready 图片 Artifact 的工作区路径；当用户要求让上一张图片动起来时必须设置"}
		properties["source_role"] = tool.ToolParam{Type: "string", Description: "图片作为视频首帧或参考图", Enum: []any{"first_frame", "reference_image"}}
		properties["task_id"] = tool.ToolParam{Type: "string", Description: "续查已经提交的视频任务；设置后不再创建新任务"}
		properties["ratio"] = tool.ToolParam{Type: "string", Description: "视频宽高比", Enum: []any{"adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}}
		properties["resolution"] = tool.ToolParam{Type: "string", Description: "视频分辨率", Enum: []any{"480p", "720p", "1080p"}}
		properties["duration"] = tool.ToolParam{Type: "integer", Description: "视频时长（秒）"}
		properties["generate_audio"] = tool.ToolParam{Type: "boolean", Description: "是否生成同步音轨"}
		properties["watermark"] = tool.ToolParam{Type: "boolean", Description: "是否包含水印"}
	}
	return tool.ToolParametersSchema{
		Type: "object", Properties: properties, Required: []string{"prompt"},
		AdditionalProperties: tool.BoolPtr(false),
	}
}

func (t *mediaTool) Definition() tool.ToolDefinition {
	capability := tool.CapabilityImageGenerate
	when := "用户明确需要生成或编辑图片时"
	notWhen := "只需分析现有图片、查找资料或修改文档时"
	displayName := "Image Generate"
	if t.configuration.ModelKind == "video" {
		capability = tool.CapabilityVideoGenerate
		when = "用户明确需要生成视频或把创意制作成视频时"
		notWhen = "只需分析现有视频、编写脚本或生成静态图片时"
		displayName = "Video Generate"
	}
	providerIdentity := fmt.Sprintf(
		"%s:%s:%s:r%d:key%d",
		providerID,
		t.configuration.Capability,
		t.configuration.ModelID,
		t.configuration.ConfigurationRevision,
		t.configuration.CredentialRevision,
	)
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: t.Name(), DisplayName: displayName, Description: t.Description(),
		Purpose:   "让当前主 Agent 通过已配置的火山 Agent Plan 生成媒体文件；模型选择和凭据由 Host 固定。",
		WhenToUse: []string{when}, WhenNotToUse: []string{notWhen},
		OutputSchema: tool.ToolParam{Type: "object", Properties: map[string]tool.ToolParam{
			"generation_status": {Type: "string", Description: "succeeded、partial、failed 或 unknown"},
			"kind":              {Type: "string", Description: "生成的媒体类型"},
			"model_id":          {Type: "string", Description: "Host 配置固定的实际生成模型"},
			"task_id":           {Type: "string", Description: "异步视频任务 ID（图片为空）"},
			"paths":             {Type: "array", Description: "已验证并保存的本地媒体路径", Items: &tool.ToolParam{Type: "string", Description: "本地媒体路径"}},
			"items": {Type: "array", Description: "每个独立媒体结果的索引、状态与路径", Items: &tool.ToolParam{
				Type: "object", Description: "独立媒体结果",
				Properties: map[string]tool.ToolParam{
					"index":  {Type: "integer", Description: "从 1 开始的结果序号"},
					"status": {Type: "string", Description: "ready、error 或 unknown"},
					"path":   {Type: "string", Description: "ready 结果的本地路径"},
				},
				AdditionalProperties: tool.BoolPtr(false),
			}},
			"requested_count": {Type: "integer", Description: "本次请求的媒体数量"},
			"failed_count":    {Type: "integer", Description: "未能保存的媒体数量"},
			"provider":        {Type: "string", Description: "实际执行生成任务的供应商"},
		}},
		SideEffects: []tool.ToolSideEffect{
			{Kind: tool.SideEffectNetwork, Resources: []string{"volcengine Agent Plan generation API"}, Description: "调用外部生成服务，可能消耗套餐额度或产生费用。", Reversibility: tool.ReversibilityIrreversible},
			{Kind: tool.SideEffectCreate, Resources: []string{"workspace output media"}, Description: "把已验证的生成结果写入任务输出目录。", Reversibility: tool.ReversibilityConditional},
		},
		Examples: []tool.ToolExample{{Scenario: "根据用户描述生成媒体", Input: map[string]any{"prompt": "一张适合产品发布页的简洁主视觉"}, Outcome: "返回本地 ready 媒体 Artifact"}},
		FailureModes: []tool.ToolFailureMode{
			{Code: "media_generate_invalid_input", Cause: "提示词或参数无效", Retryable: false, Recovery: []string{"修正参数后重试"}},
			{Code: "media_generate_authorization_failed", Cause: "Agent Plan 凭据或套餐授权失败", Retryable: false, Recovery: []string{"在 Agent Plan 配置账户中检查凭据"}},
			{Code: "media_generate_rate_limited", Cause: "上游额度或并发限制", Retryable: true, Recovery: []string{"稍后重试"}},
			{Code: "media_generate_upstream_failed", Cause: "供应商请求或任务失败", Retryable: true, Recovery: []string{"检查参数或稍后重试"}},
			{Code: "media_generate_create_unknown", Cause: "视频任务提交没有返回任务 ID，供应商侧创建状态未知", Retryable: false, Recovery: []string{"检查火山控制台中的视频任务", "确认没有任务后再由用户明确重新创建"}},
			{Code: "media_generate_download_failed", Cause: "结果文件无法验证或下载", Retryable: true, Recovery: []string{"重试生成或检查网络"}},
			{Code: "media_generate_partial", Cause: "组图中仅部分结果成功保存", Retryable: false, Recovery: []string{"先使用已生成的图片，再明确补做缺失图片"}},
			{Code: "media_generate_task_pending", Cause: "视频任务已提交，但本次运行未能确认终态", Retryable: false, Recovery: []string{"使用原 task_id 续查，禁止重新创建任务"}},
			{Code: "media_generate_task_persistence_failed", Cause: "视频任务已创建，但本地任务 ID 未能可靠保存", Retryable: false, Recovery: []string{"保留当前 task_id 并续查，禁止重新创建任务"}},
		},
		Annotations: tool.ToolAnnotations{
			ReadOnly: false, Destructive: false, OpenWorld: false, ServerName: providerID,
			CapabilityClass: tool.CapabilityArtifactWrite, TrustedOrigin: tool.TrustedOriginOfficialHosted,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite, NetworkScope: tool.NetworkScopeRestricted,
		},
		ProviderID: tool.ProviderID(providerIdentity), Capabilities: []tool.Capability{capability},
	})
}

func (t *mediaTool) ProviderManifest() tool.ProviderManifest {
	definition := t.Definition()
	return tool.ProviderManifest{
		ProviderID: definition.ProviderID, ToolName: t.Name(), DisplayName: definition.DisplayName,
		Source: tool.SourceRuntime, Capabilities: definition.Capabilities, Readiness: tool.ProviderReady,
		SideEffects: definition.SideEffects,
	}
}

func (t *mediaTool) IsReadOnly() bool { return false }

func (t *mediaTool) CheckPermission(_ context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := t.validateInput(input); err != nil {
		return tool.DenyPermission(err.Error())
	}
	if _, err := permissionContext.ResolveOutputMutationPathChecked(filepath.Join("output", "generated-media", "probe")); err != nil {
		return tool.DenyPermission(err.Error())
	}
	if t.configuration.ModelKind == "video" {
		var parameters videoInput
		_ = json.Unmarshal([]byte(input), &parameters)
		if strings.TrimSpace(parameters.SourceImagePath) != "" {
			resolved, err := permissionContext.ResolveReadablePathChecked(parameters.SourceImagePath)
			if err != nil {
				return tool.DenyPermission(err.Error())
			}
			if _, err := os.Stat(resolved); err != nil {
				return tool.DenyPermission("source_image_path 必须指向可读取的 ready 图片 Artifact")
			}
		}
	}
	return tool.AllowPermission()
}

func (t *mediaTool) Execute(ctx context.Context, input string) (string, error) {
	outcome, err := t.ExecuteOutcome(ctx, input, nil)
	if err != nil {
		return "", err
	}
	if outcome.IsError() {
		return "", errors.New(outcome.ErrorMessage)
	}
	return outcome.Content, nil
}

func (t *mediaTool) ExecuteOutcome(ctx context.Context, input string, observer tool.ToolObserver) (tool.ExecutionOutcome, error) {
	started := time.Now().UTC()
	if err := t.validateInput(input); err != nil {
		return mediaFailure(err.Error(), "media_generate_invalid_input", tool.ErrorCategoryValidation, false).WithTiming(started, time.Now().UTC()), nil
	}
	permissionContext := tool.PermissionContextFromContext(ctx)
	var result generatedResult
	var artifacts []tool.ArtifactDescriptor
	var err error
	if t.configuration.ModelKind == "image" {
		result, artifacts, err = t.generateImage(ctx, input, permissionContext, observer)
	} else {
		result, artifacts, err = t.generateVideo(ctx, input, permissionContext, observer)
	}
	if err != nil {
		outcome := classifyFailure(err)
		if result.Status != "" {
			if encoded, encodeErr := json.Marshal(result); encodeErr == nil {
				outcome.Content = string(encoded)
			}
			outcome.Artifacts = artifacts
			outcome.AffectedPaths = append([]string(nil), result.Paths...)
			outcome.Details = generatedResultDetails(result)
			if len(artifacts) > 0 {
				outcome.TransportStatus = tool.TransportCompleted
				outcome.Retryable = false
				outcome.Summary = fmt.Sprintf("已保存 %d 个媒体文件，%d 个结果未完成", len(result.Paths), result.FailedCount)
				outcome.Operation = &tool.OperationDescriptor{Type: t.Name(), SideEffectState: tool.OutcomeSideEffectPartial}
			}
		}
		return outcome.WithTiming(started, time.Now().UTC()), nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return mediaFailure("生成结果无法编码。", "media_generate_result_invalid", tool.ErrorCategoryTool, false).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	outcome.Artifacts = artifacts
	outcome.AffectedPaths = append([]string(nil), result.Paths...)
	outcome.Summary = fmt.Sprintf("已生成 %d 个%s文件", len(result.Paths), result.Kind)
	outcome.Details = generatedResultDetails(result)
	return outcome.WithTiming(started, time.Now().UTC()), nil
}

func (t *mediaTool) validateInput(input string) error {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if t.configuration.ModelKind == "image" {
		var value imageInput
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("参数必须是有效的图片生成 JSON：%w", err)
		}
		if strings.TrimSpace(value.Prompt) == "" {
			return errors.New("prompt 不能为空")
		}
		if providerControlFlag.MatchString(value.Prompt) {
			return errors.New("prompt 不能包含供应商控制参数，请使用结构化工具字段")
		}
		if value.OutputFormat != "" && value.OutputFormat != "png" && value.OutputFormat != "jpeg" {
			return errors.New("output_format 只支持 png 或 jpeg")
		}
		switch value.Mode {
		case "", "single":
			if value.MaxImages != 0 {
				return errors.New("max_images 只能在 mode=group 时设置")
			}
		case "group":
			if value.MaxImages < 1 || value.MaxImages > 15 {
				return errors.New("mode=group 时 max_images 必须在 1 到 15 之间")
			}
		default:
			return errors.New("mode 只支持 single 或 group")
		}
		return nil
	}
	var value videoInput
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("参数必须是有效的视频生成 JSON：%w", err)
	}
	if strings.TrimSpace(value.Prompt) == "" {
		return errors.New("prompt 不能为空")
	}
	if providerControlFlag.MatchString(value.Prompt) {
		return errors.New("prompt 不能包含供应商控制参数，请使用结构化工具字段")
	}
	if value.Duration < 0 || value.Duration > 60 {
		return errors.New("duration 必须在 1 到 60 秒之间")
	}
	if value.SourceRole != "" && value.SourceRole != "first_frame" && value.SourceRole != "reference_image" {
		return errors.New("source_role 只支持 first_frame 或 reference_image")
	}
	if value.SourceRole != "" && strings.TrimSpace(value.SourceImagePath) == "" {
		return errors.New("source_role 只能与 source_image_path 一起设置")
	}
	if strings.TrimSpace(value.TaskID) != "" && strings.TrimSpace(value.SourceImagePath) != "" {
		return errors.New("续查 task_id 时不能重新设置 source_image_path")
	}
	return nil
}

func (t *mediaTool) generateImage(ctx context.Context, input string, permissionContext tool.PermissionContext, observer tool.ToolObserver) (generatedResult, []tool.ArtifactDescriptor, error) {
	var parameters imageInput
	_ = json.Unmarshal([]byte(input), &parameters)
	body := map[string]any{
		"model": t.configuration.ModelID, "prompt": strings.TrimSpace(parameters.Prompt), "response_format": "url",
	}
	resultLimit := 1
	if parameters.Mode == "group" {
		resultLimit = parameters.MaxImages
		body["sequential_image_generation"] = "auto"
		body["sequential_image_generation_options"] = map[string]any{
			"max_images": parameters.MaxImages,
		}
	} else {
		body["sequential_image_generation"] = "disabled"
	}
	if parameters.Size != "" {
		body["size"] = parameters.Size
	}
	if parameters.OutputFormat != "" {
		body["output_format"] = parameters.OutputFormat
	}
	if parameters.Watermark != nil {
		body["watermark"] = *parameters.Watermark
	}
	var response struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	generatingMessage := "正在生成图片"
	if parameters.Mode == "group" {
		generatingMessage = fmt.Sprintf("正在生成最多 %d 张图片", parameters.MaxImages)
	}
	emitMediaStatus(observer, "generating", generatingMessage)
	requestCtx, cancel := withOptionalTimeout(ctx, t.imageTimeout)
	defer cancel()
	if err := t.requestJSON(requestCtx, http.MethodPost, "/images/generations", body, &response); err != nil {
		return generatedResult{}, nil, err
	}
	if len(response.Data) == 0 {
		return generatedResult{}, nil, &providerError{code: "media_generate_upstream_failed", message: "图片生成服务没有返回结果。", retryable: true}
	}
	if len(response.Data) > resultLimit {
		return generatedResult{}, nil, &providerError{code: "media_generate_upstream_failed", message: "图片生成服务返回的结果数量超过本次请求上限。", retryable: false}
	}
	paths := make([]string, 0, len(response.Data))
	artifacts := make([]tool.ArtifactDescriptor, 0, len(response.Data))
	items := make([]generatedItem, 0, len(response.Data))
	failedCount := 0
	var firstFailure error
	emitMediaStatus(observer, "downloading", fmt.Sprintf("正在载入 %d 张图片", len(response.Data)))
	for index, item := range response.Data {
		data, contentType, err := t.resolveImageBytes(ctx, item.URL, item.B64JSON)
		if err != nil {
			failedCount++
			items = append(items, generatedItem{Index: index + 1, Status: "error"})
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		contentType, err = validateMediaBytes("image", data, contentType)
		if err != nil {
			failedCount++
			items = append(items, generatedItem{Index: index + 1, Status: "error"})
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		extension := extensionForContentType(contentType, ".jpg")
		_, displayPath, err := writeMediaFile(permissionContext, "image", extension, data)
		if err != nil {
			failedCount++
			items = append(items, generatedItem{Index: index + 1, Status: "error"})
			if firstFailure == nil {
				firstFailure = err
			}
			continue
		}
		paths = append(paths, displayPath)
		items = append(items, generatedItem{Index: index + 1, Status: "ready", Path: displayPath})
		artifacts = append(artifacts, mediaArtifact(t.Name(), "image", displayPath, index+1))
	}
	result := generatedResult{
		Status: "succeeded", Kind: "image", ModelID: t.configuration.ModelID,
		Paths: paths, Items: items, RequestedCount: resultLimit, FailedCount: failedCount, Provider: providerID,
	}
	if failedCount > 0 {
		result.Status = "partial"
		message := fmt.Sprintf("已保存 %d 张图片，%d 张载入失败。", len(paths), failedCount)
		emitMediaStatusDetails(observer, "partial", message, map[string]any{"items": items})
		return result, artifacts, &providerError{
			code: "media_generate_partial", message: message, retryable: false,
			transportStatus: tool.TransportCompleted, cause: firstFailure,
		}
	}
	emitMediaStatus(observer, "ready", fmt.Sprintf("已生成 %d 张图片", len(paths)))
	return result, artifacts, nil
}

func (t *mediaTool) generateVideo(ctx context.Context, input string, permissionContext tool.PermissionContext, observer tool.ToolObserver) (generatedResult, []tool.ArtifactDescriptor, error) {
	var parameters videoInput
	_ = json.Unmarshal([]byte(input), &parameters)
	taskID := strings.TrimSpace(parameters.TaskID)
	taskFingerprint := ""
	if taskID == "" {
		content := []map[string]any{{"type": "text", "text": strings.TrimSpace(parameters.Prompt)}}
		if strings.TrimSpace(parameters.SourceImagePath) != "" {
			imageURL, err := sourceImageDataURL(permissionContext, parameters.SourceImagePath)
			if err != nil {
				return generatedResult{}, nil, &providerError{code: "media_generate_invalid_input", message: err.Error(), retryable: false, transportStatus: tool.TransportNotStarted, cause: err}
			}
			role := strings.TrimSpace(parameters.SourceRole)
			if role == "" {
				role = "first_frame"
			}
			imagePart := map[string]any{
				"type": "image_url", "image_url": map[string]any{"url": imageURL},
			}
			// The official first-frame example omits role. Seedance only needs an
			// explicit role for reference-image semantics.
			if role == "reference_image" {
				imagePart["role"] = role
			}
			content = append(content, imagePart)
		}
		body := map[string]any{
			"model":   t.configuration.ModelID,
			"content": content,
		}
		if parameters.Ratio != "" {
			body["ratio"] = parameters.Ratio
		}
		if parameters.Resolution != "" {
			body["resolution"] = parameters.Resolution
		}
		if parameters.Duration > 0 {
			body["duration"] = parameters.Duration
		}
		if parameters.GenerateAudio != nil {
			body["generate_audio"] = *parameters.GenerateAudio
		}
		if parameters.Watermark != nil {
			body["watermark"] = *parameters.Watermark
		}
		taskFingerprint = t.videoTaskFingerprint(permissionContext, body)
		var err error
		taskID, err = t.resolveOrCreateVideoTask(ctx, taskFingerprint, body, observer)
		if err != nil {
			var provider *providerError
			if errors.As(err, &provider) && provider.taskID != "" {
				return pendingVideoResult(t.configuration.ModelID, provider.taskID, provider.taskStatus), nil, err
			}
			return generatedResult{}, nil, err
		}
	} else {
		emitMediaStatusDetails(observer, "queued", "正在续查已有视频任务", map[string]any{"task_id": taskID})
	}
	pollCtx, cancel := context.WithTimeout(ctx, t.pollTimeout)
	defer cancel()
	lastProviderStatus := ""
	for {
		var task struct {
			Status string `json:"status"`
			Error  struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Content struct {
				VideoURL string `json:"video_url"`
			} `json:"content"`
		}
		requestCtx, requestCancel := withOptionalTimeout(pollCtx, t.requestTimeout)
		err := t.requestJSON(requestCtx, http.MethodGet, "/contents/generations/tasks/"+url.PathEscape(taskID), nil, &task)
		requestCancel()
		if err != nil {
			if pollCtx.Err() != nil {
				_ = t.persistVideoTask(taskFingerprint, taskID, "unknown")
				return pendingVideoResult(t.configuration.ModelID, taskID, lastProviderStatus), nil, pendingVideoError(taskID, lastProviderStatus, pollCtx.Err())
			}
			return generatedResult{}, nil, err
		}
		providerStatus := strings.ToLower(strings.TrimSpace(task.Status))
		if providerStatus != "" && providerStatus != lastProviderStatus {
			lastProviderStatus = providerStatus
			_ = t.persistVideoTask(taskFingerprint, taskID, providerStatus)
			switch providerStatus {
			case "queued":
				emitMediaStatusDetails(observer, "queued", "视频任务正在排队", map[string]any{"task_id": taskID, "provider_status": providerStatus})
			case "running", "processing":
				emitMediaStatusDetails(observer, "generating", "正在生成视频", map[string]any{"task_id": taskID, "provider_status": providerStatus})
			}
		}
		switch providerStatus {
		case "succeeded":
			emitMediaStatus(observer, "downloading", "正在载入视频成品")
			data, contentType, err := t.download(ctx, task.Content.VideoURL)
			if err != nil {
				return generatedResult{}, nil, err
			}
			contentType, err = validateMediaBytes("video", data, contentType)
			if err != nil {
				return generatedResult{}, nil, err
			}
			extension := extensionForContentType(contentType, ".mp4")
			_, displayPath, err := writeMediaFile(permissionContext, "video", extension, data)
			if err != nil {
				return generatedResult{}, nil, err
			}
			artifact := mediaArtifact(t.Name(), "video", displayPath, 1)
			_ = t.persistVideoTask(taskFingerprint, taskID, "succeeded")
			emitMediaStatusDetails(observer, "ready", "视频已生成", map[string]any{"task_id": taskID, "provider_status": providerStatus})
			return generatedResult{Status: "succeeded", Kind: "video", ModelID: t.configuration.ModelID, TaskID: taskID, Paths: []string{displayPath}, Items: []generatedItem{{Index: 1, Status: "ready", Path: displayPath}}, RequestedCount: 1, Provider: providerID}, []tool.ArtifactDescriptor{artifact}, nil
		case "failed", "cancelled":
			_ = t.persistVideoTask(taskFingerprint, taskID, providerStatus)
			message := strings.TrimSpace(task.Error.Message)
			if message == "" {
				message = "视频生成任务未成功完成。"
			}
			return generatedResult{Status: "failed", Kind: "video", ModelID: t.configuration.ModelID, TaskID: taskID, Items: []generatedItem{{Index: 1, Status: "error"}}, RequestedCount: 1, FailedCount: 1, Provider: providerID}, nil, &providerError{code: "media_generate_upstream_failed", message: message, retryable: false, taskID: taskID, taskStatus: providerStatus}
		}
		select {
		case <-pollCtx.Done():
			_ = t.persistVideoTask(taskFingerprint, taskID, "unknown")
			return pendingVideoResult(t.configuration.ModelID, taskID, lastProviderStatus), nil, pendingVideoError(taskID, lastProviderStatus, pollCtx.Err())
		case <-time.After(t.pollInterval):
		}
	}
}

func emitMediaStatus(observer tool.ToolObserver, stage, message string) {
	emitMediaStatusDetails(observer, stage, message, nil)
}

func emitMediaStatusDetails(observer tool.ToolObserver, stage, message string, details map[string]any) {
	if observer == nil {
		return
	}
	payloadValue := map[string]any{"stage": stage, "message": message}
	for key, value := range details {
		payloadValue[key] = value
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		return
	}
	observer.OnCommandOutput(tool.CommandOutput{
		Stream: "media_status",
		Delta:  string(payload) + "\n",
	})
}

type providerError struct {
	code                              string
	message                           string
	retryable                         bool
	statusCode                        int
	transportStatus                   tool.TransportStatus
	transportFailureKind              string
	cause                             error
	taskID                            string
	taskStatus                        string
	externalStateConfirmationRequired bool
}

func (e *providerError) Error() string { return e.message }
func (e *providerError) Unwrap() error { return e.cause }

func (t *mediaTool) requestJSON(ctx context.Context, method, suffix string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(t.configuration.BaseURL, "/")+suffix, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+t.configuration.APIKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := t.client.Do(request)
	if err != nil {
		return mediaTransportError("media_generate_upstream_failed", method, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumJSONBytes+1))
	if err != nil {
		failure := mediaTransportError("media_generate_upstream_failed", method, err)
		failure.message = "图片视频生成服务连接在响应完成前中断，请稍后重试。"
		return failure
	}
	if len(raw) > maximumJSONBytes {
		return &providerError{code: "media_generate_upstream_failed", message: "图片视频生成服务响应过大。", retryable: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return providerHTTPError(response.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return &providerError{code: "media_generate_upstream_failed", message: "图片视频生成服务返回了无效响应。", retryable: true, cause: err}
	}
	return nil
}

func providerHTTPError(status int, raw []byte) error {
	message := "图片视频生成服务返回 HTTP " + strconv.Itoa(status) + "。"
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &payload) == nil {
		if value := strings.TrimSpace(payload.Error.Message); value != "" {
			message = value
		} else if value := strings.TrimSpace(payload.Message); value != "" {
			message = value
		}
	}
	code := "media_generate_upstream_failed"
	retryable := status == http.StatusTooManyRequests || status >= 500
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		code = "media_generate_authorization_failed"
		retryable = false
	} else if status == http.StatusTooManyRequests {
		code = "media_generate_rate_limited"
	}
	return &providerError{code: code, message: message, retryable: retryable, statusCode: status}
}

func (t *mediaTool) resolveImageBytes(ctx context.Context, remoteURL, encoded string) ([]byte, string, error) {
	if strings.TrimSpace(encoded) != "" {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) == 0 || len(data) > maximumMediaBytes {
			return nil, "", &providerError{code: "media_generate_download_failed", message: "图片结果的 Base64 数据无效。", retryable: true, cause: err}
		}
		return data, http.DetectContentType(data), nil
	}
	return t.download(ctx, remoteURL)
}

func (t *mediaTool) download(ctx context.Context, remoteURL string) ([]byte, string, error) {
	if !isAllowedRemoteURL(remoteURL) {
		return nil, "", &providerError{code: "media_generate_download_failed", message: "供应商返回了不安全的媒体地址。", retryable: true}
	}
	downloadCtx, cancel := withOptionalTimeout(ctx, t.downloadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := t.client.Do(request)
	if err != nil {
		failure := mediaTransportError("media_generate_download_failed", http.MethodGet, err)
		if failure.transportFailureKind == "unsafe_redirect" {
			failure.message = "生成媒体下载失败：供应商返回了不安全的跳转地址。"
		} else {
			failure.message = "生成媒体下载失败：连接中断，请检查网络后重试。"
		}
		return nil, "", failure
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", &providerError{code: "media_generate_download_failed", message: "生成媒体下载返回 HTTP " + strconv.Itoa(response.StatusCode) + "。", retryable: true, statusCode: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumMediaBytes+1))
	if err != nil || len(data) == 0 || len(data) > maximumMediaBytes {
		return nil, "", &providerError{code: "media_generate_download_failed", message: "生成媒体文件为空、过大或无法读取。", retryable: true, cause: err}
	}
	contentType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func isAllowedRemoteURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return net.ParseIP(host).IsLoopback()
}

func extensionForContentType(contentType, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "video/mp4":
		return ".mp4"
	default:
		return fallback
	}
}

func validateMediaBytes(kind string, data []byte, declaredContentType string) (string, error) {
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	switch kind {
	case "image":
		if detected == "image/png" || detected == "image/jpeg" {
			return detected, nil
		}
	case "video":
		if len(data) >= 12 && string(data[4:8]) == "ftyp" {
			return "video/mp4", nil
		}
	}
	return "", &providerError{
		code:      "media_generate_download_failed",
		message:   "生成媒体的实际文件格式与预期不符。",
		retryable: true,
		cause:     fmt.Errorf("unexpected %s media type: declared=%q detected=%q", kind, declaredContentType, detected),
	}
}

func writeMediaFile(permissionContext tool.PermissionContext, kind, extension string, data []byte) (string, string, error) {
	name := fmt.Sprintf("%s-%s%s", kind, uuid.NewString(), extension)
	requested := filepath.Join("output", "generated-media", name)
	resolved, err := permissionContext.ResolveOutputMutationPathChecked(requested)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return "", "", err
	}
	temporary := resolved + ".tmp-" + uuid.NewString()
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return "", "", err
	}
	defer os.Remove(temporary)
	if err := os.Rename(temporary, resolved); err != nil {
		return "", "", err
	}
	displayPath := filepath.ToSlash(requested)
	return resolved, displayPath, nil
}

func mediaArtifact(producer, kind, displayPath string, ordinal int) tool.ArtifactDescriptor {
	return tool.ArtifactDescriptor{
		ContractVersion: tool.ArtifactContractVersion, Role: tool.ArtifactRoleDeliverable,
		Producer: producer, Action: tool.ArtifactActionCreated,
		ID: producer + ":" + uuid.NewString(), Title: fmt.Sprintf("Generated %s %d", kind, ordinal),
		Description: "Generated media saved and verified locally", Kind: kind,
		Path: displayPath, Status: "ready",
	}
}

func generatedResultDetails(result generatedResult) map[string]any {
	return map[string]any{
		"generation_status": result.Status, "kind": result.Kind, "provider": result.Provider,
		"model_id": result.ModelID, "task_id": result.TaskID, "paths": result.Paths,
		"items": result.Items, "requested_count": result.RequestedCount, "failed_count": result.FailedCount,
	}
}

func (t *mediaTool) resolveOrCreateVideoTask(ctx context.Context, fingerprint string, body map[string]any, observer tool.ToolObserver) (string, error) {
	videoTaskSubmissionMu.Lock()
	defer videoTaskSubmissionMu.Unlock()

	if recovered, ok := t.loadPendingVideoTask(fingerprint); ok {
		emitMediaStatusDetails(observer, "queued", "正在恢复上次提交的视频任务", map[string]any{
			"task_id": recovered.TaskID, "provider_status": recovered.Status,
		})
		return recovered.TaskID, nil
	}

	var created struct {
		ID string `json:"id"`
	}
	emitMediaStatus(observer, "queued", "正在提交视频任务")
	requestCtx, cancel := withOptionalTimeout(ctx, t.videoCreateTimeout)
	err := t.requestJSON(requestCtx, http.MethodPost, "/contents/generations/tasks", body, &created)
	cancel()
	if err != nil {
		var provider *providerError
		if errors.As(err, &provider) && provider.transportStatus == tool.TransportUnknown {
			provider.code = "media_generate_create_unknown"
			provider.message = "视频任务提交请求超时，且供应商没有返回任务 ID；Awaken 无法确认任务是否创建，本轮不会自动重试。请先在火山控制台确认任务状态。"
			provider.retryable = false
			provider.externalStateConfirmationRequired = true
		}
		return "", err
	}
	taskID := strings.TrimSpace(created.ID)
	if taskID == "" {
		return "", &providerError{code: "media_generate_upstream_failed", message: "视频生成服务没有返回任务 ID。", retryable: true}
	}
	emitMediaStatusDetails(observer, "queued", "视频任务已提交", map[string]any{
		"task_id": taskID, "provider_status": "queued",
	})
	if err := t.persistVideoTask(fingerprint, taskID, "queued"); err != nil {
		return "", &providerError{
			code:      "media_generate_task_persistence_failed",
			message:   "视频任务已提交，但本地未能可靠保存任务 ID；请保留当前 task_id 续查，勿重新创建任务。",
			retryable: false, transportStatus: tool.TransportUnknown,
			transportFailureKind: "task_persistence", cause: err, taskID: taskID, taskStatus: "queued",
		}
	}
	return taskID, nil
}

func withOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func sourceImageDataURL(permissionContext tool.PermissionContext, displayPath string) (string, error) {
	resolved, err := permissionContext.ResolveReadablePathChecked(displayPath)
	if err != nil {
		return "", fmt.Errorf("无法读取 source_image_path：%w", err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", errors.New("source_image_path 必须指向可读取的 ready 图片 Artifact")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximumInputImageBytes+1))
	if err != nil || len(data) == 0 || len(data) > maximumInputImageBytes {
		return "", errors.New("source_image_path 图片为空、过大或无法读取")
	}
	contentType, err := validateMediaBytes("image", data, "")
	if err != nil {
		return "", errors.New("source_image_path 只支持 PNG 或 JPEG 图片")
	}
	if len(data) > maximumInlineVideoImageBytes {
		data, contentType, err = compactVideoInputImage(data)
		if err != nil {
			return "", err
		}
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func compactVideoInputImage(data []byte) ([]byte, string, error) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", errors.New("source_image_path 图片无法为视频输入重新编码")
	}
	bounds := decoded.Bounds()
	if bounds.Empty() {
		return nil, "", errors.New("source_image_path 图片尺寸无效")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), decoded, bounds.Min, draw.Over)

	for _, quality := range []int{88, 80, 72, 64} {
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", errors.New("source_image_path 图片无法为视频输入重新编码")
		}
		if encoded.Len() <= maximumInlineVideoImageBytes {
			return encoded.Bytes(), "image/jpeg", nil
		}
		if quality == 64 && encoded.Len() <= maximumVideoImageBytes {
			return encoded.Bytes(), "image/jpeg", nil
		}
	}
	return nil, "", errors.New("source_image_path 图片重新编码后仍过大，请先缩小图片再生成视频")
}

func (t *mediaTool) videoTaskFingerprint(permissionContext tool.PermissionContext, body map[string]any) string {
	encoded, _ := json.Marshal(body)
	digest := sha256.Sum256([]byte(strings.Join([]string{
		t.configuration.AccountID,
		t.configuration.ModelID,
		permissionContext.Normalize().WorkspaceRoot,
		string(encoded),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func videoTaskPath(fingerprint string) string {
	root := strings.TrimSpace(os.Getenv("AWAKEN_HOME"))
	if root == "" || len(fingerprint) != sha256.Size*2 {
		return ""
	}
	return filepath.Join(root, "state", "media-tasks", fingerprint+".json")
}

func (t *mediaTool) loadPendingVideoTask(fingerprint string) (videoTaskRecord, bool) {
	path := videoTaskPath(fingerprint)
	if path == "" {
		return videoTaskRecord{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return videoTaskRecord{}, false
	}
	var record videoTaskRecord
	if json.Unmarshal(raw, &record) != nil || record.Version != 1 || record.Fingerprint != fingerprint ||
		record.AccountID != t.configuration.AccountID || record.ModelID != t.configuration.ModelID ||
		strings.TrimSpace(record.TaskID) == "" || time.Since(record.UpdatedAt) > 7*24*time.Hour {
		return videoTaskRecord{}, false
	}
	switch strings.ToLower(strings.TrimSpace(record.Status)) {
	case "queued", "running", "processing", "unknown":
		return record, true
	default:
		return videoTaskRecord{}, false
	}
}

func (t *mediaTool) persistVideoTask(fingerprint, taskID, status string) error {
	path := videoTaskPath(fingerprint)
	if path == "" || strings.TrimSpace(taskID) == "" {
		return nil
	}
	now := time.Now().UTC()
	createdAt := now
	if raw, err := os.ReadFile(path); err == nil {
		var existing videoTaskRecord
		if json.Unmarshal(raw, &existing) == nil && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		}
	}
	record := videoTaskRecord{
		Version: 1, Fingerprint: fingerprint, TaskID: strings.TrimSpace(taskID),
		AccountID: t.configuration.AccountID, ModelID: t.configuration.ModelID,
		Status: strings.ToLower(strings.TrimSpace(status)), CreatedAt: createdAt, UpdatedAt: now,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp-" + uuid.NewString()
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return err
	}
	defer os.Remove(temporary)
	return os.Rename(temporary, path)
}

func pendingVideoResult(modelID, taskID, _ string) generatedResult {
	return generatedResult{
		Status: "unknown", Kind: "video", ModelID: modelID, TaskID: taskID,
		Items: []generatedItem{{Index: 1, Status: "unknown"}}, RequestedCount: 1, Provider: providerID,
	}
}

func pendingVideoError(taskID, providerStatus string, cause error) *providerError {
	message := "视频任务已提交，但本次运行未能确认最终状态；请使用原 task_id 续查，勿重新创建任务。"
	return &providerError{
		code: "media_generate_task_pending", message: message, retryable: false,
		transportStatus: tool.TransportUnknown, transportFailureKind: "poll_timeout",
		cause: cause, taskID: taskID, taskStatus: providerStatus,
	}
}

func classifyFailure(err error) tool.ExecutionOutcome {
	var provider *providerError
	if errors.As(err, &provider) {
		category := tool.ErrorCategoryRuntime
		status := tool.OutcomeFailed
		if provider.code == "media_generate_authorization_failed" {
			category = tool.ErrorCategoryAuthorization
			status = tool.OutcomeNeedsAuth
		}
		if provider.code == "media_generate_invalid_input" {
			category = tool.ErrorCategoryValidation
		}
		outcome := mediaFailure(provider.message, provider.code, category, provider.retryable)
		outcome.Status = status
		if provider.code == "media_generate_task_pending" || provider.code == "media_generate_task_persistence_failed" {
			outcome.SemanticStatus = tool.SemanticUnknown
			outcome.Operation = &tool.OperationDescriptor{Type: videoToolName, Target: provider.taskID, SideEffectState: tool.OutcomeSideEffectUnknown}
			outcome.RecoveryOptions = []string{"使用原 task_id 续查视频任务", "不要重复创建视频任务"}
		} else if provider.code == "media_generate_create_unknown" {
			outcome.SemanticStatus = tool.SemanticUnknown
			outcome.Operation = &tool.OperationDescriptor{Type: videoToolName, SideEffectState: tool.OutcomeSideEffectUnknown}
			outcome.RecoveryOptions = []string{"检查火山控制台中的视频任务", "确认没有任务后再由用户明确重新创建", "不要自动更换参数重试"}
		}
		if provider.transportStatus != "" {
			outcome.TransportStatus = provider.transportStatus
		}
		diagnostic := map[string]any{}
		if provider.statusCode != 0 {
			diagnostic["http_status"] = provider.statusCode
		}
		if provider.transportFailureKind != "" {
			diagnostic["transport_failure_kind"] = provider.transportFailureKind
		}
		if provider.taskID != "" {
			diagnostic["task_id"] = provider.taskID
		}
		if provider.taskStatus != "" {
			diagnostic["provider_status"] = provider.taskStatus
		}
		if provider.externalStateConfirmationRequired {
			diagnostic["requires_external_state_confirmation"] = true
		}
		if len(diagnostic) > 0 {
			outcome.Diagnostic = diagnostic
		}
		return outcome
	}
	return mediaFailure("图片视频生成工具执行失败。", "media_generate_upstream_failed", tool.ErrorCategoryTool, true)
}

func mediaTransportError(code, method string, err error) *providerError {
	kind := safeTransportFailureKind(err)
	message := "图片视频生成服务网络请求失败，请检查网络后重试。"
	switch kind {
	case "timeout":
		message = "图片视频生成服务响应超时，请稍后重试。"
	case "dns":
		message = "无法连接图片视频生成服务，请检查网络后重试。"
	case "connection_closed", "network_read", "network_write":
		message = "图片视频生成服务连接中断，请稍后重试。"
	case "cancelled":
		message = "图片视频生成请求已停止。"
	case "unsafe_redirect":
		message = "图片视频生成服务返回了不安全的跳转地址。"
	}
	status := tool.TransportFailed
	if kind == "cancelled" {
		status = tool.TransportCancelled
	} else if kind == "timeout" && method != http.MethodGet {
		// A timed-out create request may already have consumed quota upstream.
		// Treat it as unknown and block blind replay.
		status = tool.TransportUnknown
	}
	retryable := kind != "cancelled" && kind != "unsafe_redirect"
	if status == tool.TransportUnknown {
		retryable = false
	}
	return &providerError{
		code: code, message: message, retryable: retryable,
		transportStatus: status, transportFailureKind: kind, cause: err,
	}
}

func safeTransportFailureKind(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, errUnsafeMediaRedirect) {
		return "unsafe_redirect"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return "dns"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "timeout"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return "connection_closed"
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) {
		switch strings.ToLower(strings.TrimSpace(operationError.Op)) {
		case "dial":
			return "network_dial"
		case "read":
			return "network_read"
		case "write":
			return "network_write"
		}
	}
	return "network"
}

func mediaFailure(message, code string, category tool.ErrorCategory, retryable bool) tool.ExecutionOutcome {
	outcome := tool.FailedOutcome(message, code, category, retryable)
	outcome.RecoveryOptions = []string{"检查 Agent Plan 模型与工具配置", "修正参数后重试"}
	return outcome
}

func init() {
	for _, configuration := range loadRuntimeConfigurations() {
		tool.MustRegisterToolWithMode(newMediaTool(configuration), tool.DefinitionValidationStrictBuiltin)
	}
}
