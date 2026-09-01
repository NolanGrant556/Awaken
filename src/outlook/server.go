package outlook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"awaken/src/desktopdata"
	"awaken/src/mcpclient"
	"awaken/src/tool"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Runtime struct {
	client *Client
}

func NewRuntime(tokens TokenProvider) *Runtime {
	client := NewClient(tokens)
	if root := desktopdata.AwakenHomeRoot(); root != "" {
		client.scheduler = NewMailScheduler(client, root)
	}
	return &Runtime{client: client}
}

func (r *Runtime) Start() error {
	if r != nil && r.client != nil && r.client.scheduler != nil {
		return r.client.scheduler.Start()
	}
	return nil
}

func (r *Runtime) Close() {
	if r != nil && r.client != nil && r.client.scheduler != nil {
		r.client.scheduler.Stop()
	}
}

func (r *Runtime) SetSchedulerForTest(scheduler *MailScheduler) { r.client.scheduler = scheduler }

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
		server := mcp.NewServer(&mcp.Implementation{Name: ServerName, Version: "v1.4.0"}, nil)
		registerTools(server, r.client)
		return server
	}
}

type mailboxInput struct {
	Mailbox string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
}

type listMessagesInput struct {
	Mailbox    string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	Folder     string `json:"folder,omitempty" jsonschema:"Mail folder ID or well-known folder name; defaults to inbox"`
	Query      string `json:"query,omitempty" jsonschema:"Compatibility free-text search; prefer search_messages when the intent is search"`
	Top        int    `json:"top,omitempty" jsonschema:"Maximum number of messages, from 1 to 50"`
	Cursor     string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor returned by the previous list_messages page; copy it exactly and do not decode or alter it"`
	UnreadOnly bool   `json:"unreadOnly,omitempty" jsonschema:"Return only unread messages"`
}

type searchMessagesInput struct {
	Mailbox string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	Folder  string `json:"folder,omitempty" jsonschema:"Optional folder ID or well-known folder name; omit to search the whole mailbox"`
	Query   string `json:"query" jsonschema:"Required free-text Microsoft Graph mail search, such as a subject phrase or from:person@example.com"`
	Size    int    `json:"size,omitempty" jsonschema:"Maximum results per page, from 1 to 50; defaults to 20"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"Opaque nextCursor returned by the previous search_messages page; copy it exactly and do not decode or alter it"`
}

type recentMessagesInput struct {
	Mailbox string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	TopK    int    `json:"topK,omitempty" jsonschema:"Number of recent inbox messages, from 1 to 20; defaults to 5"`
}

type messageInput struct {
	Mailbox   string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	MessageID string `json:"messageId" jsonschema:"Microsoft Graph message ID"`
}

type messageBatchInput struct {
	Mailbox    string   `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	MessageIDs []string `json:"messageIds" jsonschema:"One to 20 unique raw message IDs returned by list/search tools"`
}

type composeInput struct {
	Mailbox         string   `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	Subject         string   `json:"subject"`
	Body            string   `json:"body" jsonschema:"Plain-text message body"`
	To              []string `json:"to" jsonschema:"Required array of recipient email address strings, for example [person@example.com]. Never wrap the array or its items in an item object."`
	CC              []string `json:"cc,omitempty" jsonschema:"Optional array of CC email address strings"`
	BCC             []string `json:"bcc,omitempty" jsonschema:"Optional array of BCC email address strings"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty" jsonschema:"Absolute paths of local files the user explicitly selected, up to 150 MiB each"`
}

type updateDraftInput struct {
	Mailbox   string   `json:"mailbox,omitempty"`
	MessageID string   `json:"messageId"`
	Subject   *string  `json:"subject,omitempty"`
	Body      *string  `json:"body,omitempty" jsonschema:"Plain-text message body"`
	To        []string `json:"to,omitempty" jsonschema:"Optional array of recipient email address strings"`
	CC        []string `json:"cc,omitempty" jsonschema:"Optional array of CC email address strings"`
	BCC       []string `json:"bcc,omitempty" jsonschema:"Optional array of BCC email address strings"`
}

type replyInput struct {
	Mailbox   string `json:"mailbox,omitempty"`
	MessageID string `json:"messageId"`
	Comment   string `json:"comment" jsonschema:"Plain-text reply body"`
	ReplyAll  bool   `json:"replyAll,omitempty" jsonschema:"Reply to all original recipients instead of only the sender"`
}

type replyDraftInput struct {
	Mailbox         string   `json:"mailbox,omitempty"`
	MessageID       string   `json:"messageId"`
	Comment         string   `json:"comment,omitempty" jsonschema:"Plain-text reply body"`
	ReplyAll        bool     `json:"replyAll,omitempty"`
	CC              []string `json:"cc,omitempty" jsonschema:"Optional CC email address strings to merge into the reply draft"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty"`
}

type forwardInput struct {
	Mailbox   string   `json:"mailbox,omitempty"`
	MessageID string   `json:"messageId"`
	Comment   string   `json:"comment,omitempty" jsonschema:"Plain-text text placed above the forwarded message"`
	To        []string `json:"to" jsonschema:"Required array of recipient email address strings"`
}

type forwardDraftInput struct {
	Mailbox         string   `json:"mailbox,omitempty"`
	MessageID       string   `json:"messageId"`
	Comment         string   `json:"comment,omitempty"`
	To              []string `json:"to" jsonschema:"Required array of recipient email address strings"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty"`
}

type updateMessageInput struct {
	Mailbox    string   `json:"mailbox,omitempty"`
	MessageID  string   `json:"messageId"`
	IsRead     *bool    `json:"isRead,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

type markMessageReadStateInput struct {
	Mailbox   string `json:"mailbox,omitempty"`
	MessageID string `json:"messageId"`
	IsRead    bool   `json:"isRead" jsonschema:"true marks the message read; false marks it unread"`
}

type setMessageCategoriesInput struct {
	Mailbox    string   `json:"mailbox,omitempty"`
	MessageID  string   `json:"messageId"`
	Categories []string `json:"categories,omitempty" jsonschema:"Complete replacement list of Outlook category display names. Use clear=true instead of inventing an empty string when removing all categories."`
	Clear      bool     `json:"clear,omitempty" jsonschema:"Set true to remove all categories. Do not send categories as an empty string."`
}

type moveMessageInput struct {
	Mailbox       string `json:"mailbox,omitempty"`
	MessageID     string `json:"messageId"`
	DestinationID string `json:"destinationId" jsonschema:"Destination folder ID or well-known folder name"`
}

type listMailFoldersInput struct {
	Mailbox        string `json:"mailbox,omitempty"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
	Top            int    `json:"top,omitempty"`
	IncludeHidden  bool   `json:"includeHidden,omitempty"`
}

type findMailFolderInput struct {
	Mailbox       string `json:"mailbox,omitempty" jsonschema:"Exact shared mailbox email address or UPN; omit for the signed-in mailbox"`
	Query         string `json:"query" jsonschema:"Folder display name or path fragment, such as Finance or Projects/Finance"`
	MaxResults    int    `json:"maxResults,omitempty" jsonschema:"Maximum ranked candidates, from 1 to 20; defaults to 10"`
	IncludeHidden bool   `json:"includeHidden,omitempty" jsonschema:"Include hidden folders only when the user explicitly asks"`
}

type createMailFolderInput struct {
	Mailbox        string `json:"mailbox,omitempty"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
	DisplayName    string `json:"displayName"`
	Hidden         bool   `json:"hidden,omitempty"`
}

type updateMailFolderInput struct {
	Mailbox     string `json:"mailbox,omitempty"`
	FolderID    string `json:"folderId"`
	DisplayName string `json:"displayName"`
}

type folderInput struct {
	Mailbox  string `json:"mailbox,omitempty"`
	FolderID string `json:"folderId"`
}

type categoryInput struct {
	DisplayName string `json:"displayName"`
	Color       string `json:"color" jsonschema:"Outlook category color: none or preset0 through preset24"`
}

type updateCategoryInput struct {
	CategoryID string `json:"categoryId"`
	Color      string `json:"color" jsonschema:"New Outlook category color: none or preset0 through preset24. Microsoft Graph does not allow renaming a category after creation."`
}

type categoryIDInput struct {
	CategoryID string `json:"categoryId"`
}

type attachmentInput struct {
	Mailbox      string `json:"mailbox,omitempty"`
	MessageID    string `json:"messageId"`
	AttachmentID string `json:"attachmentId"`
}

type downloadAttachmentInput struct {
	Mailbox      string `json:"mailbox,omitempty"`
	MessageID    string `json:"messageId"`
	AttachmentID string `json:"attachmentId"`
	OutputPath   string `json:"outputPath" jsonschema:"Absolute destination path in an existing directory; existing files are never overwritten"`
}

type addAttachmentInput struct {
	Mailbox   string `json:"mailbox,omitempty"`
	MessageID string `json:"messageId"`
	FilePath  string `json:"filePath" jsonschema:"Absolute local file path explicitly selected by the user"`
}

type unsubscribeInput struct {
	Mailbox         string `json:"mailbox,omitempty"`
	MessageID       string `json:"messageId"`
	PreferredMethod string `json:"preferredMethod,omitempty" jsonschema:"Optional one_click or mailto preference"`
}

type scheduleMailInput struct {
	Mailbox         string   `json:"mailbox,omitempty"`
	Subject         string   `json:"subject"`
	Body            string   `json:"body"`
	To              []string `json:"to" jsonschema:"Required array of recipient email address strings"`
	CC              []string `json:"cc,omitempty" jsonschema:"Optional array of CC email address strings"`
	BCC             []string `json:"bcc,omitempty" jsonschema:"Optional array of BCC email address strings"`
	AttachmentPaths []string `json:"attachmentPaths,omitempty"`
	SendAt          string   `json:"sendAt" jsonschema:"RFC3339 timestamp with explicit timezone, at least 5 seconds and at most 366 days in the future"`
}

type scheduleDraftInput struct {
	Mailbox string `json:"mailbox,omitempty"`
	DraftID string `json:"draftId"`
	SendAt  string `json:"sendAt" jsonschema:"RFC3339 timestamp with explicit timezone"`
}

type cancelScheduledMailInput struct {
	ScheduleID  string `json:"scheduleId"`
	DeleteDraft bool   `json:"deleteDraft,omitempty" jsonschema:"Also delete the Outlook draft after cancelling; defaults to false"`
}

type operationOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type scheduledMailOutput struct {
	ID            string `json:"id"`
	Mailbox       string `json:"mailbox,omitempty"`
	DraftID       string `json:"draftId"`
	SendAt        string `json:"sendAt"`
	CreatedAt     string `json:"createdAt"`
	NextAttemptAt string `json:"nextAttemptAt,omitempty"`
	State         string `json:"state"`
	LastError     string `json:"lastError,omitempty"`
	DraftCleanup  string `json:"draftCleanup,omitempty"`
}

type scheduledMailListOutput struct {
	Items []scheduledMailOutput `json:"items"`
}

func registerTools(server *mcp.Server, client *Client) {
	mcp.AddTool(server, readTool("list_messages", "浏览个人/共享 Outlook 文件夹中的近期或未读邮件；结果已含主题、发件人、时间、已读状态和正文预览，除非用户明确需要完整正文，否则不要随后调用 get_message。分页时把 nextCursor 原样传回 cursor，不要推算偏移。自由文本检索优先使用 search_messages。"), func(ctx context.Context, _ *mcp.CallToolRequest, input listMessagesInput) (*mcp.CallToolResult, MessageList, error) {
		output, err := client.ListMessagesInMailboxPage(ctx, input.Mailbox, input.Folder, input.Query, input.Top, input.Cursor, input.UnreadOnly)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("search_messages", "在个人/共享 Outlook 邮箱中进行自由文本检索。传入 list/search 返回的原始 message ID，不要猜测 ID；结果已含高价值预览字段，只有用户明确需要完整正文时才继续 get_message 或 get_messages_batch。分页时把 nextCursor 原样传回 cursor，不要推算偏移。"), func(ctx context.Context, _ *mcp.CallToolRequest, input searchMessagesInput) (*mcp.CallToolResult, MessageList, error) {
		output, err := client.SearchMessagesInMailbox(ctx, input.Mailbox, input.Folder, input.Query, input.Size, input.Cursor)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("get_recent_emails", "返回个人/共享 Outlook 收件箱最近收到的少量邮件。仅用于简单的“最近几封”请求；需要未读筛选、文件夹、搜索或分页时改用 list_messages/search_messages。"), func(ctx context.Context, _ *mcp.CallToolRequest, input recentMessagesInput) (*mcp.CallToolResult, MessageList, error) {
		top := input.TopK
		if top <= 0 {
			top = 5
		}
		if top > 20 {
			top = 20
		}
		output, err := client.ListMessagesInMailboxPage(ctx, input.Mailbox, "inbox", "", top, "", false)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("get_message", "读取个人/共享 Outlook 邮箱中的一封完整邮件。只在 list/search 的主题、发件人、时间和正文预览不足时调用；敏感退订 URL 不在普通邮件结果中返回。"), func(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.GetMessageInMailbox(ctx, input.Mailbox, input.MessageID)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("get_messages_batch", "批量读取一到 20 封个人/共享 Outlook 完整邮件。只有用户明确需要多封完整正文时使用；近期、未读和摘要任务优先一次调用 list_messages/search_messages，避免对每封邮件重复抓取。单项失败会保留在 failures 中，不自动重试。"), func(ctx context.Context, _ *mcp.CallToolRequest, input messageBatchInput) (*mcp.CallToolResult, MessageBatch, error) {
		output, err := client.GetMessagesBatchInMailbox(ctx, input.Mailbox, input.MessageIDs)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("list_mail_folders", "列出个人或共享 Outlook 邮箱的顶层或指定父文件夹的直接子文件夹。用户只给文件夹名称/路径且需要移动邮件时，优先使用 find_mail_folder 取得精确 ID。"), func(ctx context.Context, _ *mcp.CallToolRequest, input listMailFoldersInput) (*mcp.CallToolResult, MailFolderList, error) {
		output, err := client.ListMailFolders(ctx, input.Mailbox, input.ParentFolderID, input.Top, input.IncludeHidden)
		return nil, output, err
	})
	mcp.AddTool(server, readTool("find_mail_folder", "按名称或路径递归查找个人/共享 Outlook 邮件文件夹，并按精确路径、精确名称、路径后缀和包含关系排序。支持 locale-independent well-known 名称，例如 inbox、drafts、sentitems、deleteditems 和 archive。移动邮件前若用户只说“财务”或“Projects/Finance”，先调用本工具；多个强候选时展示给用户，不要猜。"), func(ctx context.Context, _ *mcp.CallToolRequest, input findMailFolderInput) (*mcp.CallToolResult, MailFolderMatches, error) {
		output, err := client.FindMailFolders(ctx, input.Mailbox, input.Query, input.MaxResults, input.IncludeHidden)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("create_mail_folder", "在个人或共享 Outlook 邮箱中创建文件夹。", tool.Capability("email.folder.create"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input createMailFolderInput) (*mcp.CallToolResult, MailFolder, error) {
		output, err := client.CreateMailFolder(ctx, input.Mailbox, input.ParentFolderID, input.DisplayName, input.Hidden)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("update_mail_folder", "重命名 Outlook 邮件文件夹。", tool.Capability("email.folder.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input updateMailFolderInput) (*mcp.CallToolResult, MailFolder, error) {
		output, err := client.UpdateMailFolder(ctx, input.Mailbox, input.FolderID, input.DisplayName)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("delete_mail_folder", "删除 Outlook 邮件文件夹。", tool.Capability("email.folder.delete"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input folderInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.DeleteMailFolder(ctx, input.Mailbox, input.FolderID)
		return nil, operationOutput{Success: err == nil, Message: "邮件文件夹已删除"}, err
	})
	mcp.AddTool(server, readTool("list_categories", "列出当前 Outlook 账号的主分类。"), func(ctx context.Context, _ *mcp.CallToolRequest, _ mailboxInput) (*mcp.CallToolResult, CategoryList, error) {
		output, err := client.ListCategories(ctx)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("create_category", "创建 Outlook 主分类。", tool.Capability("email.category.create"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input categoryInput) (*mcp.CallToolResult, OutlookCategory, error) {
		output, err := client.CreateCategory(ctx, input.DisplayName, input.Color)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("update_category", "更新 Outlook 主分类颜色。Microsoft Graph 创建分类后不允许修改名称；需要新名称时创建新分类，并在用户确认迁移范围后重新应用分类。", tool.Capability("email.category.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input updateCategoryInput) (*mcp.CallToolResult, OutlookCategory, error) {
		output, err := client.UpdateCategory(ctx, input.CategoryID, input.Color)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("delete_category", "删除 Outlook 主分类。", tool.Capability("email.category.delete"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input categoryIDInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.DeleteCategory(ctx, input.CategoryID)
		return nil, operationOutput{Success: err == nil, Message: "分类已删除"}, err
	})
	mcp.AddTool(server, readTool("list_attachments", "列出一封个人/共享邮箱邮件的附件元数据。"), func(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, AttachmentList, error) {
		output, err := client.ListAttachments(ctx, input.Mailbox, input.MessageID)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("download_attachment", "把 Outlook 邮件附件下载到用户指定的新本地文件；不覆盖现有文件。", tool.Capability("email.attachment.download"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input downloadAttachmentInput) (*mcp.CallToolResult, DownloadedAttachment, error) {
		output, err := client.DownloadAttachment(ctx, input.Mailbox, input.MessageID, input.AttachmentID, input.OutputPath)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("add_attachment", "把用户明确选择的本地文件添加到 Outlook 草稿，单文件最大 150 MiB。", tool.Capability("email.attachment.add"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input addAttachmentInput) (*mcp.CallToolResult, Attachment, error) {
		output, err := client.AddAttachment(ctx, input.Mailbox, input.MessageID, input.FilePath)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("delete_attachment", "从 Outlook 草稿中删除一个附件。", tool.Capability("email.attachment.delete"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input attachmentInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.DeleteAttachment(ctx, input.Mailbox, input.MessageID, input.AttachmentID)
		return nil, operationOutput{Success: err == nil, Message: "附件已删除"}, err
	})
	mcp.AddTool(server, writeTool("create_draft", "创建个人或共享邮箱的新邮件草稿，可附加本地文件但不会发送。to/cc/bcc 必须是邮箱地址字符串数组，例如 [\"person@example.com\"]。", tool.Capability("email.draft"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input composeInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.CreateDraftInMailbox(ctx, input.Mailbox, composeMessage(input))
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("update_draft", "更新现有 Outlook 草稿的主题、正文或收件人；不会发送。收件人字段必须是邮箱地址字符串数组。", tool.Capability("email.draft.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input updateDraftInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.UpdateDraft(ctx, input.Mailbox, input.MessageID, DraftUpdate{
			Subject: input.Subject, Body: input.Body,
			To: recipientAddresses(input.To), CC: recipientAddresses(input.CC), BCC: recipientAddresses(input.BCC),
		})
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("send_draft", "立即发送一个已存在的 Outlook 草稿。调用前必须先读取草稿并向用户展示发件邮箱、收件人、主题、附件和发送方式；若用户尚未确认，先提问或停止等待。不要发送已安排定时发送的草稿。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, operationOutput, error) {
		if err := ensureDraftNotScheduled(client, input.Mailbox, input.MessageID); err != nil {
			return nil, operationOutput{}, err
		}
		err := client.SendDraft(ctx, input.Mailbox, input.MessageID)
		return nil, operationOutput{Success: err == nil, Message: "草稿已发送"}, err
	})
	mcp.AddTool(server, writeTool("send_mail", "立即发送个人或共享 Outlook 邮件，可包含本地附件。to/cc/bcc 必须是邮箱地址字符串数组。只有用户看过最终发送摘要并明确确认后才能调用；否则先提问或停止等待。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input composeInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.SendMailFromMailbox(ctx, input.Mailbox, composeMessage(input))
		return nil, operationOutput{Success: err == nil, Message: "邮件已发送"}, err
	})
	mcp.AddTool(server, writeTool("schedule_mail", "创建 Outlook 草稿并由本机 Awaken 在指定时间发送；计划持久化但 Desktop 完全退出期间不会执行。只有用户确认最终收件人、主题、附件、当地时间和时区后才能调用。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input scheduleMailInput) (*mcp.CallToolResult, scheduledMailOutput, error) {
		scheduler, err := requireMailScheduler(client)
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		output, err := scheduler.ScheduleMail(ctx, input.Mailbox, input.SendAt, ComposeMessage{
			Subject: input.Subject, Body: input.Body,
			To: recipientAddresses(input.To), CC: recipientAddresses(input.CC), BCC: recipientAddresses(input.BCC),
			AttachmentPaths: input.AttachmentPaths,
		})
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		return nil, scheduledMailResult(output, ""), nil
	})
	mcp.AddTool(server, writeTool("schedule_draft", "安排一个现有 Outlook 草稿在指定时间由本机 Awaken 发送。调用前必须读取草稿并取得用户对收件人、主题、附件、当地时间和时区的最终确认；已计划草稿不能重复安排或立即发送。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input scheduleDraftInput) (*mcp.CallToolResult, scheduledMailOutput, error) {
		scheduler, err := requireMailScheduler(client)
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		output, err := scheduler.ScheduleDraft(ctx, input.Mailbox, input.DraftID, input.SendAt)
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		return nil, scheduledMailResult(output, ""), nil
	})
	mcp.AddTool(server, readTool("list_scheduled_mail", "列出本机 Awaken 保存的 Outlook 定时发送计划，不包含邮件正文。"), func(ctx context.Context, _ *mcp.CallToolRequest, _ mailboxInput) (*mcp.CallToolResult, scheduledMailListOutput, error) {
		scheduler, err := requireMailScheduler(client)
		if err != nil {
			return nil, scheduledMailListOutput{}, err
		}
		output, err := scheduler.List()
		if err != nil {
			return nil, scheduledMailListOutput{}, err
		}
		return nil, scheduledMailListResult(output), nil
	})
	mcp.AddTool(server, writeTool("cancel_scheduled_mail", "取消本机 Outlook 定时发送计划；可选同时删除对应草稿。草稿已经不存在时仍视为清理完成。", tool.Capability("email.schedule.cancel"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input cancelScheduledMailInput) (*mcp.CallToolResult, scheduledMailOutput, error) {
		scheduler, err := requireMailScheduler(client)
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		output, err := scheduler.Cancel(ctx, input.ScheduleID, input.DeleteDraft)
		if err != nil {
			return nil, scheduledMailOutput{}, err
		}
		return nil, scheduledMailResult(output.ScheduledMail, output.DraftCleanup), nil
	})
	mcp.AddTool(server, writeTool("create_reply_draft", "基于现有邮件创建回复或回复全部草稿，可合并额外 CC、添加附件，但不会发送。用户想回复、起草、准备或尚未审核最终内容时优先使用本工具；不要直接调用 reply_message。", tool.Capability("email.draft"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input replyDraftInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.CreateReplyDraft(ctx, input.Mailbox, input.MessageID, input.Comment, input.ReplyAll, recipientAddresses(input.CC), input.AttachmentPaths)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("reply_message", "立即回复现有 Outlook 邮件；收件人由原邮件决定。调用前必须读取原邮件并向用户展示实际回复对象、回复或回复全部、正文和附件；若用户尚未确认，先创建回复草稿并提问或停止等待。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input replyInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.ReplyMessageInMailbox(ctx, input.Mailbox, input.MessageID, input.Comment, input.ReplyAll)
		return nil, operationOutput{Success: err == nil, Message: "回复已发送"}, err
	})
	mcp.AddTool(server, writeTool("create_forward_draft", "基于现有邮件创建转发草稿，可添加额外附件但不会发送。to 必须是邮箱地址字符串数组。", tool.Capability("email.draft"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input forwardDraftInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.CreateForwardDraft(ctx, input.Mailbox, input.MessageID, input.Comment, recipientAddresses(input.To), input.AttachmentPaths)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("forward_message", "立即转发现有 Outlook 邮件。to 必须是邮箱地址字符串数组；只有用户看过原邮件、最终收件人、附言和附件摘要并明确确认后才能调用。", tool.CapabilityMessageSend, false), func(ctx context.Context, _ *mcp.CallToolRequest, input forwardInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.ForwardMessage(ctx, input.Mailbox, input.MessageID, input.Comment, recipientAddresses(input.To))
		return nil, operationOutput{Success: err == nil, Message: "邮件已转发"}, err
	})
	mcp.AddTool(server, writeTool("mark_message_read_state", "把个人或共享 Outlook 邮件标记为已读或未读，并返回更新后的邮件。只改变阅读状态；不要用它表达任务已完成。", tool.Capability("email.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input markMessageReadStateInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.UpdateMessageInMailbox(ctx, input.Mailbox, input.MessageID, &input.IsRead, nil)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("set_message_categories", "完整替换个人或共享 Outlook 邮件的分类列表，并返回更新后的邮件。清除全部分类时传 clear=true，不要把 categories 传成空字符串；需要追加时先读取当前分类再合并。", tool.Capability("email.update"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input setMessageCategoriesInput) (*mcp.CallToolResult, Message, error) {
		if input.Clear && len(input.Categories) > 0 {
			return nil, Message{}, fmt.Errorf("clear=true cannot be combined with non-empty categories")
		}
		categories := input.Categories
		if input.Clear {
			categories = []string{}
		}
		if categories == nil {
			return nil, Message{}, fmt.Errorf("categories or clear=true is required")
		}
		output, err := client.UpdateMessageInMailbox(ctx, input.Mailbox, input.MessageID, nil, categories)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("update_message", "兼容性组合工具：修改个人或共享 Outlook 邮件的已读状态或分类。单一意图优先使用 mark_message_read_state 或 set_message_categories，避免把两个变更意外绑定。", tool.Capability("email.update"), false), func(ctx context.Context, _ *mcp.CallToolRequest, input updateMessageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.UpdateMessageInMailbox(ctx, input.Mailbox, input.MessageID, input.IsRead, input.Categories)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("move_message", "移动个人或共享 Outlook 邮件到目标文件夹并返回移动后的邮件。用户只给文件夹名称/路径时先用 find_mail_folder，多个候选时不得猜测；移动后继续使用返回的新 message ID。", tool.Capability("email.move"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input moveMessageInput) (*mcp.CallToolResult, Message, error) {
		output, err := client.MoveMessageInMailbox(ctx, input.Mailbox, input.MessageID, input.DestinationID)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("delete_message", "删除个人或共享 Outlook 邮件。", tool.Capability("email.delete"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, operationOutput, error) {
		err := client.DeleteMessageInMailbox(ctx, input.Mailbox, input.MessageID)
		return nil, operationOutput{Success: err == nil, Message: "邮件已删除"}, err
	})
	mcp.AddTool(server, readTool("get_unsubscribe_info", "检查邮件是否提供 RFC 8058 一键退订、mailto 退订或只能打开外部页面；不暴露带令牌的完整 URL。"), func(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, UnsubscribeInfo, error) {
		output, err := client.GetUnsubscribeInfo(ctx, input.Mailbox, input.MessageID)
		return nil, output, err
	})
	mcp.AddTool(server, writeTool("unsubscribe_message", "对一封邮件执行标准一键退订，优先使用 RFC 8058 HTTPS POST，必要时会真实发送退订邮件。调用前必须展示发件人、主题和退订方式并取得用户确认。退订不授权回复、移动或删除原邮件。", tool.Capability("email.unsubscribe"), true), func(ctx context.Context, _ *mcp.CallToolRequest, input unsubscribeInput) (*mcp.CallToolResult, UnsubscribeResult, error) {
		output, err := client.UnsubscribeMessage(ctx, input.Mailbox, input.MessageID, input.PreferredMethod)
		return nil, output, err
	})
}

func requireMailScheduler(client *Client) (*MailScheduler, error) {
	if client == nil || client.scheduler == nil {
		return nil, fmt.Errorf("Outlook 本地定时发送不可用")
	}
	return client.scheduler, nil
}

func ensureDraftNotScheduled(client *Client, mailbox, draftID string) error {
	if client == nil || client.scheduler == nil {
		return nil
	}
	job, found, err := client.scheduler.ScheduledForDraft(mailbox, draftID)
	if err != nil {
		return err
	}
	if found {
		return fmt.Errorf("该草稿已有定时发送计划 %s；请先取消计划，再决定是否立即发送", job.ID)
	}
	return nil
}

func recipientAddresses(addresses []string) []Recipient {
	if addresses == nil {
		return nil
	}
	recipients := make([]Recipient, 0, len(addresses))
	for _, address := range addresses {
		recipients = append(recipients, Recipient{Address: address})
	}
	return recipients
}

func scheduledMailResult(job ScheduledMail, draftCleanup string) scheduledMailOutput {
	result := scheduledMailOutput{
		ID: job.ID, Mailbox: job.Mailbox, DraftID: job.DraftID,
		SendAt: job.SendAt.UTC().Format(time.RFC3339Nano), CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339Nano),
		State: job.State, LastError: job.LastError, DraftCleanup: draftCleanup,
	}
	if job.NextAttemptAt != nil {
		result.NextAttemptAt = job.NextAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	return result
}

func scheduledMailListResult(list ScheduledMailList) scheduledMailListOutput {
	result := scheduledMailListOutput{Items: make([]scheduledMailOutput, 0, len(list.Items))}
	for _, item := range list.Items {
		result.Items = append(result.Items, scheduledMailResult(item, ""))
	}
	return result
}

func composeMessage(input composeInput) ComposeMessage {
	return ComposeMessage{
		Subject: input.Subject, Body: input.Body,
		To: recipientAddresses(input.To), CC: recipientAddresses(input.CC), BCC: recipientAddresses(input.BCC),
		AttachmentPaths: input.AttachmentPaths,
	}
}

func readTool(name, description string) *mcp.Tool {
	no := false
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &no, OpenWorldHint: &no},
		Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
			ProviderID: tool.ProviderID("connector:" + ConnectorID), Capabilities: []tool.Capability{tool.Capability("email.read")},
		}},
	}
}

func writeTool(name, description string, capability tool.Capability, destructive bool) *mcp.Tool {
	openWorld := true
	return &mcp.Tool{
		Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, OpenWorldHint: &openWorld},
		Meta: mcp.Meta{mcpclient.AwakenProviderMetaKey: mcpclient.MCPToolProviderMetadata{
			ProviderID: tool.ProviderID("connector:" + ConnectorID), Capabilities: []tool.Capability{capability},
		}},
	}
}
