package officerender

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedLibreOfficeIntegration(t *testing.T) {
	soffice := strings.TrimSpace(os.Getenv("AWAKEN_TEST_OFFICE_RENDER_SOFFICE"))
	root := strings.TrimSpace(os.Getenv("AWAKEN_TEST_OFFICE_RENDER_ROOT"))
	input := strings.TrimSpace(os.Getenv("AWAKEN_TEST_OFFICE_RENDER_INPUT"))
	if soffice == "" || root == "" || input == "" {
		t.Skip("set AWAKEN_TEST_OFFICE_RENDER_SOFFICE, AWAKEN_TEST_OFFICE_RENDER_ROOT, and AWAKEN_TEST_OFFICE_RENDER_INPUT")
	}
	output := filepath.Join(t.TempDir(), "integration-output.pdf")
	result, err := NewOfficeRenderWorker(Config{}).Render(context.Background(), Job{
		Runtime: Runtime{
			Revision:    "integration-test-revision",
			Root:        root,
			SOfficePath: soffice,
		},
		InputPath:  input,
		OutputPath: output,
		Format:     "pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(result.OutputPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty rendered PDF, info=%v err=%v", info, err)
	}
}

func TestSharedHostRuntimeIntegration(t *testing.T) {
	soffice := strings.TrimSpace(os.Getenv("AWAKEN_TEST_OFFICE_RENDER_SOFFICE"))
	input := strings.TrimSpace(os.Getenv("AWAKEN_TEST_OFFICE_RENDER_INPUT"))
	if soffice == "" || input == "" {
		t.Skip("set AWAKEN_TEST_OFFICE_RENDER_SOFFICE and AWAKEN_TEST_OFFICE_RENDER_INPUT")
	}
	t.Setenv("AWAKEN_SOFFICE", soffice)
	output := filepath.Join(t.TempDir(), "shared-host-output.pdf")
	result, err := Render(context.Background(), input, output, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(result.OutputPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty shared managed PDF, info=%v err=%v", info, err)
	}
}
