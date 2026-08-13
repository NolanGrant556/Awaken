package hookruntime

import "testing"

func TestExecutorSchemaIsStrictAndAppliesAgentBudget(t *testing.T) {
	valid := []string{
		`{"version":1,"enabled":true,"hooks":{"RunStart":[{"matcher":{"toolNames":[]},"hooks":[{"id":"http","type":"http","url":"https://example.com/hook","timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"continue"}]}]}}`,
		`{"version":1,"enabled":true,"hooks":{"BeforeFinish":[{"matcher":{"toolNames":[]},"hooks":[{"id":"prompt","type":"prompt","prompt":"check $ARGUMENTS","timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"block"}]}]}}`,
		`{"version":1,"enabled":true,"hooks":{"BeforeFinish":[{"matcher":{"toolNames":[]},"hooks":[{"id":"agent","type":"agent","prompt":"inspect workspace","timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"block"}]}]}}`,
	}
	for _, raw := range valid {
		cfg, err := decodeConfig([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if hooks := cfg.Hooks[EventBeforeFinish]; len(hooks) > 0 && hooks[0].Hooks[0].Type == "agent" && hooks[0].Hooks[0].MaxTurns != 8 {
			t.Fatalf("agent default budget lost: %+v", hooks[0].Hooks[0])
		}
	}
	invalid := []string{
		`{"version":1,"enabled":true,"hooks":{"RunStart":[{"matcher":{"toolNames":[]},"hooks":[{"id":"mixed","type":"http","url":"https://example.com","command":["echo"],"timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"continue"}]}]}}`,
		`{"version":1,"enabled":true,"hooks":{"BeforeFinish":[{"matcher":{"toolNames":[]},"hooks":[{"id":"wide","type":"agent","prompt":"inspect","maxTurns":9,"timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"block"}]}]}}`,
		`{"version":1,"enabled":true,"hooks":{"RunStart":[{"matcher":{"toolNames":[]},"hooks":[{"id":"secret","type":"http","url":"https://user:pass@example.com","timeoutMs":1000,"maxOutputBytes":1024,"onFailure":"continue"}]}]}}`,
	}
	for _, raw := range invalid {
		if _, err := decodeConfig([]byte(raw)); err == nil {
			t.Fatalf("invalid executor config accepted: %s", raw)
		}
	}
}
