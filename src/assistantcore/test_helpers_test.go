package assistantcore

import "testing"

func validAssistant(id string, source AssistantSource, sortOrder int) Assistant {
	return Assistant{
		ID:     id,
		Source: source,
		Profile: AssistantProfile{
			Name: "Assistant " + id,
		},
		State: AssistantState{
			Enabled:   true,
			SortOrder: sortOrder,
		},
		Binding: RuntimeBinding{
			AgentID:   "main",
			RuntimeID: "awaken",
			Source:    "internal",
		},
		Defaults: AssistantDefaults{
			Model:      ScalarDefault{Mode: DefaultModeFixed, Value: "claude-sonnet"},
			Permission: ScalarDefault{Mode: DefaultModeFixed, Value: string(PermissionModeDefault)},
			Skills:     ListDefault{Mode: DefaultModeFixed, Value: []string{"read"}},
			MCPServers: ListDefault{Mode: DefaultModeFixed, Value: []string{"fs"}},
		},
	}
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func assertIDs(t *testing.T, got []Assistant, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected ids %v, got %v", want, idsOf(got))
	}
	for index := range want {
		if got[index].ID != want[index] {
			t.Fatalf("expected ids %v, got %v", want, idsOf(got))
		}
	}
}

func idsOf(assistants []Assistant) []string {
	ids := make([]string, 0, len(assistants))
	for _, assistant := range assistants {
		ids = append(ids, assistant.ID)
	}
	return ids
}
