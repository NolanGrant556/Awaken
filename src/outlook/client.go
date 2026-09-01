package outlook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"awaken/src/connectorauth"
	"awaken/src/networkroute"
)

const (
	ConnectorID = "awaken.microsoft.outlook"
	ServerName  = "awaken-outlook"

	defaultGraphBaseURL = "https://graph.microsoft.com/v1.0"
	maxGraphResponse    = 2 << 20
)

var ErrNeedsAuth = errors.New("outlook account authorization is required")

// TokenProvider is implemented by the Core-owned Connector Auth Broker. The
// Outlook runtime receives only a short-lived access token at request time;
// it never owns or persists account credentials.
type TokenProvider interface {
	AccessToken(ctx context.Context, connectorID string) (string, error)
	MarkNeedsReauth(connectorID string) error
}

type Client struct {
	httpClient  *http.Client
	baseURL     string
	tokens      TokenProvider
	onNeedsAuth func(serverName, reason string) error
	scheduler   *MailScheduler
}

func NewClient(tokens TokenProvider) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    defaultGraphBaseURL,
		tokens:     tokens,
	}
}

func (c *Client) SetTokenProvider(tokens TokenProvider) { c.tokens = tokens }

func (c *Client) SetNeedsAuthCallback(callback func(serverName, reason string) error) {
	c.onNeedsAuth = callback
}

// SetHTTPClientForTest replaces the Graph endpoint and transport. Production
// callers must use NewClient's fixed Microsoft Graph endpoint.
func (c *Client) SetHTTPClientForTest(client *http.Client, baseURL string) {
	if client != nil {
		c.httpClient = client
	}
	if strings.TrimSpace(baseURL) != "" {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

type Recipient struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

type Message struct {
	ID               string       `json:"id"`
	ConversationID   string       `json:"conversationId,omitempty"`
	Subject          string       `json:"subject,omitempty"`
	BodyPreview      string       `json:"bodyPreview,omitempty"`
	Body             MessageBody  `json:"body,omitempty"`
	From             EmailParty   `json:"from,omitempty"`
	Sender           EmailParty   `json:"sender,omitempty"`
	ToRecipients     []EmailParty `json:"toRecipients,omitempty"`
	CCRecipients     []EmailParty `json:"ccRecipients,omitempty"`
	BCCRecipients    []EmailParty `json:"bccRecipients,omitempty"`
	ReplyTo          []EmailParty `json:"replyTo,omitempty"`
	ReceivedDateTime string       `json:"receivedDateTime,omitempty"`
	SentDateTime     string       `json:"sentDateTime,omitempty"`
	IsRead           bool         `json:"isRead"`
	IsDraft          bool         `json:"isDraft"`
	HasAttachments   bool         `json:"hasAttachments"`
	Categories       []string     `json:"categories,omitempty"`
	WebLink          string       `json:"webLink,omitempty"`
}

type InternetHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type MessageBody struct {
	ContentType string `json:"contentType,omitempty"`
	Content     string `json:"content,omitempty"`
}

type EmailParty struct {
	EmailAddress Recipient `json:"emailAddress"`
}

type MessageList struct {
	Messages   []Message `json:"messages"`
	HasMore    bool      `json:"hasMore"`
	NextCursor string    `json:"nextCursor,omitempty"`
	NextLink   string    `json:"-"`
}

type graphMessageList struct {
	Value    []Message `json:"value"`
	NextLink string    `json:"@odata.nextLink"`
}

type graphErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type GraphError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *GraphError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = http.StatusText(e.StatusCode)
	}
	if strings.TrimSpace(e.Code) != "" {
		return fmt.Sprintf("Microsoft Graph %s: %s", e.Code, message)
	}
	return fmt.Sprintf("Microsoft Graph HTTP %d: %s", e.StatusCode, message)
}

func (c *Client) ListMessages(ctx context.Context, folder, query string, top int, unreadOnly bool) (MessageList, error) {
	return c.ListMessagesInMailbox(ctx, "", folder, query, top, unreadOnly)
}

func (c *Client) ListMessagesInMailbox(ctx context.Context, mailbox, folder, query string, top int, unreadOnly bool) (MessageList, error) {
	return c.ListMessagesInMailboxPage(ctx, mailbox, folder, query, top, "", unreadOnly)
}

func (c *Client) ListMessagesInMailboxPage(ctx context.Context, mailbox, folder, query string, top int, cursor string, unreadOnly bool) (MessageList, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MessageList{}, err
	}
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = "inbox"
	}
	if top <= 0 {
		top = 20
	}
	if top > 50 {
		top = 50
	}
	resourcePath := root + "/mailFolders/" + url.PathEscape(folder) + "/messages"
	if strings.TrimSpace(cursor) != "" {
		path, cursorErr := c.continuationPath(cursor, resourcePath)
		if cursorErr != nil {
			return MessageList{}, cursorErr
		}
		var response graphMessageList
		if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
			return MessageList{}, err
		}
		return messageListPage(response.Value, response.NextLink), nil
	}
	values := url.Values{}
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,conversationId,subject,bodyPreview,from,sender,toRecipients,ccRecipients,bccRecipients,replyTo,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,categories,webLink")
	values.Set("$orderby", "receivedDateTime DESC")
	if unreadOnly && strings.TrimSpace(query) == "" {
		values.Set("$filter", "isRead eq false")
	}
	if query = strings.TrimSpace(query); query != "" {
		if len(query) > 256 {
			return MessageList{}, fmt.Errorf("search query exceeds 256 characters")
		}
		values.Set("$search", `"`+strings.ReplaceAll(query, `"`, `\"`)+`"`)
		values.Del("$orderby")
	}
	path := resourcePath + "?" + values.Encode()
	var response graphMessageList
	if err := c.do(ctx, http.MethodGet, path, nil, &response); err != nil {
		return MessageList{}, err
	}
	if unreadOnly && strings.TrimSpace(query) != "" {
		filtered := response.Value[:0]
		for _, message := range response.Value {
			if !message.IsRead {
				filtered = append(filtered, message)
			}
		}
		response.Value = filtered
	}
	return messageListPage(response.Value, response.NextLink), nil
}

func (c *Client) SearchMessagesInMailbox(ctx context.Context, mailbox, folder, query string, size int, cursor string) (MessageList, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MessageList{}, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return MessageList{}, fmt.Errorf("query is required; use list_messages or get_recent_emails for recent inbox mail")
	}
	if len(query) > 256 {
		return MessageList{}, fmt.Errorf("search query exceeds 256 characters")
	}
	if size <= 0 {
		size = 20
	}
	if size > 50 {
		size = 50
	}
	path := root + "/messages"
	if folder = strings.TrimSpace(folder); folder != "" {
		path = root + "/mailFolders/" + url.PathEscape(folder) + "/messages"
	}
	if strings.TrimSpace(cursor) != "" {
		continuation, cursorErr := c.continuationPath(cursor, path)
		if cursorErr != nil {
			return MessageList{}, cursorErr
		}
		var response graphMessageList
		if err := c.do(ctx, http.MethodGet, continuation, nil, &response); err != nil {
			return MessageList{}, err
		}
		return messageListPage(response.Value, response.NextLink), nil
	}
	values := url.Values{}
	values.Set("$top", strconv.Itoa(size))
	values.Set("$select", "id,conversationId,subject,bodyPreview,from,sender,toRecipients,ccRecipients,bccRecipients,replyTo,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,categories,webLink")
	values.Set("$search", `"`+strings.ReplaceAll(query, `"`, `\"`)+`"`)
	var response graphMessageList
	if err := c.do(ctx, http.MethodGet, path+"?"+values.Encode(), nil, &response); err != nil {
		return MessageList{}, err
	}
	return messageListPage(response.Value, response.NextLink), nil
}

func messageListPage(messages []Message, nextLink string) MessageList {
	result := MessageList{Messages: messages, HasMore: strings.TrimSpace(nextLink) != "", NextLink: nextLink}
	if result.HasMore {
		result.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(nextLink))
	}
	return result
}

func (c *Client) continuationPath(cursor, expectedResourcePath string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(cursor))
	if err != nil || len(decoded) == 0 || len(decoded) > 16*1024 {
		return "", fmt.Errorf("pagination cursor is invalid")
	}
	next, err := url.Parse(string(decoded))
	if err != nil || next.Scheme == "" || next.Host == "" || next.User != nil || next.Fragment != "" {
		return "", fmt.Errorf("pagination cursor is invalid")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || !strings.EqualFold(next.Scheme, base.Scheme) || !strings.EqualFold(next.Host, base.Host) {
		return "", fmt.Errorf("pagination cursor origin is invalid")
	}
	path := next.EscapedPath()
	basePath := strings.TrimSuffix(base.EscapedPath(), "/")
	if basePath != "" {
		if !strings.HasPrefix(path, basePath+"/") {
			return "", fmt.Errorf("pagination cursor path is invalid")
		}
		path = strings.TrimPrefix(path, basePath)
	}
	decodedPath, pathErr := url.PathUnescape(path)
	decodedExpected, expectedErr := url.PathUnescape(expectedResourcePath)
	if pathErr != nil || expectedErr != nil || canonicalGraphResourcePath(decodedPath) != canonicalGraphResourcePath(decodedExpected) {
		return "", fmt.Errorf("pagination cursor does not belong to this mailbox resource")
	}
	if next.RawQuery == "" {
		return "", fmt.Errorf("pagination cursor query is missing")
	}
	return path + "?" + next.RawQuery, nil
}

var graphODataKeySegment = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)\('((?:[^']|'')*)'\)$`)

// Microsoft Graph can canonicalize a collection item from slash notation
// (`/mailFolders/inbox`) to OData key notation (`/mailFolders('inbox')`) in
// @odata.nextLink. Both forms identify the same resource. Normalize only this
// narrowly defined syntax before comparing the cursor with the originating
// mailbox resource; origin, mailbox and full remaining path still have to
// match exactly.
func canonicalGraphResourcePath(path string) string {
	parts := strings.Split(strings.TrimSpace(path), "/")
	canonical := make([]string, 0, len(parts)+2)
	for _, part := range parts {
		matches := graphODataKeySegment.FindStringSubmatch(part)
		if len(matches) != 3 {
			canonical = append(canonical, part)
			continue
		}
		canonical = append(canonical, matches[1], strings.ReplaceAll(matches[2], "''", "'"))
	}
	return strings.Join(canonical, "/")
}

func (c *Client) GetMessage(ctx context.Context, messageID string) (Message, error) {
	return c.GetMessageInMailbox(ctx, "", messageID)
}

func (c *Client) GetMessageInMailbox(ctx context.Context, mailbox, messageID string) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	values := url.Values{}
	values.Set("$select", "id,conversationId,subject,body,bodyPreview,from,sender,toRecipients,ccRecipients,bccRecipients,replyTo,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,categories,webLink")
	var message Message
	if err := c.do(ctx, http.MethodGet, root+"/messages/"+url.PathEscape(messageID)+"?"+values.Encode(), nil, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

type MessageBatchFailure struct {
	MessageID  string `json:"messageId"`
	StatusCode int    `json:"statusCode"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	RetryAfter string `json:"retryAfter,omitempty"`
}

type MessageBatch struct {
	Messages []Message             `json:"messages"`
	Failures []MessageBatchFailure `json:"failures,omitempty"`
}

type graphBatchRequest struct {
	Requests []graphBatchRequestItem `json:"requests"`
}

type graphBatchRequestItem struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	URL    string `json:"url"`
}

type graphBatchResponse struct {
	Responses []graphBatchResponseItem `json:"responses"`
}

type graphBatchResponseItem struct {
	ID      string            `json:"id"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func (c *Client) GetMessagesBatchInMailbox(ctx context.Context, mailbox string, messageIDs []string) (MessageBatch, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return MessageBatch{}, err
	}
	if len(messageIDs) == 0 {
		return MessageBatch{}, fmt.Errorf("at least one messageId is required")
	}
	if len(messageIDs) > 20 {
		return MessageBatch{}, fmt.Errorf("messageIds cannot contain more than 20 items")
	}
	requests := make([]graphBatchRequestItem, 0, len(messageIDs))
	requestToMessage := make(map[string]string, len(messageIDs))
	seen := make(map[string]struct{}, len(messageIDs))
	values := url.Values{}
	values.Set("$select", "id,conversationId,subject,body,bodyPreview,from,sender,toRecipients,ccRecipients,bccRecipients,replyTo,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,categories,webLink")
	for index, rawID := range messageIDs {
		messageID, idErr := requiredIdentifier("messageId", rawID)
		if idErr != nil {
			return MessageBatch{}, idErr
		}
		if _, duplicate := seen[messageID]; duplicate {
			return MessageBatch{}, fmt.Errorf("messageIds must be unique")
		}
		seen[messageID] = struct{}{}
		requestID := strconv.Itoa(index + 1)
		requestToMessage[requestID] = messageID
		requests = append(requests, graphBatchRequestItem{
			ID: requestID, Method: http.MethodGet,
			URL: root + "/messages/" + url.PathEscape(messageID) + "?" + values.Encode(),
		})
	}
	var response graphBatchResponse
	if err := c.do(ctx, http.MethodPost, "/$batch", graphBatchRequest{Requests: requests}, &response); err != nil {
		return MessageBatch{}, err
	}
	byRequestID := make(map[string]graphBatchResponseItem, len(response.Responses))
	for _, item := range response.Responses {
		byRequestID[item.ID] = item
	}
	result := MessageBatch{Messages: make([]Message, 0, len(messageIDs))}
	for _, request := range requests {
		item, ok := byRequestID[request.ID]
		messageID := requestToMessage[request.ID]
		if !ok {
			result.Failures = append(result.Failures, MessageBatchFailure{MessageID: messageID, Message: "Microsoft Graph batch response omitted this message"})
			continue
		}
		if item.Status == http.StatusUnauthorized {
			_ = c.tokens.MarkNeedsReauth(ConnectorID)
			if c.onNeedsAuth != nil {
				_ = c.onNeedsAuth(ServerName, "Microsoft Outlook authorization expired")
			}
			return MessageBatch{}, ErrNeedsAuth
		}
		if item.Status >= 200 && item.Status < 300 {
			var message Message
			if err := json.Unmarshal(item.Body, &message); err != nil {
				result.Failures = append(result.Failures, MessageBatchFailure{MessageID: messageID, StatusCode: item.Status, Message: "decode Microsoft Graph batch item"})
				continue
			}
			result.Messages = append(result.Messages, message)
			continue
		}
		var envelope graphErrorEnvelope
		_ = json.Unmarshal(item.Body, &envelope)
		result.Failures = append(result.Failures, MessageBatchFailure{
			MessageID: messageID, StatusCode: item.Status,
			Code: safeGraphText(envelope.Error.Code, 120), Message: safeGraphText(envelope.Error.Message, 512),
			RetryAfter: batchHeader(item.Headers, "Retry-After"),
		})
	}
	return result, nil
}

func batchHeader(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return safeGraphText(value, 120)
		}
	}
	return ""
}

type graphMessageWithHeaders struct {
	Message
	InternetHeaders []InternetHeader `json:"internetMessageHeaders,omitempty"`
}

func (c *Client) getMessageWithHeaders(ctx context.Context, mailbox, messageID string) (graphMessageWithHeaders, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return graphMessageWithHeaders{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return graphMessageWithHeaders{}, err
	}
	values := url.Values{}
	values.Set("$select", "id,subject,from,receivedDateTime,internetMessageHeaders")
	var message graphMessageWithHeaders
	if err := c.do(ctx, http.MethodGet, root+"/messages/"+url.PathEscape(messageID)+"?"+values.Encode(), nil, &message); err != nil {
		return graphMessageWithHeaders{}, err
	}
	return message, nil
}

type ComposeMessage struct {
	Subject         string
	Body            string
	To              []Recipient
	CC              []Recipient
	BCC             []Recipient
	From            *Recipient
	AttachmentPaths []string
}

func (c *Client) CreateDraft(ctx context.Context, input ComposeMessage) (Message, error) {
	return c.CreateDraftInMailbox(ctx, "", input)
}

func (c *Client) CreateDraftInMailbox(ctx context.Context, mailbox string, input ComposeMessage) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	if input.From == nil && strings.TrimSpace(mailbox) != "" {
		from := Recipient{Address: strings.TrimSpace(mailbox)}
		input.From = &from
	}
	payload, err := composePayload(input)
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := c.do(ctx, http.MethodPost, root+"/messages", payload, &message); err != nil {
		return Message{}, err
	}
	if err := c.AddAttachments(ctx, mailbox, message.ID, input.AttachmentPaths); err != nil {
		return Message{}, fmt.Errorf("draft %s was created but attachments could not be added: %w", message.ID, err)
	}
	return message, nil
}

func (c *Client) SendMail(ctx context.Context, input ComposeMessage) error {
	return c.SendMailFromMailbox(ctx, "", input)
}

func (c *Client) SendMailFromMailbox(ctx context.Context, mailbox string, input ComposeMessage) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	if input.From == nil && strings.TrimSpace(mailbox) != "" {
		from := Recipient{Address: strings.TrimSpace(mailbox)}
		input.From = &from
	}
	if len(input.AttachmentPaths) > 0 {
		draft, err := c.CreateDraftInMailbox(ctx, mailbox, input)
		if err != nil {
			return err
		}
		if err := c.SendDraft(ctx, mailbox, draft.ID); err != nil {
			return fmt.Errorf("draft %s contains the prepared message but could not be sent: %w", draft.ID, err)
		}
		return nil
	}
	payload, err := composePayload(input)
	if err != nil {
		return err
	}
	body := map[string]any{"message": payload, "saveToSentItems": true}
	return c.do(ctx, http.MethodPost, root+"/sendMail", body, nil)
}

func (c *Client) ReplyMessage(ctx context.Context, messageID, comment string) error {
	return c.ReplyMessageInMailbox(ctx, "", messageID, comment, false)
}

func (c *Client) ReplyMessageInMailbox(ctx context.Context, mailbox, messageID, comment string, replyAll bool) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	comment = strings.TrimSpace(comment)
	if comment == "" {
		return fmt.Errorf("comment is required")
	}
	if len(comment) > 100_000 {
		return fmt.Errorf("comment exceeds 100000 characters")
	}
	action := "reply"
	if replyAll {
		action = "replyAll"
	}
	return c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/"+action, map[string]string{"comment": comment}, nil)
}

func (c *Client) UpdateMessage(ctx context.Context, messageID string, isRead *bool, categories []string) (Message, error) {
	return c.UpdateMessageInMailbox(ctx, "", messageID, isRead, categories)
}

func (c *Client) UpdateMessageInMailbox(ctx context.Context, mailbox, messageID string, isRead *bool, categories []string) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	payload := map[string]any{}
	if isRead != nil {
		payload["isRead"] = *isRead
	}
	if categories != nil {
		if len(categories) > 25 {
			return Message{}, fmt.Errorf("categories cannot contain more than 25 values")
		}
		cleaned := make([]string, 0, len(categories))
		for _, category := range categories {
			category = strings.TrimSpace(category)
			if category == "" || len(category) > 255 {
				return Message{}, fmt.Errorf("category names must contain 1 to 255 characters")
			}
			cleaned = append(cleaned, category)
		}
		payload["categories"] = cleaned
	}
	if len(payload) == 0 {
		return Message{}, fmt.Errorf("isRead or categories is required")
	}
	var message Message
	if err := c.do(ctx, http.MethodPatch, root+"/messages/"+url.PathEscape(messageID), payload, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) MoveMessage(ctx context.Context, messageID, destinationID string) (Message, error) {
	return c.MoveMessageInMailbox(ctx, "", messageID, destinationID)
}

func (c *Client) MoveMessageInMailbox(ctx context.Context, mailbox, messageID, destinationID string) (Message, error) {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return Message{}, err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	destinationID, err = requiredIdentifier("destinationId", destinationID)
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := c.do(ctx, http.MethodPost, root+"/messages/"+url.PathEscape(messageID)+"/move", map[string]string{"destinationId": destinationID}, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	return c.DeleteMessageInMailbox(ctx, "", messageID)
}

func (c *Client) DeleteMessageInMailbox(ctx context.Context, mailbox, messageID string) error {
	root, err := mailboxRoot(mailbox)
	if err != nil {
		return err
	}
	messageID, err = requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, root+"/messages/"+url.PathEscape(messageID), nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, output any) error {
	if c == nil || c.tokens == nil {
		return fmt.Errorf("outlook connector auth broker is unavailable")
	}
	token, err := c.tokens.AccessToken(ctx, ConnectorID)
	if err != nil {
		return outlookAccessTokenError(err)
	}
	if strings.TrimSpace(token) == "" {
		return ErrNeedsAuth
	}
	var reader io.Reader
	if body != nil {
		data, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return fmt.Errorf("encode Graph request: %w", marshalErr)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.baseURL, "/")+path, reader)
	if err != nil {
		return fmt.Errorf("build Graph request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.performRequest(request)
	if err != nil {
		return fmt.Errorf("call Microsoft Graph: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxGraphResponse+1))
	if err != nil {
		return fmt.Errorf("read Microsoft Graph response: %w", err)
	}
	if len(responseBody) > maxGraphResponse {
		return fmt.Errorf("Microsoft Graph response exceeds %d bytes", maxGraphResponse)
	}
	if response.StatusCode == http.StatusUnauthorized {
		_ = c.tokens.MarkNeedsReauth(ConnectorID)
		if c.onNeedsAuth != nil {
			_ = c.onNeedsAuth(ServerName, "Microsoft Outlook authorization expired")
		}
		return ErrNeedsAuth
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var graphError graphErrorEnvelope
		_ = json.Unmarshal(responseBody, &graphError)
		return &GraphError{StatusCode: response.StatusCode, Code: safeGraphText(graphError.Error.Code, 120), Message: safeGraphText(graphError.Error.Message, 512)}
	}
	if output == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		return fmt.Errorf("decode Microsoft Graph response: %w", err)
	}
	return nil
}

func outlookAccessTokenError(err error) error {
	if errors.Is(err, connectorauth.ErrNeedsAuth) || errors.Is(err, connectorauth.ErrCredentialChanged) {
		return fmt.Errorf("%w: %v", ErrNeedsAuth, err)
	}
	return fmt.Errorf("resolve Outlook access token: %w", err)
}

func (c *Client) performRequest(request *http.Request) (*http.Response, error) {
	response, err := c.httpClient.Do(request)
	if err == nil {
		return response, nil
	}
	proxyFailed := networkroute.ReportDefaultProxyFailure(request)
	if !proxyFailed || !safeGraphRetryMethod(request.Method) || request.Body != nil {
		return nil, err
	}
	// A loopback proxy can keep accepting TCP while its CONNECT/TLS path is
	// broken. GET/HEAD/OPTIONS have no external side effect, so auto policy may
	// retry once through the newly selected direct route. Writes remain
	// do-not-retry because their remote result can be unknown.
	c.httpClient.CloseIdleConnections()
	retry := request.Clone(request.Context())
	return c.httpClient.Do(retry)
}

func safeGraphRetryMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func safeGraphText(value string, limit int) string {
	value = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value))
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func composePayload(input ComposeMessage) (map[string]any, error) {
	input.Subject = strings.TrimSpace(input.Subject)
	if len(input.Subject) > 998 {
		return nil, fmt.Errorf("subject exceeds 998 characters")
	}
	if len(input.Body) > 100_000 {
		return nil, fmt.Errorf("body exceeds 100000 characters")
	}
	if len(input.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	to, err := recipientPayload(input.To)
	if err != nil {
		return nil, err
	}
	cc, err := recipientPayload(input.CC)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"subject":      input.Subject,
		"body":         map[string]string{"contentType": "Text", "content": input.Body},
		"toRecipients": to,
	}
	if len(cc) > 0 {
		payload["ccRecipients"] = cc
	}
	bcc, err := recipientPayload(input.BCC)
	if err != nil {
		return nil, err
	}
	if len(bcc) > 0 {
		payload["bccRecipients"] = bcc
	}
	if input.From != nil {
		from, err := recipientPayload([]Recipient{*input.From})
		if err != nil {
			return nil, err
		}
		payload["from"] = from[0]
	}
	return payload, nil
}

func mailboxRoot(mailbox string) (string, error) {
	mailbox = strings.TrimSpace(mailbox)
	if mailbox == "" {
		return "/me", nil
	}
	mailbox, err := requiredIdentifier("mailbox", mailbox)
	if err != nil {
		return "", err
	}
	if !strings.Contains(mailbox, "@") || strings.ContainsAny(mailbox, " /\\?#") {
		return "", fmt.Errorf("mailbox must be an exact email address or user principal name")
	}
	return "/users/" + url.PathEscape(mailbox), nil
}

func recipientPayload(values []Recipient) ([]map[string]any, error) {
	if len(values) > 100 {
		return nil, fmt.Errorf("recipient list cannot contain more than 100 addresses")
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		value.Address = strings.TrimSpace(value.Address)
		value.Name = strings.TrimSpace(value.Name)
		if value.Address == "" || len(value.Address) > 320 || strings.ContainsAny(value.Address, "\r\n") {
			return nil, fmt.Errorf("recipient address is invalid")
		}
		result = append(result, map[string]any{"emailAddress": map[string]string{"address": value.Address, "name": value.Name}})
	}
	return result, nil
}

func requiredIdentifier(name, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if len(value) > 1024 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%s is invalid", name)
	}
	return value, nil
}
