package loadworkspacedependencies

import (
	"awaken/src/managedenv"
	"context"
	"strings"
	"testing"
)

func TestNoModelInstallationParameters(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"capability":"word.create"}`, `{"path":"/tmp/install"}`, `{"requirements":[]}`, `{} {}`} {
		if err := validate(raw); err == nil {
			t.Fatalf("accepted model installation options: %s", raw)
		}
	}
	if err := validate(" { } \n"); err != nil {
		t.Fatal(err)
	}
	d := (&Tool{}).Definition()
	if d.Annotations.ReadOnly || len(d.SideEffects) == 0 {
		t.Fatal("first preparation was falsely declared read-only")
	}
}

func TestMissingManagerReportsOfficeTaskBlocker(t *testing.T) {
	previous := managedenv.Default()
	managedenv.SetDefault(nil)
	defer managedenv.SetDefault(previous)
	out, err := (&Tool{}).ExecuteOutcome(context.Background(), "{}", nil)
	if err != nil || !out.IsError() || out.ErrorCode != "environment_manager_unavailable" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !strings.Contains(out.Content, "没有返回可执行的 environment_ref") || !strings.Contains(out.Content, "不要探测或改用系统") {
		t.Fatalf("missing failure guidance: %+v", out)
	}
}
