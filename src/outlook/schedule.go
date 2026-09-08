package outlook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/filestore"

	"github.com/google/uuid"
)

const scheduledMailStoreVersion = 1

type ScheduledMail struct {
	ID            string     `json:"id"`
	Mailbox       string     `json:"mailbox,omitempty"`
	DraftID       string     `json:"draftId"`
	SendAt        time.Time  `json:"sendAt"`
	CreatedAt     time.Time  `json:"createdAt"`
	NextAttemptAt *time.Time `json:"nextAttemptAt,omitempty"`
	State         string     `json:"state"`
	LastError     string     `json:"lastError,omitempty"`
}

type ScheduledMailList struct {
	Items []ScheduledMail `json:"items"`
}

type CancelScheduledMailResult struct {
	ScheduledMail ScheduledMail
	DraftCleanup  string
}

type scheduledMailFile struct {
	Version int             `json:"version"`
	Items   []ScheduledMail `json:"items"`
}

type MailScheduler struct {
	client *Client
	path   string
	now    func() time.Time

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	doneCh  chan struct{}
	nudge   chan struct{}
}

func NewMailScheduler(client *Client, awakenRoot string) *MailScheduler {
	return &MailScheduler{
		client: client, path: filepath.Join(filepath.Clean(awakenRoot), "outlook", "scheduled-mail.json"),
		now: time.Now, nudge: make(chan struct{}, 1),
	}
}

func (s *MailScheduler) Start() error {
	if s == nil || s.client == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("Outlook 本地定时发送不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	changed := false
	for i := range file.Items {
		if file.Items[i].State == "sending" {
			file.Items[i].State = "failed"
			file.Items[i].LastError = "发送结果未知，请先核对已发送邮件；不会自动重试"
			changed = true
		}
	}
	if changed {
		if err := s.saveLocked(file); err != nil {
			return err
		}
	}
	s.started = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	go s.loop(s.stopCh, s.doneCh)
	return nil
}

func (s *MailScheduler) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.started = false
	stopCh, doneCh := s.stopCh, s.doneCh
	close(stopCh)
	s.mu.Unlock()
	<-doneCh
}

func (s *MailScheduler) ScheduleMail(ctx context.Context, mailbox, sendAt string, input ComposeMessage) (ScheduledMail, error) {
	when, err := s.validateSendAt(sendAt)
	if err != nil {
		return ScheduledMail{}, err
	}
	draft, err := s.client.CreateDraftInMailbox(ctx, mailbox, input)
	if err != nil {
		return ScheduledMail{}, err
	}
	job, err := s.scheduleDraft(mailbox, draft.ID, when)
	if err != nil {
		return ScheduledMail{}, fmt.Errorf("草稿 %s 已创建，但无法保存定时发送：%w", draft.ID, err)
	}
	return job, nil
}

func (s *MailScheduler) ScheduleDraft(ctx context.Context, mailbox, draftID, sendAt string) (ScheduledMail, error) {
	when, err := s.validateSendAt(sendAt)
	if err != nil {
		return ScheduledMail{}, err
	}
	message, err := s.client.GetMessageInMailbox(ctx, mailbox, draftID)
	if err != nil {
		return ScheduledMail{}, err
	}
	if !message.IsDraft {
		return ScheduledMail{}, fmt.Errorf("message is not an Outlook draft")
	}
	return s.scheduleDraft(mailbox, draftID, when)
}

func (s *MailScheduler) scheduleDraft(mailbox, draftID string, when time.Time) (ScheduledMail, error) {
	if s == nil {
		return ScheduledMail{}, fmt.Errorf("Outlook 本地定时发送不可用")
	}
	if _, err := mailboxRoot(mailbox); err != nil {
		return ScheduledMail{}, err
	}
	draftID, err := requiredIdentifier("draftId", draftID)
	if err != nil {
		return ScheduledMail{}, err
	}
	job := ScheduledMail{
		ID: uuid.NewString(), Mailbox: strings.TrimSpace(mailbox), DraftID: draftID,
		SendAt: when.UTC(), CreatedAt: s.now().UTC(), State: "scheduled",
	}
	s.mu.Lock()
	file, err := s.loadLocked()
	if err == nil {
		for _, existing := range file.Items {
			if existing.DraftID == job.DraftID && strings.EqualFold(strings.TrimSpace(existing.Mailbox), strings.TrimSpace(job.Mailbox)) {
				err = fmt.Errorf("draft already has scheduled mail plan %s", existing.ID)
				break
			}
		}
		if err == nil {
			file.Items = append(file.Items, job)
			err = s.saveLocked(file)
		}
	}
	s.mu.Unlock()
	if err != nil {
		return ScheduledMail{}, err
	}
	s.reload()
	return job, nil
}

func (s *MailScheduler) ScheduledForDraft(mailbox, draftID string) (ScheduledMail, bool, error) {
	if s == nil {
		return ScheduledMail{}, false, fmt.Errorf("Outlook 本地定时发送不可用")
	}
	if _, err := mailboxRoot(mailbox); err != nil {
		return ScheduledMail{}, false, err
	}
	draftID, err := requiredIdentifier("draftId", draftID)
	if err != nil {
		return ScheduledMail{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return ScheduledMail{}, false, err
	}
	for _, item := range file.Items {
		if item.DraftID == draftID && strings.EqualFold(strings.TrimSpace(item.Mailbox), strings.TrimSpace(mailbox)) {
			return item, true, nil
		}
	}
	return ScheduledMail{}, false, nil
}

func (s *MailScheduler) List() (ScheduledMailList, error) {
	if s == nil {
		return ScheduledMailList{}, fmt.Errorf("Outlook 本地定时发送不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return ScheduledMailList{}, err
	}
	sort.Slice(file.Items, func(i, j int) bool { return file.Items[i].SendAt.Before(file.Items[j].SendAt) })
	return ScheduledMailList{Items: file.Items}, nil
}

func (s *MailScheduler) Cancel(ctx context.Context, scheduleID string, deleteDraft bool) (CancelScheduledMailResult, error) {
	if s == nil {
		return CancelScheduledMailResult{}, fmt.Errorf("Outlook 本地定时发送不可用")
	}
	scheduleID, err := requiredIdentifier("scheduleId", scheduleID)
	if err != nil {
		return CancelScheduledMailResult{}, err
	}
	s.mu.Lock()
	file, err := s.loadLocked()
	var cancelled ScheduledMail
	if err == nil {
		kept := file.Items[:0]
		for _, item := range file.Items {
			if item.ID == scheduleID {
				if item.State == "sending" {
					s.mu.Unlock()
					return CancelScheduledMailResult{}, fmt.Errorf("邮件已进入发送阶段，无法确认取消；请核对已发送邮件")
				}
				cancelled = item
				continue
			}
			kept = append(kept, item)
		}
		if cancelled.ID == "" {
			err = fmt.Errorf("scheduled mail not found")
		} else {
			file.Items = kept
			err = s.saveLocked(file)
		}
	}
	s.mu.Unlock()
	if err != nil {
		return CancelScheduledMailResult{}, err
	}
	s.reload()
	cancelled.State = "cancelled"
	result := CancelScheduledMailResult{ScheduledMail: cancelled, DraftCleanup: "retained"}
	if deleteDraft {
		if err := s.client.DeleteMessageInMailbox(ctx, cancelled.Mailbox, cancelled.DraftID); err != nil {
			if graphItemNotFound(err) {
				result.DraftCleanup = "already_absent"
				return result, nil
			}
			result.DraftCleanup = "failed"
			result.ScheduledMail.LastError = safeGraphText("定时任务已取消，但草稿未能删除："+err.Error(), 512)
			return result, nil
		}
		result.DraftCleanup = "deleted"
	}
	return result, nil
}

func graphItemNotFound(err error) bool {
	var graphErr *GraphError
	if !errors.As(err, &graphErr) {
		return false
	}
	return graphErr.StatusCode == 404 || strings.EqualFold(strings.TrimSpace(graphErr.Code), "ErrorItemNotFound")
}

func (s *MailScheduler) validateSendAt(value string) (time.Time, error) {
	when, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("sendAt must be an RFC3339 timestamp with timezone")
	}
	now := s.now()
	if !when.After(now.Add(5 * time.Second)) {
		return time.Time{}, fmt.Errorf("sendAt must be at least 5 seconds in the future")
	}
	if when.After(now.Add(366 * 24 * time.Hour)) {
		return time.Time{}, fmt.Errorf("sendAt cannot be more than 366 days in the future")
	}
	return when, nil
}

func (s *MailScheduler) loop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	for {
		delay := s.nextDelay()
		timer := time.NewTimer(delay)
		select {
		case <-stopCh:
			timer.Stop()
			return
		case <-s.nudge:
			timer.Stop()
			continue
		case <-timer.C:
			s.processDue()
		}
	}
}

func (s *MailScheduler) nextDelay() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil || len(file.Items) == 0 {
		return time.Minute
	}
	now := s.now()
	var next time.Time
	for _, item := range file.Items {
		if item.State != "scheduled" {
			continue
		}
		due := item.SendAt
		if item.NextAttemptAt != nil && item.NextAttemptAt.After(due) {
			due = *item.NextAttemptAt
		}
		if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	if next.IsZero() {
		return time.Minute
	}
	if !next.After(now) {
		return 0
	}
	return next.Sub(now)
}

func (s *MailScheduler) processDue() {
	s.mu.Lock()
	file, err := s.loadLocked()
	if err != nil {
		s.mu.Unlock()
		return
	}
	now := s.now().UTC()
	due := make([]ScheduledMail, 0)
	for _, item := range file.Items {
		if item.State != "scheduled" {
			continue
		}
		dueAt := item.SendAt
		if item.NextAttemptAt != nil && item.NextAttemptAt.After(dueAt) {
			dueAt = *item.NextAttemptAt
		}
		if !dueAt.After(now) {
			due = append(due, item)
		}
	}
	s.mu.Unlock()
	for _, candidate := range due {
		item, claimed, err := s.claimDue(candidate.ID)
		if err != nil {
			return
		}
		if !claimed {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		sendErr := s.client.SendDraft(ctx, item.Mailbox, item.DraftID)
		cancel()
		if err := s.recordAttempt(item.ID, sendErr); err != nil {
			return
		}
	}
}

// claimDue rechecks the durable record immediately before dispatch. Cancellation
// and dispatch have one owner: only a persisted sending claim permits a send.
func (s *MailScheduler) claimDue(id string) (ScheduledMail, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return ScheduledMail{}, false, err
	}
	now := s.now()
	for i := range file.Items {
		item := &file.Items[i]
		if item.ID != id || item.State != "scheduled" || item.SendAt.After(now) || (item.NextAttemptAt != nil && item.NextAttemptAt.After(now)) {
			continue
		}
		item.State = "sending"
		if err := s.saveLocked(file); err != nil {
			return ScheduledMail{}, false, err
		}
		return *item, true, nil
	}
	return ScheduledMail{}, false, nil
}

func (s *MailScheduler) recordAttempt(scheduleID string, sendErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.loadLocked()
	if err != nil {
		return err
	}
	kept := file.Items[:0]
	for _, item := range file.Items {
		if item.ID != scheduleID {
			kept = append(kept, item)
			continue
		}
		if sendErr == nil {
			continue
		}
		item.LastError = safeGraphText(sendErr.Error(), 512)
		if errors.Is(sendErr, ErrNeedsAuth) {
			retryAt := s.now().UTC().Add(5 * time.Minute)
			item.State = "scheduled"
			item.NextAttemptAt = &retryAt
		} else {
			// Any non-auth failure may have an unknown send result. Do not
			// replay a write automatically; the user must inspect Sent Items
			// before deciding whether to schedule another send.
			item.State = "failed"
			item.NextAttemptAt = nil
		}
		kept = append(kept, item)
	}
	file.Items = kept
	return s.saveLocked(file)
}

func (s *MailScheduler) reload() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

func (s *MailScheduler) loadLocked() (scheduledMailFile, error) {
	file := scheduledMailFile{Version: scheduledMailStoreVersion, Items: []ScheduledMail{}}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return file, nil
	}
	if err != nil {
		return file, fmt.Errorf("read scheduled mail store: %w", err)
	}
	if err := json.Unmarshal(data, &file); err != nil || file.Version != scheduledMailStoreVersion {
		return scheduledMailFile{}, fmt.Errorf("scheduled mail store is invalid")
	}
	return file, nil
}

func (s *MailScheduler) saveLocked(file scheduledMailFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create scheduled mail directory: %w", err)
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode scheduled mail store: %w", err)
	}
	if err := filestore.WriteFileAtomic(s.path, data); err != nil {
		return fmt.Errorf("commit scheduled mail store: %w", err)
	}
	return nil
}
