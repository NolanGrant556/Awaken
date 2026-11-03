package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"awaken/src/agentcore"
)

const maxPendingSteering = 8

var ErrSteeringUnavailable = errors.New("当前运行无法接收插话")

type SteeringRequest struct {
	ThreadID    string               `json:"thread_id"`
	RequestID   string               `json:"request_id"`
	Text        string               `json:"text"`
	Attachments []RunInputAttachment `json:"attachments,omitempty"`
}

type SteeringReceipt struct {
	ID       string `json:"id"`
	Sequence int    `json:"sequence"`
}

type steeringEntry struct {
	receipt SteeringReceipt
	digest  [32]byte
}

type steeringQueue struct {
	mu       sync.Mutex
	desktop  bool
	enabled  bool
	vision   bool
	sealed   bool
	pending  []agentcore.SteeringInput
	accepted map[string]steeringEntry
}

func (s *RunService) SubmitSteering(ctx context.Context, runID string, req SteeringRequest) (SteeringReceipt, error) {
	session, err := s.session(runID)
	if err != nil {
		return SteeringReceipt{}, err
	}
	return session.submitSteering(ctx, req)
}

func (s *Session) submitSteering(ctx context.Context, req SteeringRequest) (SteeringReceipt, error) {
	req.Text = strings.TrimSpace(req.Text)
	if req.RequestID == "" || len(req.RequestID) > 128 || len(req.Text) > 8192 ||
		(req.Text == "" && len(req.Attachments) == 0) || len(req.Attachments) > 8 {
		return SteeringReceipt{}, fmt.Errorf("%w: 插话正文或附件超出限制", ErrInvalidInput)
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return SteeringReceipt{}, err
	}
	digest := sha256.Sum256(encoded)
	q := &s.steering
	q.mu.Lock()
	run, prior, err := s.checkSteeringRequest(q, req, digest)
	vision := q.vision
	q.mu.Unlock()
	if err != nil || prior.ID != "" {
		return prior, err
	}

	id := newRuntimeID("steering")
	attachments, cleanup, err := stageSteeringAttachments(run.ThreadID, id, vision, req.Attachments)
	if err != nil {
		return SteeringReceipt{}, err
	}
	accepted := false
	defer func() {
		if !accepted {
			cleanup()
		}
	}()
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return SteeringReceipt{}, err
	}
	run, prior, err = s.checkSteeringRequest(q, req, digest)
	if err != nil || prior.ID != "" {
		return prior, err
	}
	durable, ok := s.semanticRecorderSnapshot().(agentcore.FlushedSemanticRecorder)
	if !ok {
		return SteeringReceipt{}, agentcore.ErrPersistenceUnavailable
	}
	record := agentcore.NewSemanticRecord(agentcore.SemanticKindRuntimeNotice, run.ThreadID, run.RunID, "", "", "",
		map[string]any{"event_type": "steering_accepted", "steering_id": id, "content": req.Text,
			"attachments": contextAttachmentsToTranscriptAttachments(inputAttachmentsToContextAttachments(attachments))})
	if _, err = durable.AppendFlushed(ctx, record); err != nil {
		return SteeringReceipt{}, err
	}
	if q.accepted == nil {
		q.accepted = make(map[string]steeringEntry)
	}
	receipt := SteeringReceipt{ID: id, Sequence: len(q.accepted) + 1}
	input := agentcore.SteeringInput{ID: id, Text: req.Text, Attachments: inputAttachmentsToContextAttachments(attachments)}
	q.pending = append(q.pending, input)
	q.accepted[req.RequestID] = steeringEntry{receipt: receipt, digest: digest}
	accepted = true
	return receipt, nil
}

// Caller holds the steering lock, shared with finalization and Stop.
func (s *Session) checkSteeringRequest(q *steeringQueue, req SteeringRequest, digest [32]byte) (Run, SteeringReceipt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run := s.run
	if run.ThreadID != req.ThreadID {
		return run, SteeringReceipt{}, ErrSteeringUnavailable
	}
	if previous, ok := q.accepted[req.RequestID]; ok {
		if previous.digest != digest {
			return run, SteeringReceipt{}, fmt.Errorf("%w: 插话请求 ID 已用于另一条消息", ErrInvalidInput)
		}
		return run, previous.receipt, nil
	}
	hasGate := len(s.userQuestions) != 0 || len(s.externalInteractions) != 0 || len(s.computerReviewOwners) != 0
	if !q.enabled || q.sealed || hasGate || run.Status != RunStatusRunning || len(q.pending) >= maxPendingSteering {
		return run, SteeringReceipt{}, ErrSteeringUnavailable
	}
	return run, SteeringReceipt{}, nil
}

func stageSteeringAttachments(threadID, id string, vision bool, source []RunInputAttachment) ([]RunInputAttachment, func(), error) {
	var stored []RunInputAttachment
	var paths []string
	cleanup := func() {
		for _, path := range paths {
			_ = os.Remove(path)
		}
	}
	fail := func(err error) ([]RunInputAttachment, func(), error) {
		cleanup()
		return nil, nil, err
	}
	for i, attachment := range source {
		attachment.ID = fmt.Sprintf("%s_%d", id, i)
		if attachment.Kind == RunInputAttachmentKindImage && !vision {
			return fail(fmt.Errorf("%w: 本轮模型不支持图片", ErrInvalidInput))
		}
		normalized, imageBytes, _, err := normalizeRunInputAttachment(attachment)
		if err != nil {
			return fail(err)
		}
		if normalized.Kind == RunInputAttachmentKindImage {
			if !strings.EqualFold(strings.TrimSpace(normalized.MimeType), http.DetectContentType(imageBytes)) {
				return fail(fmt.Errorf("%w: 图片内容与声明格式不匹配", ErrInvalidInput))
			}
			images, err := persistRunInputAttachments(threadID, []RunInputAttachment{normalized})
			if err != nil {
				return fail(err)
			}
			normalized = images[0]
			paths = append(paths, filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(normalized.StorageKey)))
		} else {
			path, err := copySteeringFile(threadID, normalized.ID, normalized.Path)
			if err != nil {
				return fail(err)
			}
			paths = append(paths, path)
			if normalized.DisplayName == "" {
				normalized.DisplayName = filepath.Base(normalized.Path)
			}
			normalized.Path = path
		}
		stored = append(stored, normalized)
	}
	return stored, cleanup, nil
}

func copySteeringFile(threadID, id, source string) (string, error) {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxRunInputAttachmentBytes {
		return "", fmt.Errorf("%w: 文件附件须为不超过 8 MiB 的普通文件", ErrInvalidInput)
	}
	file, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("%w: 无法读取文件附件", ErrInvalidInput)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", fmt.Errorf("%w: 文件附件已变化", ErrInvalidInput)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRunInputAttachmentBytes+1))
	if err != nil || len(data) > maxRunInputAttachmentBytes {
		return "", fmt.Errorf("%w: 无法读取文件附件或文件超限", ErrInvalidInput)
	}
	dir := filepath.Join(runInputAttachmentRootDir(), safeTranscriptFileName(threadID), "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, id+filepath.Ext(source))
	if err := os.WriteFile(path, data, 0o400); err != nil {
		return "", err
	}
	return path, nil
}

func (s sessionEventSink) TakeSteering(ctx context.Context) ([]agentcore.SteeringInput, error) {
	q := &s.session.steering
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var inputs []agentcore.SteeringInput
	for _, input := range q.pending {
		if steeringAttachmentsAvailable(input) {
			inputs = append(inputs, input)
			continue
		}
		recorder, ok := s.session.semanticRecorderSnapshot().(agentcore.FlushedSemanticRecorder)
		if !ok {
			return nil, agentcore.ErrPersistenceUnavailable
		}
		run := s.session.Snapshot()
		record := agentcore.NewSemanticRecord(agentcore.SemanticKindRuntimeNotice, run.ThreadID, run.RunID, "", "", "",
			map[string]any{"event_type": "steering_unapplied", "steering_id": input.ID, "content": "附件已失效"})
		if _, err := recorder.AppendFlushed(ctx, record); err != nil {
			return nil, err
		}
	}
	q.pending = nil
	return inputs, nil
}

func (s sessionEventSink) SealSteering(allowContinuation bool) bool {
	q := &s.session.steering
	q.mu.Lock()
	defer q.mu.Unlock()
	if allowContinuation && len(q.pending) > 0 {
		return true
	}
	q.sealed = true
	return false
}

func steeringAttachmentsAvailable(input agentcore.SteeringInput) bool {
	for _, attachment := range input.Attachments {
		path := attachment.Path
		if attachment.Kind == "image" {
			path = filepath.Join(runInputAttachmentRootDir(), filepath.FromSlash(attachment.StorageKey))
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}
