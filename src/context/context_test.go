package context

import "testing"

func TestContextDoesNotStoreSystemPrompt(t *testing.T) {
	ctx := New()
	if got := ctx.GetMessages(); len(got) != 0 {
		t.Fatalf("expected new context to have no messages, got %d", len(got))
	}

	ctx.AddUserMessage("hello")
	ctx.Clear()
	if got := ctx.GetMessages(); len(got) != 0 {
		t.Fatalf("expected cleared context to have no messages, got %d", len(got))
	}
}
