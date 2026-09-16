package managedenv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This opt-in check exercises the actual installer on locally built immutable
// archives. It is auxiliary evidence, not real-LLM Desktop acceptance or proof
// that unpublished release URLs can already be downloaded by users.
func TestNativeArchivePreparation(t *testing.T) {
	assets := os.Getenv("AWAKEN_NATIVE_ASSET_DIRECTORY")
	root := os.Getenv("AWAKEN_NATIVE_TEST_ROOT")
	if assets == "" || root == "" {
		t.Skip("set explicit native assets and isolated test root")
	}
	catalog, err := BuiltinCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for name, component := range catalog.Components {
		asset, ok := component.Assets[Target()]
		if ok && strings.Contains(asset.URL, "/Awaken-Releases/") {
			asset.URL = "file://" + filepath.ToSlash(filepath.Join(assets, filepath.Base(asset.URL)))
			component.Assets[Target()] = asset
			catalog.Components[name] = component
		}
	}
	m := managerForTest(t, Options{Root: root, Catalog: catalog, AllowFileAssets: true})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	owner := testOwner("原生资源路径验证")
	capabilities := []string{"poppler", "git"}
	if value := os.Getenv("AWAKEN_NATIVE_TEST_COMPONENTS"); value != "" {
		capabilities = strings.Split(value, ",")
	}
	for _, capability := range capabilities {
		binding, err := m.Ensure(ctx, Request{Capability: capability}, owner, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(capability, binding.InstanceID, binding.Executables)
		if capability == "ffmpeg" {
			file := filepath.Join(root, "一秒 音频.wav")
			if _, err = m.runWithBinding(ctx, root, binding.Executables["ffmpeg"], &binding, "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", file); err != nil {
				t.Fatal(err)
			}
			out, err := m.runWithBinding(ctx, root, binding.Executables["ffprobe"], &binding, "-v", "error", "-show_entries", "format=duration", file)
			if err != nil || !strings.Contains(out, "1.000000") {
				t.Fatalf("audio probe: %v %s", err, out)
			}
		}
		if capability == "poppler" {
			pdf := os.Getenv("AWAKEN_NATIVE_TEST_PDF")
			if pdf == "" {
				t.Fatal("provide an actual PDF to verify data loading and rendering")
			}
			output, err := m.runWithBinding(ctx, root, binding.Executables["pdftotext"], &binding, pdf, "-")
			if err != nil || !strings.Contains(output, "采购") {
				t.Fatalf("Chinese PDF text failed: %v %s", err, output)
			}
			if _, err = m.runWithBinding(ctx, root, binding.Executables["pdftoppm"], &binding, "-f", "1", "-singlefile", "-scale-to", "800", "-png", pdf, filepath.Join(root, "native-preview")); err != nil {
				t.Fatal(err)
			}
			image, err := os.ReadFile(filepath.Join(root, "native-preview.png"))
			if err != nil || len(image) < 1000 || string(image[:8]) != "\x89PNG\r\n\x1a\n" {
				t.Fatal("native renderer did not produce a PNG")
			}
		}
		second, err := m.Ensure(ctx, Request{Capability: capability}, testOwner("另一个项目"), nil)
		if err != nil || !second.Reused {
			t.Fatalf("native component not reused: %v", err)
		}
		// A service may have created a compatible superset environment. Its
		// environment ID can differ while the native component stays identical.
		for name := range catalog.Components[capability].Assets[Target()].Executables {
			if second.Executables[name].Executable != binding.Executables[name].Executable {
				t.Fatalf("component %s was duplicated", name)
			}
		}
		if capability == "browser-automation" {
			verifyNativeBrowserService(t, ctx, m, root)
		}
	}
}

// Exercise the supported service adapter with its upstream launch defaults.
// Raw Chrome --dump-dom is not how Awaken's browser MCP starts or controls it.
func verifyNativeBrowserService(t *testing.T, ctx context.Context, m *Manager, root string) {
	t.Helper()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "<!doctype html><title>shared-runtime</title><p>ready</p>")
	}))
	defer page.Close()
	base, err := m.installEnv()
	if err != nil {
		t.Fatal(err)
	}
	// Service launches inherit the OS temporary directory in Desktop. The
	// installer's private long staging path is not the service's socket root.
	base = append(base, "TMPDIR="+os.TempDir(), "TMP="+os.TempDir(), "TEMP="+os.TempDir())
	launch, env, release, err := m.PrepareCommand(ctx, "npx", []string{"-y", "@playwright/mcp@0.0.80", "--headless", "--isolated"}, base, "native-browser-validation")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	cmd := exec.Command(launch.Executable, launch.Args...)
	cmd.Env, cmd.Dir = env, root
	client := mcp.NewClient(&mcp.Implementation{Name: "native-resource-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	result, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "browser_navigate", Arguments: map[string]any{"url": page.URL}})
	if err != nil {
		t.Fatal(err)
	}
	var content strings.Builder
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			content.WriteString(text.Text)
		}
	}
	if result.IsError || !strings.Contains(content.String(), "shared-runtime") {
		t.Fatalf("browser service did not load the test page: %s", content.String())
	}
	result, err = session.CallTool(callCtx, &mcp.CallToolParams{Name: "browser_evaluate", Arguments: map[string]any{"function": "() => document.body.textContent"}})
	if err != nil {
		t.Fatal(err)
	}
	content.Reset()
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			content.WriteString(text.Text)
		}
	}
	if result.IsError || !strings.Contains(content.String(), "ready") {
		t.Fatalf("browser page content: %s", content.String())
	}
}
