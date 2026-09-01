package skills

import "testing"

func TestParseFrontmatterReadsStandardMetadataDisplayName(t *testing.T) {
	definition, err := ParseFrontmatter(`---
name: outlook-email
description: Manage Outlook email.
metadata:
  display_name: Outlook 邮件
---
Body.
`)
	if err != nil {
		t.Fatal(err)
	}
	if definition.DisplayName != "Outlook 邮件" {
		t.Fatalf("display name = %q", definition.DisplayName)
	}
	if definition.UnknownFields != nil {
		t.Fatalf("metadata must be recognized, unknown = %#v", definition.UnknownFields)
	}
}
