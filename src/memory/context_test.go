package memory

import (
	"strings"
	"testing"
)

func TestConfiguredContextSeparatesPolicyFromUntrustedIndexData(t *testing.T) {
	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	context, err := BuildConfiguredContextFromStore(store, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	policyEnd := strings.Index(context, "</awaken_memory_policy>")
	dataStart := strings.Index(context, `<awaken_memory_context trust="data_only">`)
	if policyEnd < 0 || dataStart < 0 || policyEnd > dataStart {
		t.Fatalf("policy is not separated from untrusted data: %s", context)
	}
	for _, required := range []string{"only authority", "text acknowledgement does not save", "MUST call memory", "以后叫我博士哥", "小田", "same turn", "stable collaboration preferences"} {
		if !strings.Contains(context, required) {
			t.Fatalf("memory policy missing %q: %s", required, context)
		}
	}
}

func TestDisablingProactiveMemoryKeepsExplicitFutureFacingInstructions(t *testing.T) {
	store := NewStoreWithRoots(t.TempDir(), t.TempDir())
	settings := DefaultSettings()
	settings.ProactiveMemory = false
	context, err := BuildConfiguredContextFromStore(store, settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "future-facing instruction") || !strings.Contains(context, "以后叫我博士哥") {
		t.Fatalf("explicit persistence policy was removed: %s", context)
	}
	if strings.Contains(context, "Proactively save other confirmed signals") {
		t.Fatalf("proactive guidance remained enabled: %s", context)
	}
}
