package runtime

import (
	"context"
	"errors"
	"strings"
	"time"

	"awaken/src/agentcore"
	"awaken/src/modelprofile"
)

// Scheduled-task integration (contract docs/12_定时任务/定时任务设计契约.md
// §4.5, §9). Everything here is host-only: these are plain Go methods on
// RunService used by src/scheduledtask, never mapped by HTTP handlers, so a
// renderer client cannot forge the scheduled_task source or pin titles.

// ScheduledClientSource is the ClientInfo.Source and thread source recorded
// for scheduled-task dedicated threads.
const ScheduledClientSource = "scheduled_task"

// ScheduledRunRequest is the host-only input for an unattended scheduled
// Agent execution.
type ScheduledRunRequest struct {
	TaskID       string
	ExecutionID  string
	TaskName     string
	Prompt       string
	ThreadID     string // empty creates the task's dedicated thread
	Workspace    WorkspaceConfig
	ModelProfile *ModelProfileConfig
}

// StartScheduledRun starts an unattended Agent run for a scheduled task on
// the task's stable dedicated thread (contract §9.3). It reuses StartRun's
// journal, context, tool snapshot, project fence and recovery path; the only
// differences are the scheduled_task client identity, the task-owned initial
// thread title (no prompt-derived label, no LLM title generation) and the
// workspace re-anchor that lets the dedicated thread follow its project when
// the project folders changed. A user run holding the thread makes this
// return ErrThreadBusy, which the caller records as skipped/TASK_THREAD_BUSY.
func (s *RunService) StartScheduledRun(req ScheduledRunRequest) (Run, error) {
	return s.StartRun(StartRunRequest{
		Input:     req.Prompt,
		Workspace: req.Workspace,
		Client: ClientInfo{
			EntryPoint: ScheduledClientSource,
			Source:     ScheduledClientSource,
			RequestID:  req.ExecutionID,
		},
		ThreadID:          req.ThreadID,
		ModelProfile:      req.ModelProfile,
		Title:             req.TaskName,
		reanchorWorkspace: true,
	})
}

// ScheduledNoticeRequest is the host-only input for a fixed reminder
// (contract §9.4). Content is the fixed reminder body; no model is involved.
type ScheduledNoticeRequest struct {
	TaskID      string
	ExecutionID string
	TaskName    string
	Content     string
	ThreadID    string // empty creates the task's dedicated thread
	Workspace   WorkspaceConfig
}

// PublishScheduledNotice writes the host-owned
// run_started -> scheduled_notice -> run_terminal(success) semantic records
// of one reminder execution into the task's dedicated thread and publishes
// them through a short-lived session so live subscribers of the run stream
// see the same facts the journal replays. It never calls a model, never
// creates a tool snapshot and never fabricates user/assistant messages.
//
// Live path: sessions are per-run event buses, so the notice goes through a
// real (agent-less) session registered in RunService like any other run;
// subscribers of that run's stream receive the three records, and the
// durable journal makes them replayable through the regular transcript path.
// A run that is active on the dedicated thread makes this return
// ErrThreadBusy (recorded as skipped/TASK_THREAD_BUSY by the caller).
func (s *RunService) PublishScheduledNotice(ctx context.Context, req ScheduledNoticeRequest) (threadID, runID string, err error) {
	if strings.TrimSpace(req.Content) == "" {
		return "", "", ErrInvalidInput
	}
	if s.journals == nil {
		return "", "", ErrPersistenceUnavailable
	}
	workspace, err := normalizeWorkspace(req.Workspace)
	if err != nil {
		return "", "", err
	}
	s.projectMutationMu.RLock()
	defer s.projectMutationMu.RUnlock()
	if s.projectService != nil {
		allowed, err := s.projectService.CanStartRun(workspace.WorkspaceRoot)
		if err != nil {
			return "", "", err
		}
		if !allowed {
			return "", "", ErrProjectOperationConflict
		}
	}
	s.mu.RLock()
	projectFenced := len(s.projectFences[normalizeProjectWorkspaceRoot(workspace.WorkspaceRoot)]) > 0
	s.mu.RUnlock()
	if projectFenced {
		return "", "", ErrProjectOperationConflict
	}

	threadID = strings.TrimSpace(req.ThreadID)
	runID = newRuntimeID("run")
	now := time.Now().UTC()
	newThread := threadID == ""
	if newThread {
		threadID = newRuntimeID("thread")
	}

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := NewSession(runID, threadID, "", nil, nil, cancel)

	s.mu.Lock()
	if !newThread {
		thread, ok := s.threads[threadID]
		if !ok {
			// Restore the durable thread into memory so the notice joins the
			// same run list the transcript view assembles.
			s.mu.Unlock()
			meta, restored, restoreErr := s.threadStore.RestoreThread(threadID)
			if restoreErr != nil {
				return "", "", restoreErr
			}
			s.mu.Lock()
			thread = &Thread{
				threadID:       threadID,
				projectID:      firstNonEmpty(meta.Snapshot.ProjectID, scopeProjectID(workspace)),
				workspace:      workspace,
				label:          firstNonEmpty(meta.Snapshot.Label, restored.Label),
				titleSource:    meta.Snapshot.TitleSource,
				titleStatus:    meta.Snapshot.TitleStatus,
				titleUpdatedAt: meta.Snapshot.TitleUpdatedAt,
				titleSeedRunID: meta.Snapshot.TitleSeedRunID,
				latestRunID:    runID,
				runIDs:         append(append([]string(nil), restored.RunIDs...), runID),
				createdAt:      firstNonZeroTime(restored.CreatedAt, meta.Snapshot.CreatedAt, now),
				updatedAt:      now,
				source:         meta.Snapshot.Source,
				clientName:     meta.Snapshot.ClientName,
				clientVersion:  meta.Snapshot.ClientVersion,
				activeRunID:    runID,
			}
			s.threads[threadID] = thread
		} else {
			if thread.deletionRequested {
				s.mu.Unlock()
				return "", "", ErrThreadNotFound
			}
			if thread.activeRunID != "" {
				s.mu.Unlock()
				cancel()
				return "", "", ErrThreadBusy
			}
			thread.activeRunID = runID
			thread.latestRunID = runID
			thread.runIDs = append(thread.runIDs, runID)
			thread.updatedAt = now
		}
	} else {
		s.threads[threadID] = &Thread{
			threadID:       threadID,
			projectID:      scopeProjectID(workspace),
			workspace:      workspace,
			label:          trimThreadLabel(req.TaskName),
			titleSource:    ThreadTitleSourceTask,
			titleStatus:    ThreadTitleStatusReady,
			titleUpdatedAt: now,
			latestRunID:    runID,
			activeRunID:    runID,
			runIDs:         []string{runID},
			createdAt:      now,
			updatedAt:      now,
			source:         ScheduledClientSource,
		}
		s.pruneInactiveThreadsLocked()
	}
	s.sessions[runID] = session
	threadSnapshot := s.threads[threadID].snapshot()
	threadSnapshot.RunID = runID
	s.mu.Unlock()

	defer s.clearActiveRun(threadID, runID)

	// Durable thread metadata first: the notice must never leave a thread the
	// store cannot describe.
	preview := ""
	if newThread {
		preview = trimThreadLabel(req.TaskName)
	}
	if err := s.threadStore.CreateThread(threadSnapshot, "", preview); err != nil {
		return "", "", err
	}

	recorder, err := s.journals.OpenRecorder(threadID)
	if err != nil {
		return "", "", err
	}
	if journal, ok := recorder.(*ExecutionJournal); ok {
		if err := ensureThreadJournalRecord(ctx, journal, threadID, threadSnapshot.Label); err != nil {
			return "", "", err
		}
	}
	if assigned, ok := recorder.(agentcore.AssignedSemanticRecorder); ok {
		recorder = newLiveSemanticRecorder(assigned, session)
	}
	session.SetSemanticRecorder(recorder)

	records := []agentcore.SemanticRecord{
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunStarted, threadID, runID, "", "", "", map[string]any{}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindScheduledNotice, threadID, runID, "", "", "", map[string]any{
			"task_id":      req.TaskID,
			"execution_id": req.ExecutionID,
			"task_name":    req.TaskName,
			"content":      req.Content,
		}),
		agentcore.NewSemanticRecord(agentcore.SemanticKindRunTerminal, threadID, runID, "", "", "", map[string]any{
			"status": "finished",
		}),
	}
	for _, record := range records {
		if err := recorder.Append(ctx, record); err != nil {
			return "", "", err
		}
	}
	if err := recorder.Flush(ctx); err != nil {
		return "", "", err
	}
	return threadID, runID, nil
}

// UpdateScheduledThreadTitle follows a scheduled-task rename on the task's
// dedicated thread (contract §4.5): a title the user renamed manually
// (titleSource=manual) is never overwritten. It is a no-op when the thread is
// unknown or already manual.
func (s *RunService) UpdateScheduledThreadTitle(threadID, taskName string) error {
	threadID = strings.TrimSpace(threadID)
	label := trimThreadLabel(strings.TrimSpace(taskName))
	if threadID == "" || label == "" {
		return nil
	}

	s.mu.RLock()
	thread, inMemory := s.threads[threadID]
	s.mu.RUnlock()
	if inMemory && thread.titleSource == ThreadTitleSourceManual {
		return nil
	}
	if !inMemory {
		record, _, err := s.threadStore.RestoreThread(threadID)
		if err != nil {
			if errors.Is(err, ErrThreadNotFound) {
				return nil
			}
			return err
		}
		if record.Snapshot.TitleSource == ThreadTitleSourceManual {
			return nil
		}
	}

	now := time.Now().UTC()
	persisted, err := s.threadStore.UpdateThreadTitleMetadata(threadID, label, ThreadTitleSourceTask, ThreadTitleStatusReady, "", now)
	if err != nil {
		return err
	}
	if persisted.TitleSource == ThreadTitleSourceManual || persisted.Label != label {
		// The store-side CAS lost to a manual rename; keep the user's title.
		return nil
	}

	s.mu.Lock()
	if thread, ok := s.threads[threadID]; ok && thread.titleSource != ThreadTitleSourceManual {
		thread.label = persisted.Label
		thread.titleSource = persisted.TitleSource
		thread.titleStatus = persisted.TitleStatus
		thread.titleUpdatedAt = persisted.TitleUpdatedAt
		thread.updatedAt = persisted.UpdatedAt
	}
	s.mu.Unlock()
	return nil
}

// ModelProfileConfigFromResolved converts an Awaken-owned resolved profile
// into the runtime run configuration. This is the single conversion site
// shared by the ACP control route and scheduled tasks; the resolved API key
// only ever travels in memory through this struct and must never be
// serialized into task definitions, journals, logs or error messages.
func ModelProfileConfigFromResolved(profile modelprofile.ResolvedProfile) *ModelProfileConfig {
	return &ModelProfileConfig{
		ProfileID:          profile.ID,
		ProviderID:         profile.ProviderID,
		DisplayName:        profile.DisplayName,
		ModelID:            profile.ModelID,
		CapabilityModelID:  profile.CapabilityModelID,
		EndpointSelector:   profile.EndpointSelector,
		ModelSelectorKind:  profile.ModelSelectorKind,
		BaseURL:            profile.BaseURL,
		Protocol:           profile.Protocol,
		AuthScheme:         profile.AuthScheme,
		APIKey:             profile.APIKey,
		Temperature:        profile.Temperature,
		MaxTokens:          profile.MaxTokens,
		ContextWindow:      profile.ContextWindow,
		CredentialRevision: profile.CredentialRevision,
		SupportsVision:     profile.SupportsVision,
		SupportsNativePDF:  profile.SupportsNativePDF,
		SupportsVideo:      profile.SupportsVideo,
		SupportsTools:      profile.SupportsTools,
		ReasoningEffort:    profile.ReasoningEffort,
	}
}
