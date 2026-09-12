// Local acceptance fixture. No credentials, external requests, or writes beyond its event log.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:39178", "loopback listen address")
	logPath := flag.String("log", "/tmp/awaken-reliability-mcp.jsonl", "event log")
	flag.Parse()
	f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var mu sync.Mutex
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewEncoder(f).Encode(map[string]any{"event": event, "at": time.Now().UTC()})
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "reliability-local", Version: "1"}, nil)
	no := false
	mcp.AddTool(server, &mcp.Tool{Name: "hold", Description: "Local acceptance: wait for a non-sensitive form response", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &no, OpenWorldHint: &no}}, func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		record("call_started")
		defer record("call_returned")
		result, err := req.Session.Elicit(ctx, &mcp.ElicitParams{Mode: "form", Message: "退出复合验收：此表单保持待答", RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{"note": map[string]any{"type": "string", "title": "非敏感测试备注"}}}})
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: result.Action}}}, nil, nil
	})
	log.Fatal(http.ListenAndServe(*addr, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)))
}
