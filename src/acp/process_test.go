package acp

import (
	"os"
	"strings"
	"testing"
)

func TestMergeProcessEnvInheritsBase(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/home/user"}
	overrides := map[string]string{"FOO": "bar"}
	merged := MergeProcessEnv(base, overrides)

	has := func(key, value string) bool {
		prefix := key + "=" + value
		for _, e := range merged {
			if e == prefix {
				return true
			}
		}
		return false
	}

	if !has("FOO", "bar") {
		t.Fatalf("expected override FOO=bar in %v", merged)
	}
	if !has("PATH", "/usr/bin") {
		t.Fatalf("expected base PATH to be preserved in %v", merged)
	}
	if !has("HOME", "/home/user") {
		t.Fatalf("expected base HOME to be preserved in %v", merged)
	}
}

func TestMergeProcessEnvOverridesBase(t *testing.T) {
	base := []string{"PATH=/usr/bin", "FOO=old"}
	overrides := map[string]string{"FOO": "new"}
	merged := MergeProcessEnv(base, overrides)

	for _, e := range merged {
		if e == "FOO=new" {
			return
		}
	}
	t.Fatalf("expected FOO=new in %v", merged)
}

func TestMergeProcessEnvNoOverridesReturnsBase(t *testing.T) {
	base := []string{"PATH=/usr/bin"}
	merged := MergeProcessEnv(base, nil)
	if len(merged) != 1 || merged[0] != base[0] {
		t.Fatalf("expected base unchanged, got %v", merged)
	}
}

func TestMergeProcessEnvWindowsCaseInsensitive(t *testing.T) {
	if os.PathSeparator != '\\' {
		t.Skip("Windows-only case-insensitive env test")
	}
	base := []string{"Path=C:\\old"}
	overrides := map[string]string{"PATH": "C:\\new"}
	merged := MergeProcessEnv(base, overrides)

	for _, e := range merged {
		if strings.EqualFold(e, "PATH=C:\\new") {
			return
		}
	}
	t.Fatalf("expected PATH override on Windows, got %v", merged)
}
