package outlook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	Subject          string       `json:"subject,omitempty"`
	BodyPreview      string       `json:"bodyPreview,omitempty"`
	Body             MessageBody  `json:"body,omitempty"`
	From             EmailParty   `json:"from,omitempty"`
	ToRecipients     []EmailParty `json:"toRecipients,omitempty"`
	CCRecipients     []EmailParty `json:"ccRecipients,omitempty"`
	ReceivedDateTime string       `json:"receivedDateTime,omitempty"`
	SentDateTime     string       `json:"sentDateTime,omitempty"`
	IsRead           bool         `json:"isRead"`
	Categories       []string     `json:"categories,omitempty"`
	WebLink          string       `json:"webLink,omitempty"`
}

type MessageBody struct {
	ContentType string `json:"contentType,omitempty"`
	Content     string `json:"content,omitempty"`
}

type EmailParty struct {
	EmailAddress Recipient `json:"emailAddress"`
}

type MessageList struct {
	Messages []Message `json:"messages"`
	NextLink string    `json:"nextLink,omitempty"`
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
	values := url.Values{}
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,subject,bodyPreview,from,toRecipients,ccRecipients,receivedDateTime,sentDateTime,isRead,categories,webLink")
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
	path := "/me/mailFolders/" + url.PathEscape(folder) + "/messages?" + values.Encode()
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
	return MessageList{Messages: response.Value, NextLink: response.NextLink}, nil
}

func (c *Client) GetMessage(ctx context.Context, messageID string) (Message, error) {
	messageID, err := requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	values := url.Values{}
	values.Set("$select", "id,subject,body,bodyPreview,from,toRecipients,ccRecipients,receivedDateTime,sentDateTime,isRead,categories,webLink")
	var message Message
	if err := c.do(ctx, http.MethodGet, "/me/messages/"+url.PathEscape(messageID)+"?"+values.Encode(), nil, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

type ComposeMessage struct {
	Subject string
	Body    string
	To      []Recipient
	CC      []Recipient
}

func (c *Client) CreateDraft(ctx context.Context, input ComposeMessage) (Message, error) {
	payload, err := composePayload(input)
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := c.do(ctx, http.MethodPost, "/me/messages", payload, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) SendMail(ctx context.Context, input ComposeMessage) error {
	payload, err := composePayload(input)
	if err != nil {
		return err
	}
	body := map[string]any{"message": payload, "saveToSentItems": true}
	return c.do(ctx, http.MethodPost, "/me/sendMail", body, nil)
}

func (c *Client) ReplyMessage(ctx context.Context, messageID, comment string) error {
	messageID, err := requiredIdentifier("messageId", messageID)
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
	return c.do(ctx, http.MethodPost, "/me/messages/"+url.PathEscape(messageID)+"/reply", map[string]string{"comment": comment}, nil)
}

func (c *Client) UpdateMessage(ctx context.Context, messageID string, isRead *bool, categories []string) (Message, error) {
	messageID, err := requiredIdentifier("messageId", messageID)
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
	if err := c.do(ctx, http.MethodPatch, "/me/messages/"+url.PathEscape(messageID), payload, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) MoveMessage(ctx context.Context, messageID, destinationID string) (Message, error) {
	messageID, err := requiredIdentifier("messageId", messageID)
	if err != nil {
		return Message{}, err
	}
	destinationID, err = requiredIdentifier("destinationId", destinationID)
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := c.do(ctx, http.MethodPost, "/me/messages/"+url.PathEscape(messageID)+"/move", map[string]string{"destinationId": destinationID}, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	messageID, err := requiredIdentifier("messageId", messageID)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, "/me/messages/"+url.PathEscape(messageID), nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, output any) error {
	if c == nil || c.tokens == nil {
		return fmt.Errorf("outlook connector auth broker is unavailable")
	}
	token, err := c.tokens.AccessToken(ctx, ConnectorID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNeedsAuth, err)
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
	response, err := c.httpClient.Do(request)
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
	return payload, nil
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
