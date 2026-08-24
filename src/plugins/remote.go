package plugins

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

var (
	safeGitRefPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	httpsURLTextPattern = regexp.MustCompile(`https://[^\s'"<>]+`)
)

func materializeRemoteURL(ctx context.Context, rawURL, staging string) error {
	if err := validateHTTPSURL(rawURL, "remote_url"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(rawURL), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download plugin archive: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download plugin archive: unexpected HTTP status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxDownloadBytes+1))
	if err != nil {
		return fmt.Errorf("read plugin archive: %w", err)
	}
	if int64(len(data)) > MaxDownloadBytes {
		return fmt.Errorf("plugin archive exceeds %d bytes", MaxDownloadBytes)
	}
	return extractZip(data, staging)
}

func materializeRemoteGit(ctx context.Context, source Source, staging string) error {
	rawURL, err := cleanRemoteGitURL(source.URL)
	if err != nil {
		return err
	}
	ref, err := cleanGitRef(source.Ref)
	if err != nil {
		return err
	}
	subpath, err := cleanGitSubpath(source.Subpath)
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "awaken-plugin-git-*")
	if err != nil {
		return fmt.Errorf("create temporary git directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	templateDir := filepath.Join(tmpDir, "empty-template")
	if err := os.Mkdir(templateDir, 0o700); err != nil {
		return fmt.Errorf("create safe Git template directory: %w", err)
	}
	repoDir := filepath.Join(tmpDir, "repo")
	if err := runGit(ctx, "clone", "--depth", "1", "--template", templateDir, "--", rawURL, repoDir); err != nil {
		return err
	}
	if ref != "" {
		if err := runGit(ctx, "-C", repoDir, "fetch", "--depth", "1", "--", "origin", ref); err != nil {
			return err
		}
		if err := runGit(ctx, "-C", repoDir, "checkout", "--detach", "FETCH_HEAD"); err != nil {
			return err
		}
	}
	sourceDir := repoDir
	if subpath != "" {
		sourceDir = filepath.Join(repoDir, filepath.FromSlash(subpath))
		if !tool.PathWithinRoot(sourceDir, repoDir) {
			return fmt.Errorf("subpath escapes the repository")
		}
	}
	return copyTree(sourceDir, staging)
}

func validateHTTPSURL(raw, field string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", field, err)
	}
	if parsed.Scheme != "https" || strings.TrimSpace(parsed.Host) == "" || parsed.Opaque != "" {
		return fmt.Errorf("%s only supports absolute https URLs", field)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s URL must not contain inline credentials", field)
	}
	return nil
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
