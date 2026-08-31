package outlook

import (
	"context"
	"net/http"

	"awaken/src/mcpclient"
	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Runtime struct {
	client *Client
}

func NewRuntime(tokens TokenProvider) *Runtime {
	return &Runtime{client: NewClient(tokens)}
}

func (r *Runtime) SetTokenProvider(tokens TokenProvider) { r.client.SetTokenProvider(tokens) }

func (r *Runtime) SetNeedsAuthCallback(callback func(serverName, reason string) error) {
	r.client.SetNeedsAuthCallback(callback)
}

func (r *Runtime) SetHTTPClientForTest(client *http.Client, baseURL string) {
	r.client.SetHTTPClientForTest(client, baseURL)
}

// Factory returns a fresh in-process MCP server while sharing only the
// stateless Graph client and Core-owned token resolver.
func (r *Runtime) Factory() mcpclient.InProcServerFactory {
	return func() *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: "v1.0.0"}, nil)
		registerTools(server, r.client)
		return server
	}
}

type listMessagesInput struct {
	Folder     string `json:"folder,omitempty" jsonschema:"Mail folder ID or well-known folder name; defaults to inbox"`
	Query      string `json:"query,omitempty" jsonschema:"Optional Microsoft Graph message search text"`
	Top        int    `json:"top,omitempty" jsonschema:"Maximum number of messages, from 1 to 50"`
	UnreadOnly bool   `json:"unreadOnly,omitempty" jsonschema:"Return only unread messages"`
}

type getMessageInput struct {
	MessageID string `json:"messageId" jsonschema:"Microsoft Graph message ID"`
}

type composeInput struct {
	Subject string      `json:"subject"`
	Body    string      `json:"body" jsonschema:"Plain-text message body"`
	To      []Recipient `json:"to"`
	CC      []Recipient `json:"cc,omitempty"`
}

type replyInput struct {
	MessageID string `json:"messageId"`
	Comment   string `json:"comment" jsonschema:"Plain-text reply body"`
}

type updateMessageInput struct {
	MessageID  string   `json:"messageId"`
	IsRead     *bool    `json:"isRead,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

type moveMessageInput struct {
	MessageID     string `json:"messageId"`
	DestinationID string `json:"destinationId" jsonschema:"Destination folder ID or well-known folder name"`
}

type operationOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func registerTools(server *mcp.Server, client *Client) {
	mcp.AddTool(server, readTool("list_messages", "List or search Outlook messages without returning full message bodies."), func(ctx context.Context, _ *mcp.CallToolRequest, input listMessagesInput) (*mcp.CallToolResult, MessageList, error) {
		output, err := client.ListMessages(ctx, input.Folder, input.Query, input.Top, input.UnreadOnly)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("get_message", "Read one Outlook message, including its body, by Microsoft Graph message ID."), func(ctx context.Context, _ *mcp.CallToolRequest, input getMessageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.GetMessage(ctx, input.MessageID)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("create_draft", "Create an Outlook email draft. This does not send the message.", tool.Capability("email.draft"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input composeInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.CreateDraft(ctx, composeMessage(input))
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("send_mail", "Send an Outlook email and save it to Sent Items.", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input composeInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.SendMail(ctx, composeMessage(input))
		return nil, operationOutput{Success: err == nil, Message: "email sent"}, err
	})
	mcp.AddTool(server, writeTool("reply_message", "Reply to an existing Outlook message.", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input replyInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.ReplyMessage(ctx, input.MessageID, input.Comment)
		return nil, operationOutput{Success: err == nil, Message: "reply sent"}, err
	})
	mcp.AddTool(server, writeTool("update_message", "Change the read state or categories of an Outlook message.", tool.Capability("email.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input updateMessageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.UpdateMessage(ctx, input.MessageID, input.IsRead, input.Categories)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("move_message", "Move an Outlook message to another mail folder.", tool.Capability("email.move"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input moveMessageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.MoveMessage(ctx, input.MessageID, input.DestinationID)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("delete_message", "Delete an Outlook message.", tool.Capability("email.delete"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input getMessageInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.DeleteMessage(ctx, input.MessageID)
		return nil, operationOutput{Success: err == nil, Message: "message deleted"}, err
	})
}

func composeMessage(input composeInput) ComposeMessage {
	return ComposeMessage{Subject: input.Subject, Body: input.Body, To: input.To, CC: input.CC}
}

func readTool(name, description string) *mcp.Tool {
	no := false
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &no, OpenWorldHint: &no},
		Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
			ProviderID:   tool.ProviderID("connector:" + ConnectorID),
			Capabilities: []tool.Capability{tool.Capability("email.read")},
		}},
	}
}

func writeTool(name, description string, capability tool.Capability, destructive bool) *mcp.Tool {
	openWorld := true
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, OpenWorldHint: &openWorld},
		Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
			ProviderID:   tool.ProviderID("connector:" + ConnectorID),
			Capabilities: []tool.Capability{capability},
		}},
	}
}
