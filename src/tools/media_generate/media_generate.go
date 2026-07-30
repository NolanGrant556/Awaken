package mediagenerate

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"awaken/src/tool"

	"github.com/google/uuid"
)

const (
	imageToolName     = "image_generate"
	videoToolName     = "video_generate"
	maximumJSONBytes  = 4 * 1024 * 1024
	maximumMediaBytes = 128 * 1024 * 1024
)

var providerControlFlag = regexp.MustCompile(`(?i)(?:^|\s)--(?:ratio|dur|duration|resolution|generate_audio|watermark|model)(?:\s|=|$)`)

type mediaTool struct {
	configuration runtimeConfiguration
	client        *http.Client
	pollInterval  time.Duration
	pollTimeout   time.Duration
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
	Prompt        string `json:"prompt"`
	Ratio         string `json:"ratio,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	GenerateAudio *bool  `json:"generate_audio,omitempty"`
	Watermark     *bool  `json:"watermark,omitempty"`
}

type generatedResult struct {
	Status   string   `json:"status"`
	Kind     string   `json:"kind"`
	ModelID  string   `json:"model_id"`
	TaskID   string   `json:"task_id,omitempty"`
	Paths    []string `json:"paths"`
	Provider string   `json:"provider"`
}

func newMediaTool(configuration runtimeConfiguration) *mediaTool {
	return &mediaTool{
		configuration: configuration,
		client: &http.Client{
			Timeout: 2 * time.Minute,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many media redirects")
				}
				if !isAllowedRemoteURL(request.URL.String()) {
					return errors.New("unsafe media redirect")
				}
				return nil
			},
		},
		pollInterval: 3 * time.Second,
		pollTimeout:  15 * time.Minute,
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
	return "根据用户目标创建视频生成任务，等待真实终态并保存为本地 MP4 交付物；生成模型已由设置固定"
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
			"kind":     {Type: "string", Description: "生成的媒体类型"},
			"model_id": {Type: "string", Description: "Host 配置固定的实际生成模型"},
			"task_id":  {Type: "string", Description: "异步视频任务 ID（图片为空）"},
			"paths":    {Type: "array", Description: "已验证并保存的本地媒体路径", Items: &tool.ToolParam{Type: "string", Description: "本地媒体路径"}},
			"provider": {Type: "string", Description: "实际执行生成任务的供应商"},
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
			{Code: "media_generate_download_failed", Cause: "结果文件无法验证或下载", Retryable: true, Recovery: []string{"重试生成或检查网络"}},
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
		return classifyFailure(err).WithTiming(started, time.Now().UTC()), nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return mediaFailure("生成结果无法编码。", "media_generate_result_invalid", tool.ErrorCategoryTool, false).WithTiming(started, time.Now().UTC()), nil
	}
	outcome := tool.SucceededOutcome(string(encoded))
	outcome.Artifacts = artifacts
	outcome.AffectedPaths = append([]string(nil), result.Paths...)
	outcome.Summary = fmt.Sprintf("已生成 %d 个%s文件", len(result.Paths), result.Kind)
	outcome.Details = map[string]any{
		"kind": result.Kind, "provider": result.Provider, "model_id": result.ModelID,
		"task_id": result.TaskID,
		"paths":   result.Paths,
	}
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
	if err := t.requestJSON(ctx, http.MethodPost, "/images/generations", body, &response); err != nil {
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
	writtenPaths := make([]string, 0, len(response.Data))
	cleanupWritten := func() {
		for _, writtenPath := range writtenPaths {
			_ = os.Remove(writtenPath)
		}
	}
	emitMediaStatus(observer, "downloading", fmt.Sprintf("正在载入 %d 张图片", len(response.Data)))
	for index, item := range response.Data {
		data, contentType, err := t.resolveImageBytes(ctx, item.URL, item.B64JSON)
		if err != nil {
			cleanupWritten()
			return generatedResult{}, nil, err
		}
		contentType, err = validateMediaBytes("image", data, contentType)
		if err != nil {
			cleanupWritten()
			return generatedResult{}, nil, err
		}
		extension := extensionForContentType(contentType, ".jpg")
		resolvedPath, displayPath, err := writeMediaFile(permissionContext, "image", extension, data)
		if err != nil {
			cleanupWritten()
			return generatedResult{}, nil, err
		}
		writtenPaths = append(writtenPaths, resolvedPath)
		paths = append(paths, displayPath)
		artifacts = append(artifacts, mediaArtifact(t.Name(), "image", displayPath, index+1))
	}
	emitMediaStatus(observer, "ready", fmt.Sprintf("已生成 %d 张图片", len(paths)))
	return generatedResult{Status: "succeeded", Kind: "image", ModelID: t.configuration.ModelID, Paths: paths, Provider: providerID}, artifacts, nil
}

func (t *mediaTool) generateVideo(ctx context.Context, input string, permissionContext tool.PermissionContext, observer tool.ToolObserver) (generatedResult, []tool.ArtifactDescriptor, error) {
	var parameters videoInput
	_ = json.Unmarshal([]byte(input), &parameters)
	body := map[string]any{
		"model":   t.configuration.ModelID,
		"content": []map[string]any{{"type": "text", "text": strings.TrimSpace(parameters.Prompt)}},
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
	var created struct {
		ID string `json:"id"`
	}
	if err := t.requestJSON(ctx, http.MethodPost, "/contents/generations/tasks", body, &created); err != nil {
		return generatedResult{}, nil, err
	}
	created.ID = strings.TrimSpace(created.ID)
	if created.ID == "" {
		return generatedResult{}, nil, &providerError{code: "media_generate_upstream_failed", message: "视频生成服务没有返回任务 ID。", retryable: true}
	}
	emitMediaStatus(observer, "queued", "视频任务已提交")
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
		if err := t.requestJSON(pollCtx, http.MethodGet, "/contents/generations/tasks/"+url.PathEscape(created.ID), nil, &task); err != nil {
			return generatedResult{}, nil, err
		}
		providerStatus := strings.ToLower(strings.TrimSpace(task.Status))
		if providerStatus != "" && providerStatus != lastProviderStatus {
			lastProviderStatus = providerStatus
			switch providerStatus {
			case "queued":
				emitMediaStatus(observer, "queued", "视频任务正在排队")
			case "running", "processing":
				emitMediaStatus(observer, "generating", "正在生成视频")
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
			emitMediaStatus(observer, "ready", "视频已生成")
			return generatedResult{Status: "succeeded", Kind: "video", ModelID: t.configuration.ModelID, TaskID: created.ID, Paths: []string{displayPath}, Provider: providerID}, []tool.ArtifactDescriptor{artifact}, nil
		case "failed", "cancelled":
			message := strings.TrimSpace(task.Error.Message)
			if message == "" {
				message = "视频生成任务未成功完成。"
			}
			return generatedResult{}, nil, &providerError{code: "media_generate_upstream_failed", message: message, retryable: task.Status == "failed"}
		}
		select {
		case <-pollCtx.Done():
			return generatedResult{}, nil, &providerError{code: "media_generate_upstream_failed", message: "视频生成任务等待已中断或超时。", retryable: true, cause: pollCtx.Err()}
		case <-time.After(t.pollInterval):
		}
	}
}

func emitMediaStatus(observer tool.ToolObserver, stage, message string) {
	if observer == nil {
		return
	}
	payload, err := json.Marshal(map[string]string{"stage": stage, "message": message})
	if err != nil {
		return
	}
	observer.OnCommandOutput(tool.CommandOutput{
		Stream: "media_status",
		Delta:  string(payload) + "\n",
	})
}

type providerError struct {
	code       string
	message    string
	retryable  bool
	statusCode int
	cause      error
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
		return &providerError{code: "media_generate_upstream_failed", message: "图片视频生成服务请求失败。", retryable: true, cause: err}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumJSONBytes+1))
	if err != nil {
		return &providerError{code: "media_generate_upstream_failed", message: "无法读取图片视频生成服务响应。", retryable: true, cause: err}
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := t.client.Do(request)
	if err != nil {
		return nil, "", &providerError{code: "media_generate_download_failed", message: "生成媒体下载失败。", retryable: true, cause: err}
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

func classifyFailure(err error) tool.ExecutionOutcome {
	var provider *providerError
	if errors.As(err, &provider) {
		category := tool.ErrorCategoryRuntime
		status := tool.OutcomeFailed
		if provider.code == "media_generate_authorization_failed" {
			category = tool.ErrorCategoryAuthorization
			status = tool.OutcomeNeedsAuth
		}
		outcome := mediaFailure(provider.message, provider.code, category, provider.retryable)
		outcome.Status = status
		if provider.statusCode != 0 {
			outcome.Diagnostic = map[string]any{"http_status": provider.statusCode}
		}
		return outcome
	}
	return mediaFailure("图片视频生成工具执行失败。", "media_generate_upstream_failed", tool.ErrorCategoryTool, true)
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
