package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"awaken/src/agentcore"
	convcontext "awaken/src/context"
	awakenruntime "awaken/src/runtime"
	"awaken/src/tool"
)

func collaborationBenchmarkFixture(b *testing.B, count int) (*awakenruntime.ExecutionJournal, int64) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "journal.jsonl")
	journal, err := awakenruntime.NewExecutionJournal(path, "perf-synthetic")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = journal.Close() })
	records := make([]agentcore.SemanticRecord, count)
	for i := range records {
		records[i] = agentcore.SemanticRecord{
			SchemaVersion: agentcore.SemanticSchemaVersion,
			RecordID:      fmt.Sprintf("seed-%d", i),
			ThreadID:      "perf-synthetic", RunID: "synthetic-run",
			Kind:      agentcore.SemanticKindAssistantMessage,
			Timestamp: time.Unix(0, 0).UTC(),
			Payload:   map[string]any{"agent_id": fmt.Sprintf("agent-%d", i%8), "content": strings.Repeat("x", 256)},
		}
	}
	if err := journal.AppendBatch(context.Background(), records); err != nil {
		b.Fatal(err)
	}
	if err := journal.Flush(context.Background()); err != nil {
		b.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	return journal, stat.Size()
}

func collaborationBenchmarkRecord(i int) agentcore.SemanticRecord {
	return agentcore.SemanticRecord{
		SchemaVersion: agentcore.SemanticSchemaVersion,
		RecordID:      fmt.Sprintf("operation-%d", i), ThreadID: "perf-synthetic", RunID: "synthetic-run",
		Kind: agentcore.SemanticKindRuntimeNotice, Timestamp: time.Unix(0, 0).UTC(),
		Payload: map[string]any{"notice": "synthetic-only"},
	}
}

func BenchmarkJournal(b *testing.B) {
	for _, count := range []int{1000, 10000, 30000} {
		b.Run(fmt.Sprintf("Records%d", count), func(b *testing.B) {
			b.Run("AppendAndFlush", func(b *testing.B) {
				j, size := collaborationBenchmarkFixture(b, count)
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := j.Append(ctx, collaborationBenchmarkRecord(i)); err != nil {
						b.Fatal(err)
					}
					if err := j.Flush(ctx); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(size)/1024, "seed_KiB")
			})
			b.Run("ReadLast32", func(b *testing.B) {
				j, size := collaborationBenchmarkFixture(b, count)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					values, err := j.SemanticRecords(uint64(count-31), uint64(count))
					if err != nil {
						b.Fatal(err)
					}
					if len(values) != 32 {
						b.Fatalf("got %d records", len(values))
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(size)/1024, "seed_KiB")
			})
			b.Run("AppendFlushedTransaction", func(b *testing.B) {
				j, size := collaborationBenchmarkFixture(b, count)
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := j.AppendFlushed(ctx, collaborationBenchmarkRecord(i)); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(size)/1024, "seed_KiB")
			})
		})
	}
}

func BenchmarkRequestProjection(b *testing.B) {
	for _, count := range []int{32, 128, 512, 1000, 10000, 30000} {
		b.Run(fmt.Sprintf("Messages%d", count), func(b *testing.B) {
			cfg := agentcore.AgentConfig{
				SystemPrompt: "Synthetic local benchmark. No model calls.",
				ModelConfig:  agentcore.ModelConfig{Model: "synthetic-model", ProviderID: "custom", Protocol: "openai_chat", ContextWindow: 131072, MaxTokens: 2048},
			}
			conversation := convcontext.NewConversation(nil)
			messages := make([]convcontext.Message, count)
			for i := range messages {
				role := convcontext.RoleUser
				if i%2 != 0 {
					role = convcontext.RoleAssistant
				}
				messages[i] = convcontext.Message{Role: role, Content: strings.Repeat("x", 512)}
			}
			conversation.Context.SetMessages(messages)
			builder := agentcore.NewRequestBuilder(cfg).BindThread("synthetic-thread")
			emptyTools := []tool.ToolSchema{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := builder.Build(conversation.Context, emptyTools)
				if result.Model != "synthetic-model" {
					b.Fatal("unexpected model")
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(count*512), "history_bytes")
		})
	}
}
