package outlook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientExpandedMailboxDraftAttachmentAndOrganizationActions(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var graph *httptest.Server
	graph = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access-token" && request.URL.Path != "/unsubscribe" {
			t.Errorf("authorization header = %q for %s", request.Header.Get("Authorization"), request.URL.Path)
		}
		mu.Lock()
		calls = append(calls, request.Method+" "+request.URL.EscapedPath())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/unsubscribe":
			if request.Method != http.MethodPost {
				t.Errorf("unsubscribe method = %s", request.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(request.URL.Path, "/attachments/a1/$value"):
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("attachment body"))
		case strings.HasSuffix(request.URL.Path, "/attachments") && request.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"value":[{"id":"a1","name":"notes.txt","size":15}]}`))
		case strings.HasSuffix(request.URL.Path, "/attachments") && request.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"a2","name":"upload.txt","size":6}`))
		case strings.Contains(request.URL.Path, "/outlook/masterCategories") && request.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"value":[{"id":"c1","displayName":"客户","color":"preset0"}]}`))
		case strings.Contains(request.URL.Path, "/outlook/masterCategories") && request.Method == http.MethodPatch:
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if _, present := payload["displayName"]; present {
				t.Fatalf("category update must not send read-only displayName: %+v", payload)
			}
			_, _ = w.Write([]byte(`{"id":"c1","displayName":"客户","color":"preset15"}`))
		case strings.Contains(request.URL.Path, "/outlook/masterCategories") && request.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"c1","displayName":"客户","color":"preset0"}`))
		case strings.Contains(request.URL.Path, "/mailFolders") && request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"value":[{"id":"m1","subject":"Shared"}]}`))
		case strings.Contains(request.URL.Path, "/mailFolders") && request.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"value":[{"id":"f1","displayName":"Archive"}]}`))
		case strings.Contains(request.URL.Path, "/mailFolders") && request.Method != http.MethodDelete:
			_, _ = w.Write([]byte(`{"id":"f1","displayName":"Archive"}`))
		case strings.HasSuffix(request.URL.Path, "/messages/m1") && request.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"m1","subject":"Newsletter","internetMessageHeaders":[{"name":"List-Unsubscribe","value":"<https://%s/unsubscribe?token=do-not-return>"},{"name":"List-Unsubscribe-Post","value":"List-Unsubscribe=One-Click"}]}`, request.Host)))
		case strings.HasSuffix(request.URL.Path, "/createReplyAll") || strings.HasSuffix(request.URL.Path, "/createForward"):
			_, _ = w.Write([]byte(`{"id":"draft-reply","isDraft":true}`))
		case strings.HasSuffix(request.URL.Path, "/send") || strings.HasSuffix(request.URL.Path, "/replyAll") || strings.HasSuffix(request.URL.Path, "/forward"):
			w.WriteHeader(http.StatusAccepted)
		case request.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"id":"draft","isDraft":true}`))
		}
	}))
	defer graph.Close()

	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	ctx := context.Background()
	mailbox := "support@example.com"

	if list, err := client.ListMessagesInMailbox(ctx, mailbox, "inbox", "", 10, false); err != nil || len(list.Messages) != 1 {
		t.Fatalf("shared list = %+v err=%v", list, err)
	}
	if folders, err := client.ListMailFolders(ctx, mailbox, "", 10, false); err != nil || len(folders.Folders) != 1 {
		t.Fatalf("folders = %+v err=%v", folders, err)
	}
	if _, err := client.CreateMailFolder(ctx, mailbox, "", "Archive", false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateMailFolder(ctx, mailbox, "f1", "Archived"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteMailFolder(ctx, mailbox, "f1"); err != nil {
		t.Fatal(err)
	}
	if categories, err := client.ListCategories(ctx); err != nil || len(categories.Categories) != 1 {
		t.Fatalf("categories = %+v err=%v", categories, err)
	}
	category, err := client.CreateCategory(ctx, "客户", "preset0")
	if err != nil || category.ID != "c1" {
		t.Fatalf("category = %+v err=%v", category, err)
	}
	if category, err := client.UpdateCategory(ctx, "c1", "preset15"); err != nil || category.Color != "preset15" {
		t.Fatal(err)
	}
	if err := client.DeleteCategory(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if attachments, err := client.ListAttachments(ctx, mailbox, "m1"); err != nil || len(attachments.Attachments) != 1 {
		t.Fatalf("attachments = %+v err=%v", attachments, err)
	}
	downloadPath := filepath.Join(t.TempDir(), "notes.txt")
	download, err := client.DownloadAttachment(ctx, mailbox, "m1", "a1", downloadPath)
	if err != nil || download.Bytes != 15 {
		t.Fatalf("download = %+v err=%v", download, err)
	}
	if data, err := os.ReadFile(downloadPath); err != nil || string(data) != "attachment body" {
		t.Fatalf("downloaded data = %q err=%v", data, err)
	}
	uploadPath := filepath.Join(t.TempDir(), "upload.txt")
	if err := os.WriteFile(uploadPath, []byte("upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AddAttachment(ctx, mailbox, "m1", uploadPath); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteAttachment(ctx, mailbox, "m1", "a1"); err != nil {
		t.Fatal(err)
	}
	body := "updated"
	if _, err := client.UpdateDraft(ctx, mailbox, "draft", DraftUpdate{Body: &body}); err != nil {
		t.Fatal(err)
	}
	if err := client.SendDraft(ctx, mailbox, "draft"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateReplyDraft(ctx, mailbox, "m1", "reply", true, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.ReplyMessageInMailbox(ctx, mailbox, "m1", "reply", true); err != nil {
		t.Fatal(err)
	}
	to := []Recipient{{Address: "person@example.com"}}
	if _, err := client.CreateForwardDraft(ctx, mailbox, "m1", "forward", to, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.ForwardMessage(ctx, mailbox, "m1", "forward", to); err != nil {
		t.Fatal(err)
	}
	message, err := client.GetMessageInMailbox(ctx, mailbox, "m1")
	if err != nil {
		t.Fatal(err)
	}
	publicMessage, _ := json.Marshal(message)
	if strings.Contains(string(publicMessage), "List-Unsubscribe") || strings.Contains(string(publicMessage), "do-not-return") {
		t.Fatalf("ordinary message result leaked unsubscribe material: %s", publicMessage)
	}
	info, err := client.GetUnsubscribeInfo(ctx, mailbox, "m1")
	if err != nil || !info.OneClick || info.HTTPSHost == "" || strings.Contains(fmt.Sprintf("%+v", info), "do-not-return") {
		t.Fatalf("unsubscribe info = %+v err=%v", info, err)
	}
	result, err := client.UnsubscribeMessage(ctx, mailbox, "m1", "one_click")
	if err != nil || !result.Success || result.Method != "one_click" {
		t.Fatalf("unsubscribe = %+v err=%v", result, err)
	}

	mu.Lock()
	joined := strings.Join(calls, "\n")
	mu.Unlock()
	for _, want := range []string{
		"GET /users/support@example.com/mailFolders/inbox/messages",
		"GET /users/support@example.com/mailFolders",
		"POST /users/support@example.com/messages/m1/createReplyAll",
		"POST /users/support@example.com/messages/m1/replyAll",
		"POST /users/support@example.com/messages/m1/forward",
		"POST /unsubscribe",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("calls do not contain %q:\n%s", want, joined)
		}
	}
}

func TestClientSearchBatchAndFolderResolutionUseStableModelFacingResults(t *testing.T) {
	searchCalls := 0
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/users/shared@example.com/messages":
			searchCalls++
			if got := request.URL.Query().Get("$search"); got != `"quarterly review"` {
				t.Errorf("search = %q", got)
			}
			if searchCalls == 1 {
				if got := request.URL.Query().Get("$skip"); got != "" {
					t.Errorf("first-page skip = %q", got)
				}
				next := "http://" + request.Host + "/users/shared@example.com/messages?%24search=%22quarterly+review%22&%24skip=37&%24top=1"
				_, _ = w.Write([]byte(fmt.Sprintf(`{"value":[{"id":"m1","subject":"Quarterly review"}],"@odata.nextLink":%q}`, next)))
				break
			}
			if got := request.URL.Query().Get("$skip"); got != "37" {
				t.Errorf("continuation skip = %q", got)
			}
			_, _ = w.Write([]byte(`{"value":[{"id":"m2","subject":"Quarterly review continued"}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/$batch":
			var batch graphBatchRequest
			if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
				t.Fatal(err)
			}
			if len(batch.Requests) != 2 || !strings.Contains(batch.Requests[0].URL, "/users/shared@example.com/messages/m1") {
				t.Fatalf("batch requests = %+v", batch.Requests)
			}
			_, _ = w.Write([]byte(`{"responses":[{"id":"1","status":200,"body":{"id":"m1","subject":"Full message","body":{"contentType":"text","content":"body"}}},{"id":"2","status":429,"headers":{"Retry-After":"7"},"body":{"error":{"code":"TooManyRequests","message":"try later"}}}]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/users/shared@example.com/mailFolders":
			_, _ = w.Write([]byte(`{"value":[{"id":"projects","displayName":"Projects","childFolderCount":1},{"id":"archive","displayName":"Archive","childFolderCount":0}]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/users/shared@example.com/mailFolders/projects/childFolders":
			_, _ = w.Write([]byte(`{"value":[{"id":"finance","displayName":"Finance","parentFolderId":"projects","childFolderCount":0}]}`))
		default:
			t.Fatalf("unexpected Graph request %s %s", request.Method, request.URL.String())
		}
	}))
	defer graph.Close()

	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	ctx := context.Background()
	search, err := client.SearchMessagesInMailbox(ctx, "shared@example.com", "", "quarterly review", 1, "")
	if err != nil || len(search.Messages) != 1 || !search.HasMore || search.NextCursor == "" || search.NextLink == "" {
		t.Fatalf("search = %+v err=%v", search, err)
	}
	publicSearch, err := json.Marshal(search)
	if err != nil || strings.Contains(string(publicSearch), "@odata.nextLink") || strings.Contains(string(publicSearch), "graph.microsoft.com") || !strings.Contains(string(publicSearch), "nextCursor") {
		t.Fatalf("model-facing search page = %s err=%v", publicSearch, err)
	}
	continued, err := client.SearchMessagesInMailbox(ctx, "shared@example.com", "", "quarterly review", 1, search.NextCursor)
	if err != nil || len(continued.Messages) != 1 || continued.Messages[0].ID != "m2" || continued.HasMore {
		t.Fatalf("continued search = %+v err=%v", continued, err)
	}
	batch, err := client.GetMessagesBatchInMailbox(ctx, "shared@example.com", []string{"m1", "m2"})
	if err != nil || len(batch.Messages) != 1 || len(batch.Failures) != 1 || batch.Failures[0].RetryAfter != "7" {
		t.Fatalf("batch = %+v err=%v", batch, err)
	}
	folders, err := client.FindMailFolders(ctx, "shared@example.com", `Projects\Finance`, 5, false)
	if err != nil || len(folders.Matches) != 1 || folders.Matches[0].ID != "finance" || folders.Matches[0].Path != "Projects/Finance" || folders.Matches[0].MatchKind != "exact_path" {
		t.Fatalf("folders = %+v err=%v", folders, err)
	}
}

func TestCreateReplyDraftMergesAdditionalCCWithoutDuplicates(t *testing.T) {
	var patched []EmailParty
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/createReply"):
			_, _ = w.Write([]byte(`{"id":"draft-1","isDraft":true,"ccRecipients":[{"emailAddress":{"address":"existing@example.com"}}]}`))
		case request.Method == http.MethodPatch && strings.HasSuffix(request.URL.Path, "/messages/draft-1"):
			var payload struct {
				CC []EmailParty `json:"ccRecipients"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			patched = payload.CC
			_, _ = w.Write([]byte(`{"id":"draft-1","isDraft":true}`))
		default:
			t.Fatalf("unexpected Graph request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer graph.Close()

	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	_, err := client.CreateReplyDraft(context.Background(), "", "source-1", "reply", false, []Recipient{
		{Address: "existing@example.com"}, {Address: "new@example.com"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(patched) != 2 || patched[0].EmailAddress.Address != "existing@example.com" || patched[1].EmailAddress.Address != "new@example.com" {
		t.Fatalf("patched CC = %+v", patched)
	}
}

func TestFindMailFoldersResolvesLocaleIndependentWellKnownName(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/me/mailFolders/sentitems" {
			t.Fatalf("unexpected Graph request %s %s", request.Method, request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sent-real-id","displayName":"已发送邮件"}`))
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	result, err := client.FindMailFolders(context.Background(), "", "SentItems", 10, false)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].ID != "sent-real-id" || result.Matches[0].MatchKind != "well_known" {
		t.Fatalf("well-known result=%+v err=%v", result, err)
	}
}

func TestPaginationCursorRejectsCrossOriginAndDifferentMailbox(t *testing.T) {
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(http.DefaultClient, "https://graph.microsoft.com/v1.0")
	for _, raw := range []string{
		"https://evil.example/v1.0/me/messages?%24skip=20",
		"https://graph.microsoft.com/v1.0/users/other@example.com/messages?%24skip=20",
	} {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := client.continuationPath(cursor, "/me/messages"); err == nil {
			t.Fatalf("cursor %q should be rejected", raw)
		}
	}
	valid := base64.RawURLEncoding.EncodeToString([]byte("https://graph.microsoft.com/v1.0/me/messages?%24skip=37&%24top=20"))
	path, err := client.continuationPath(valid, "/me/messages")
	if err != nil || !strings.Contains(path, "%24skip=37") {
		t.Fatalf("valid cursor path=%q err=%v", path, err)
	}
	realGraphShape := base64.RawURLEncoding.EncodeToString([]byte("https://graph.microsoft.com/v1.0/me/mailFolders('inbox')/messages?%24skip=1&%24top=1"))
	path, err = client.continuationPath(realGraphShape, "/me/mailFolders/inbox/messages")
	if err != nil || !strings.Contains(path, "mailFolders('inbox')/messages") {
		t.Fatalf("Graph OData cursor path=%q err=%v", path, err)
	}
	sharedGraphShape := base64.RawURLEncoding.EncodeToString([]byte("https://graph.microsoft.com/v1.0/users('shared@example.com')/mailFolders('inbox')/messages?%24skip=1&%24top=1"))
	path, err = client.continuationPath(sharedGraphShape, "/users/shared@example.com/mailFolders/inbox/messages")
	if err != nil || !strings.Contains(path, "users('shared@example.com')") {
		t.Fatalf("shared Graph OData cursor path=%q err=%v", path, err)
	}
	crossFolder := base64.RawURLEncoding.EncodeToString([]byte("https://graph.microsoft.com/v1.0/me/mailFolders('sentitems')/messages?%24skip=1"))
	if _, err := client.continuationPath(crossFolder, "/me/mailFolders/inbox/messages"); err == nil {
		t.Fatal("cursor for a different folder must be rejected")
	}
}

func TestListMessagesContinuesRealGraphODataNextLink(t *testing.T) {
	call := 0
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			if request.URL.Path != "/me/mailFolders/inbox/messages" {
				t.Fatalf("first path = %q", request.URL.Path)
			}
			next := "http://" + request.Host + "/me/mailFolders('inbox')/messages?%24skip=1&%24top=1"
			_, _ = w.Write([]byte(fmt.Sprintf(`{"value":[{"id":"m1"}],"@odata.nextLink":%q}`, next)))
		case 2:
			if request.URL.Path != "/me/mailFolders('inbox')/messages" || request.URL.Query().Get("$skip") != "1" {
				t.Fatalf("continuation = %s", request.URL.String())
			}
			_, _ = w.Write([]byte(`{"value":[{"id":"m2"}]}`))
		default:
			t.Fatalf("unexpected request %d: %s", call, request.URL.String())
		}
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	first, err := client.ListMessagesInMailboxPage(context.Background(), "", "inbox", "", 1, "", false)
	if err != nil || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	second, err := client.ListMessagesInMailboxPage(context.Background(), "", "inbox", "", 1, first.NextCursor, false)
	if err != nil || len(second.Messages) != 1 || second.Messages[0].ID != "m2" || second.HasMore {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
}

func TestUpdateCategoryRejectsGraphResponseThatDidNotPersistColor(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","displayName":"客户","color":"preset0"}`))
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	if _, err := client.UpdateCategory(context.Background(), "c1", "preset15"); err == nil || !strings.Contains(err.Error(), "did not persist") {
		t.Fatalf("expected truthful persistence error, got %v", err)
	}
}

func TestMailSchedulerPersistsOnlyDraftIdentityAndProcessesDueMail(t *testing.T) {
	var sendCalls int
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/send") {
			sendCalls++
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"draft-1","isDraft":true}`))
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	scheduler := NewMailScheduler(client, t.TempDir())
	fixed := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return fixed }
	client.scheduler = scheduler
	job, err := scheduler.ScheduleMail(context.Background(), "", fixed.Add(time.Minute).Format(time.RFC3339), ComposeMessage{
		Subject: "sensitive subject", Body: "sensitive body", To: []Recipient{{Address: "person@example.com"}},
	})
	if err != nil || job.DraftID != "draft-1" {
		t.Fatalf("job = %+v err=%v", job, err)
	}
	data, err := os.ReadFile(scheduler.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"sensitive subject", "sensitive body", "person@example.com"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("scheduled store leaked %q: %s", forbidden, data)
		}
	}
	scheduler.now = func() time.Time { return fixed.Add(2 * time.Minute) }
	scheduler.processDue()
	if sendCalls != 1 {
		t.Fatalf("send calls = %d", sendCalls)
	}
	list, err := scheduler.List()
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("list after send = %+v err=%v", list, err)
	}
}

func TestMailSchedulerRetriesOnlyExplicitAuthenticationFailure(t *testing.T) {
	client := NewClient(&fakeTokens{token: "access-token"})
	scheduler := NewMailScheduler(client, t.TempDir())
	fixed := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return fixed }
	networkJob, err := scheduler.scheduleDraft("", "draft-network", fixed.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	authJob, err := scheduler.scheduleDraft("", "draft-auth", fixed.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	scheduler.recordAttempt(networkJob.ID, errors.New("result unknown"))
	scheduler.recordAttempt(authJob.ID, ErrNeedsAuth)
	list, err := scheduler.List()
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]ScheduledMail{}
	for _, item := range list.Items {
		states[item.ID] = item
	}
	if states[networkJob.ID].State != "failed" || states[networkJob.ID].NextAttemptAt != nil {
		t.Fatalf("unknown result must not replay: %+v", states[networkJob.ID])
	}
	if states[authJob.ID].State != "scheduled" || states[authJob.ID].NextAttemptAt == nil {
		t.Fatalf("auth failure should retry: %+v", states[authJob.ID])
	}
}

func TestMailSchedulerRejectsDuplicateDraftAndImmediateSend(t *testing.T) {
	client := NewClient(&fakeTokens{token: "access-token"})
	scheduler := NewMailScheduler(client, t.TempDir())
	client.scheduler = scheduler
	fixed := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return fixed }
	job, err := scheduler.scheduleDraft("", "draft-1", fixed.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.scheduleDraft("", "draft-1", fixed.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), job.ID) {
		t.Fatalf("duplicate schedule error = %v", err)
	}
	if err := ensureDraftNotScheduled(client, "", "draft-1"); err == nil || !strings.Contains(err.Error(), job.ID) {
		t.Fatalf("immediate send guard error = %v", err)
	}
	if _, err := scheduler.Cancel(context.Background(), job.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := ensureDraftNotScheduled(client, "", "draft-1"); err != nil {
		t.Fatalf("cancelled draft should be sendable: %v", err)
	}
}

func TestMailSchedulerCancelTreatsMissingDraftAsCleaned(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound","message":"draft is absent"}}`))
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	scheduler := NewMailScheduler(client, t.TempDir())
	fixed := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return fixed }
	job, err := scheduler.scheduleDraft("", "draft-gone", fixed.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.Cancel(context.Background(), job.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledMail.State != "cancelled" || result.DraftCleanup != "already_absent" {
		t.Fatalf("cancel result = %+v", result)
	}
	list, err := scheduler.List()
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("list after cancel = %+v err=%v", list, err)
	}
}

func TestMailSchedulerCancelReportsDraftCleanupFailureWithoutUndoingCancellation(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"try later"}}`))
	}))
	defer graph.Close()
	client := NewClient(&fakeTokens{token: "access-token"})
	client.SetHTTPClientForTest(graph.Client(), graph.URL)
	scheduler := NewMailScheduler(client, t.TempDir())
	fixed := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return fixed }
	job, err := scheduler.scheduleDraft("", "draft-kept", fixed.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.Cancel(context.Background(), job.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScheduledMail.State != "cancelled" || result.DraftCleanup != "failed" || result.ScheduledMail.LastError == "" {
		t.Fatalf("cancel result = %+v", result)
	}
	list, err := scheduler.List()
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("list after partial cleanup = %+v err=%v", list, err)
	}
}

func TestUnsubscribeTargetBlocksPrivateAddresses(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1"} {
		if !blockedUnsubscribeIP(net.ParseIP(raw)) {
			t.Fatalf("expected %s to be blocked", raw)
		}
	}
}
