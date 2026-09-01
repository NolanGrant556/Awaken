package authapproval

import "testing"

func TestStorePersistsOnlyMatchingFingerprintAndClearsPrefix(t *testing.T) {
	t.Setenv("AWAKEN_HOME", t.TempDir())
	store := NewStore("approval-test.json")
	if err := store.Mark("plugin\x00account", "v1"); err != nil {
		t.Fatal(err)
	}
	if matched, err := NewStore("approval-test.json").Matches("plugin\x00account", "v1"); err != nil || !matched {
		t.Fatalf("persisted match=%v err=%v", matched, err)
	}
	if matched, err := store.Matches("plugin\x00account", "v2"); err != nil || matched {
		t.Fatalf("stale match=%v err=%v", matched, err)
	}
	if err := store.Mark("other\x00account", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearPrefix("plugin\x00"); err != nil {
		t.Fatal(err)
	}
	if matched, _ := store.Matches("plugin\x00account", "v1"); matched {
		t.Fatal("prefixed approval state was not cleared")
	}
	if matched, _ := store.Matches("other\x00account", "v1"); !matched {
		t.Fatal("unrelated approval state was cleared")
	}
}

func TestRequiredByProviderIsConservative(t *testing.T) {
	for _, code := range []string{"admin_consent_required", "administrator_approval_required", "approval_required"} {
		if !RequiredByProvider(code, "") {
			t.Fatalf("explicit code was not recognized: %s", code)
		}
	}
	if !RequiredByProvider("access_denied", "AADSTS90094: Administrator approval required") {
		t.Fatal("explicit administrator description was not recognized")
	}
	for _, code := range []string{"consent_required", "interaction_required", "authorization_pending", "access_denied"} {
		if RequiredByProvider(code, "") {
			t.Fatalf("ambiguous code was misclassified: %s", code)
		}
	}
}
