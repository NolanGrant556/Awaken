package skillruntime

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"awaken/src/internal/outputlimit"
	"awaken/src/skills"
	"awaken/src/tool"
)

var (
	safeGitRefPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	httpsURLTextPattern = regexp.MustCompile(`https://[^\s'"<>]+`)
)

func cloneRemoteGitSkillPackage(ctx context.Context, req skillInstallInput) (skills.SkillPackage, error) {
	if err := validateRemoteGitRequest(req); err != nil {
		return skills.SkillPackage{}, invalidSkillInstallSource(err)
	}
	rawURL, _ := cleanRemoteGitURL(req.URL)
	ref, _ := cleanGitRef(req.Ref)
	cleanSubpath, _ := cleanGitSubpath(req.Subpath)
	tmpDir, err := os.MkdirTemp("", "awaken-skill-git-*")
	if err != nil {
		return skills.SkillPackage{}, unavailableSkillInstallSource(fmt.Errorf("create temporary git directory: %w", err))
	}
	defer os.RemoveAll(tmpDir)
	templateDir := filepath.Join(tmpDir, "empty-template")
	if err := os.Mkdir(templateDir, 0o700); err != nil {
		return skills.SkillPackage{}, unavailableSkillInstallSource(fmt.Errorf("create safe Git template directory: %w", err))
	}

	repoDir := filepath.Join(tmpDir, "repo")
	if err := runGit(ctx, "clone", "--depth", "1", "--template", templateDir, "--", rawURL, repoDir); err != nil {
		return skills.SkillPackage{}, unavailableSkillInstallSource(err)
	}
	if ref != "" {
		if err := runGit(ctx, "-C", repoDir, "fetch", "--depth", "1", "--", "origin", ref); err != nil {
			return skills.SkillPackage{}, unavailableSkillInstallSource(err)
		}
		if err := runGit(ctx, "-C", repoDir, "checkout", "--detach", "FETCH_HEAD"); err != nil {
			return skills.SkillPackage{}, unavailableSkillInstallSource(err)
		}
	}

	sourceDir := repoDir
	if cleanSubpath != "" {
		sourceDir = filepath.Join(repoDir, filepath.FromSlash(cleanSubpath))
		if !tool.PathWithinRoot(sourceDir, repoDir) {
			return skills.SkillPackage{}, invalidSkillInstallSource(fmt.Errorf("subpath escapes the repository"))
		}
	}
	pkg, err := skills.PackageFromPath(sourceDir)
	if err != nil {
		return skills.SkillPackage{}, invalidSkillInstallSource(err)
	}
	return pkg, nil
}

func runGit(ctx context.Context, args ...string) error {
	safeArgs := []string{
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.file.allow=never",
		"-c", "protocol.ssh.allow=never",
		"-c", "protocol.git.allow=never",
		"-c", "protocol.http.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "credential.helper=",
	}
	safeArgs = append(safeArgs, args...)
	cmd := exec.CommandContext(ctx, "git", safeArgs...)
	cmd.Env = safeGitEnvironment()
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(output))
	msg = httpsURLTextPattern.ReplaceAllString(msg, "<redacted-https-url>")
	if prefix, truncated := outputlimit.Prefix(msg, 2000); truncated {
		msg = prefix + "...(truncated)"
	} else {
		msg = prefix
	}
	if msg == "" {
		return fmt.Errorf("git operation failed: %w", err)
	}
	return fmt.Errorf("git operation failed: %w: %s", err, msg)
}

func cleanRemoteGitURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse remote Git URL: %w", err)
	}
	if parsed.Scheme != "https" || strings.TrimSpace(parsed.Host) == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("remote_git only supports absolute https repository URLs")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("remote_git URL must not contain inline credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("remote_git URL must not contain a query string or fragment")
	}
	return parsed.String(), nil
}

func validateRemoteGitRequest(req skillInstallInput) error {
	if _, err := cleanRemoteGitURL(req.URL); err != nil {
		return err
	}
	if _, err := cleanGitRef(req.Ref); err != nil {
		return err
	}
	if _, err := cleanGitSubpath(req.Subpath); err != nil {
		return err
	}
	return nil
}

func cleanGitRef(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if !safeGitRefPattern.MatchString(value) || strings.Contains(value, "..") || strings.Contains(value, "@{") || strings.Contains(value, "//") || strings.HasSuffix(value, "/") || strings.HasSuffix(value, ".") || strings.HasSuffix(value, ".lock") {
		return "", fmt.Errorf("ref must be a branch, tag, or commit using safe Git ref characters")
	}
	return value, nil
}

func safeGitEnvironment() []string {
	allowed := map[string]bool{
		"PATH": true, "TMPDIR": true, "TMP": true, "TEMP": true,
		"LANG": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true,
		"HTTPS_PROXY": true, "https_proxy": true, "NO_PROXY": true, "no_proxy": true,
	}
	env := make([]string, 0, 20)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if allowed[key] || strings.HasPrefix(key, "LC_") {
			env = append(env, entry)
		}
	}
	return append(env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_ALLOW_PROTOCOL=https",
	)
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
