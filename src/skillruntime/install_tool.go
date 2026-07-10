package skillruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/skills"
	"awaken/src/tool"
)

const SkillInstallToolName = "skill_install"

type SkillInstallTool struct {
	registry *skills.SkillRegistry
}

type skillInstallInput struct {
	SourceType         string `json:"source_type"`
	FileName           string `json:"file_name,omitempty"`
	Content            string `json:"content,omitempty"`
	ContentBase64      string `json:"content_base64,omitempty"`
	Path               string `json:"path,omitempty"`
	URL                string `json:"url,omitempty"`
	Ref                string `json:"ref,omitempty"`
	Subpath            string `json:"subpath,omitempty"`
	Scope              string `json:"scope,omitempty"`
	ProjectRoot        string `json:"project_root,omitempty"`
	OverwritePolicy    string `json:"overwrite_policy,omitempty"`
	ReloadAfterInstall *bool  `json:"reload_after_install,omitempty"`
}

type skillInstallOutput struct {
	Status        string `json:"status"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	SourceOrigin  string `json:"source_origin"`
	RootDir       string `json:"root_dir"`
	SkillFilePath string `json:"skill_file_path"`
	ProjectPath   string `json:"project_path,omitempty"`
	State         string `json:"state"`
	Reloaded      bool   `json:"reloaded"`
	Message       string `json:"message"`
}

func (t *SkillInstallTool) Name() string { return SkillInstallToolName }

func (t *SkillInstallTool) Description() string {
	return "Install a validated Awaken Skill package from content, a local path, an HTTP(S) archive/file, or a remote Git repository. The tool resolves project/global skill roots deterministically; do not guess or write .awaken/skills paths manually."
}

func (t *SkillInstallTool) Parameters() []tool.ToolParameter {
	return []tool.ToolParameter{
		{Name: "source_type", Type: "string", Required: true, Description: "Skill source adapter: content, content_base64, local_path, remote_url, or remote_git."},
		{Name: "file_name", Type: "string", Required: false, Description: "Source file name for content/content_base64/remote_url when needed; use SKILL.md or a .zip file name."},
		{Name: "content", Type: "string", Required: false, Description: "Raw SKILL.md or zip bytes represented as text when source_type is content."},
		{Name: "content_base64", Type: "string", Required: false, Description: "Base64 encoded SKILL.md or zip bytes when source_type is content_base64."},
		{Name: "path", Type: "string", Required: false, Description: "Local file or directory path when source_type is local_path. Directories must contain exactly one SKILL.md unless the root has SKILL.md."},
		{Name: "url", Type: "string", Required: false, Description: "HTTP(S) URL for remote_url, or remote Git repository URL for remote_git."},
		{Name: "ref", Type: "string", Required: false, Description: "Optional branch, tag, or commit-ish for remote_git."},
		{Name: "subpath", Type: "string", Required: false, Description: "Optional relative subdirectory within a remote_git repository that contains the skill package."},
		{Name: "scope", Type: "string", Required: false, Description: "Install scope: project (default) or global. user is accepted as a global alias."},
		{Name: "project_root", Type: "string", Required: false, Description: "Project root for project scope; defaults to the current workspace and is resolved by the runtime."},
		{Name: "overwrite_policy", Type: "string", Required: false, Description: "Existing skill policy: abort (default) or replace."},
		{Name: "reload_after_install", Type: "boolean", Required: false, Description: "Reload the skill registry after installation. Defaults to true."},
	}
}

func (t *SkillInstallTool) Definition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        SkillInstallToolName,
		DisplayName: "Skill Install",
		Source:      tool.SourceBuiltin,
		Description: t.Description(),
		Annotations: tool.ToolAnnotations{
			ReadOnly:        false,
			Destructive:     false,
			OpenWorld:       true,
			CapabilityClass: tool.CapabilityArtifactWrite,
			TrustedOrigin:   tool.TrustedOriginBuiltin,
			FileBridgeScope: tool.FileBridgeTaskScopeWrite,
			NetworkScope:    tool.NetworkScopeOpen,
		},
	}
}

func (t *SkillInstallTool) CheckPermission(ctx context.Context, input string, permissionContext tool.PermissionContext) tool.PermissionDecision {
	if err := ctx.Err(); err != nil {
		return tool.DenyPermission(err.Error())
	}
	req, err := parseSkillInstallInput(input)
	if err != nil {
		return tool.DenyPermission(fmt.Sprintf("invalid parameters: %v", err))
	}
	target, err := resolveSkillInstallTargetForTool(req, permissionContext)
	if err != nil {
		return tool.DenyPermission(err.Error())
	}
	if strings.TrimSpace(req.Path) != "" {
		if _, err := permissionContext.ResolveReadablePathChecked(req.Path); err != nil {
			return tool.DenyPermission(err.Error())
		}
	}
	return tool.AllowPermissionWithMetadata(tool.DecisionMetadata{
		Title:         "Install skill",
		Description:   fmt.Sprintf("Install skill into %s scope", target.scope),
		RiskTone:      "warning",
		AffectedPaths: []string{target.installDir},
	})
}

func (t *SkillInstallTool) Execute(ctx context.Context, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if t.registry == nil {
		return "", fmt.Errorf("skill registry unavailable")
	}
	req, err := parseSkillInstallInput(input)
	if err != nil {
		return "", err
	}
	target, err := resolveSkillInstallTargetForTool(req, tool.PermissionContextFromContext(ctx))
	if err != nil {
		return "", err
	}
	pkg, err := t.packageFromInput(ctx, req)
	if err != nil {
		return "", err
	}
	result, err := skills.InstallSkillPackage(skills.InstallRequest{
		Package:          pkg,
		Scope:            target.scope,
		ProjectRoot:      target.projectRoot,
		DefaultWorkspace: target.defaultWorkspace,
		OverwritePolicy:  req.OverwritePolicy,
	})
	if err != nil {
		if errors.Is(err, skills.ErrSkillAlreadyExists) {
			return "", fmt.Errorf("skill already exists; set overwrite_policy to replace to replace it: %w", err)
		}
		return "", err
	}

	reload := true
	if req.ReloadAfterInstall != nil {
		reload = *req.ReloadAfterInstall
	}
	if reload {
		t.registry.ReloadLocalSources(skills.LocalSourceDirs(target.projectRoot))
	}

	out := skillInstallOutput{
		Status:        "installed",
		ID:            result.Skill.ID,
		Name:          result.Skill.Name,
		Scope:         result.Scope,
		SourceOrigin:  string(result.Skill.Source),
		RootDir:       result.Skill.RootDir,
		SkillFilePath: result.Skill.SkillFilePath,
		ProjectPath:   result.Skill.ProjectPath,
		State:         string(result.Skill.State),
		Reloaded:      reload,
		Message:       result.Message,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *SkillInstallTool) packageFromInput(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	switch normalizeSourceType(req.SourceType) {
	case "content":
		if req.Content == "" {
			return skills.SkillPackage{}, fmt.Errorf("content is required")
		}
		return skills.PackageFromBytes(defaultSkillFileName(req.FileName), []byte(req.Content))
	case "content_base64":
		if strings.TrimSpace(req.ContentBase64) == "" {
			return skills.SkillPackage{}, fmt.Errorf("content_base64 is required")
		}
		data, err := base64.StdEncoding.DecodeString(req.ContentBase64)
		if err != nil {
			return skills.SkillPackage{}, fmt.Errorf("decode content_base64: %w", err)
		}
		return skills.PackageFromBytes(defaultSkillFileName(req.FileName), data)
	case "local_path":
		if strings.TrimSpace(req.Path) == "" {
			return skills.SkillPackage{}, fmt.Errorf("path is required")
		}
		resolved, err := tool.PermissionContextFromContext(ctx).ResolveReadablePathChecked(req.Path)
		if err != nil {
			return skills.SkillPackage{}, err
		}
		return skills.PackageFromPath(resolved)
	case "remote_url":
		return fetchRemoteSkillPackage(ctx, req)
	case "remote_git":
		return cloneRemoteGitSkillPackage(ctx, req)
	default:
		return skills.SkillPackage{}, fmt.Errorf("unsupported source_type %q", req.SourceType)
	}
}

type skillInstallTarget struct {
	scope            string
	projectRoot      string
	defaultWorkspace string
	installDir       string
}

func resolveSkillInstallTargetForTool(req skillInstallInput, permissionContext tool.PermissionContext) (skillInstallTarget, error) {
	normalized := permissionContext.Normalize()
	defaultWorkspace := normalized.WorkspaceRootOrCWD()
	projectRoot := strings.TrimSpace(req.ProjectRoot)
	if projectRoot != "" {
		projectRoot = normalized.ResolvePath(projectRoot)
		if !normalized.IsWithinWorkspace(projectRoot) {
			return skillInstallTarget{}, fmt.Errorf("project_root must be inside the current workspace")
		}
	}
	scope := normalizeSkillInstallScope(req.Scope)
	resolvedScope, resolvedProjectRoot, installDir, _, err := skills.ResolveInstallTarget(skills.InstallTargetRequest{
		Scope:            scope,
		ProjectRoot:      projectRoot,
		DefaultWorkspace: defaultWorkspace,
	})
	if err != nil {
		return skillInstallTarget{}, err
	}
	return skillInstallTarget{
		scope:            resolvedScope,
		projectRoot:      resolvedProjectRoot,
		defaultWorkspace: defaultWorkspace,
		installDir:       installDir,
	}, nil
}

func parseSkillInstallInput(input string) (skillInstallInput, error) {
	var req skillInstallInput
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &req); err != nil {
		return req, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if normalizeSourceType(req.SourceType) == "" {
		return req, fmt.Errorf("source_type is required")
	}
	return req, nil
}

func normalizeSourceType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeSkillInstallScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", string(skills.SourceProject):
		return string(skills.SourceProject)
	case "global", string(skills.SourceUser):
		return "global"
	default:
		return strings.TrimSpace(raw)
	}
}

func defaultSkillFileName(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "SKILL.md"
	}
	return strings.TrimSpace(raw)
}

func fetchRemoteSkillPackage(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		return skills.SkillPackage{}, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return skills.SkillPackage{}, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return skills.SkillPackage{}, fmt.Errorf("remote_url only supports http or https URLs")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return skills.SkillPackage{}, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return skills.SkillPackage{}, fmt.Errorf("download skill package: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return skills.SkillPackage{}, fmt.Errorf("download skill package: unexpected HTTP status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, skills.MaxSkillPackageBytes+1))
	if err != nil {
		return skills.SkillPackage{}, fmt.Errorf("read remote package: %w", err)
	}
	if len(data) > skills.MaxSkillPackageBytes {
		return skills.SkillPackage{}, fmt.Errorf("remote package exceeds %d bytes", skills.MaxSkillPackageBytes)
	}
	fileName := defaultRemoteFileName(req.FileName, parsed.Path)
	return skills.PackageFromBytes(fileName, data)
}

func cloneRemoteGitSkillPackage(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		return skills.SkillPackage{}, fmt.Errorf("url is required")
	}
	tmpDir, err := os.MkdirTemp("", "awaken-skill-git-*")
	if err != nil {
		return skills.SkillPackage{}, fmt.Errorf("create temporary git directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	if err := runGit(ctx, "clone", "--depth", "1", "--", rawURL, repoDir); err != nil {
		return skills.SkillPackage{}, err
	}
	if ref := strings.TrimSpace(req.Ref); ref != "" {
		if err := runGit(ctx, "-C", repoDir, "fetch", "--depth", "1", "origin", ref); err != nil {
			return skills.SkillPackage{}, err
		}
		if err := runGit(ctx, "-C", repoDir, "checkout", "--detach", "FETCH_HEAD"); err != nil {
			return skills.SkillPackage{}, err
		}
	}

	sourceDir := repoDir
	if strings.TrimSpace(req.Subpath) != "" {
		cleanSubpath, err := cleanGitSubpath(req.Subpath)
		if err != nil {
			return skills.SkillPackage{}, err
		}
		sourceDir = filepath.Join(repoDir, filepath.FromSlash(cleanSubpath))
		if !tool.PathWithinRoot(sourceDir, repoDir) {
			return skills.SkillPackage{}, fmt.Errorf("subpath escapes the repository")
		}
	}
	return skills.PackageFromPath(sourceDir)
}

func runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	if prefix, truncated := outputlimit.Prefix(msg, 2000); truncated {
		msg = prefix + "...(truncated)"
	} else {
		msg = prefix
	}
	if msg == "" {
		return fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, msg)
}

func defaultRemoteFileName(rawFileName string, rawPath string) string {
	if strings.TrimSpace(rawFileName) != "" {
		return strings.TrimSpace(rawFileName)
	}
	base := path.Base(rawPath)
	if base == "." || base == "/" || strings.TrimSpace(base) == "" {
		return "SKILL.md"
	}
	return base
}

func cleanGitSubpath(raw string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if normalized == "" {
		return "", nil
	}
	if path.IsAbs(normalized) {
		return "", fmt.Errorf("subpath must be relative")
	}
	clean := path.Clean(normalized)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("subpath escapes the repository")
	}
	return clean, nil
}
