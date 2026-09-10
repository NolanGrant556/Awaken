import {
  ipcMain,
  type WebContents
} from "electron";
import {
  normalizeModelProfileRunConfig,
  normalizeRewindThreadResult,
  normalizeRuntimeInfo,
  normalizeRuntimeRun,
  normalizeRuntimeThread,
  normalizeRuntimeThreadTranscript,
  normalizeStartRunRequest,
  type ACPRunnerCreateRequest,
  type ACPRunnerProbeResult,
  type ACPRunnersSettings,
  type ACPRunnerUpdateRequest,
  type ACPRunnerView,
  type ProjectMutationResult,
  type RenameThreadRequest,
  type RewindThreadRequest,
  type RewindThreadResultWire,
  type RuntimeInfoWire,
  type RuntimeLogEntry,
  type RuntimeRunWire,
  type RuntimeThreadListView,
  type RuntimeThreadTranscriptWire,
  type RuntimeThreadWire,
  type SetThreadArchivedResult,
  type StartRunRequest,
  type ThreadBatchDeleteResult,
  type ThreadDeleteResult,
  type ThreadModelPreferenceRequest
} from "../shared/runtime.js";
import { runDesktopOperation } from "./desktop-operation.js";
import { ModelProfileStore } from "./model-profile-store.js";
import type { RuntimeRequester } from "./runtime-http.js";
import { RuntimeDomainError } from "./runtime-http.js";
import { registerRuntimeRequest } from "./runtime-request.js";
import { RuntimeSupervisor, type SupervisorStatusInfo } from "./runtime-supervisor.js";
type ThreadBatchDeleteWire = {
  deleted_thread_ids: string[];
  failures: Array<{
    thread_id: string;
    error: string;
    message: string;
  }>;
};
type Dependencies = {
  readonly requestRuntimeOK: RuntimeRequester["requestVoid"];
  readonly requestRuntime: RuntimeRequester["request"];
  readonly modelProfileStore: ModelProfileStore;
  readonly hostFromBaseURL: (baseURL: string) => string;
  readonly supervisor: RuntimeSupervisor;
  readonly runProjectMutation: (pathname: string, method: "DELETE" | "POST", projectId: string, idempotencyKey: string, body?: unknown) => Promise<ProjectMutationResult>;
  readonly cancelActiveDreams: () => Promise<void>;
  readonly prepareNetworkRoute: (surface: string) => Promise<void>;
  readonly ensureComputerRunMonitor: (runId: string) => void;
  readonly subscribeToRun: (target: WebContents, runId: string, subscriptionId: string, afterCursor?: number) => Promise<void>;
  readonly subscribeToThreadActivity?: (target: WebContents, threadId: string, subscriptionId: string, afterCursor?: number) => Promise<void>;
  readonly toRendererSupervisorStatus: (info: SupervisorStatusInfo) => { status: SupervisorStatusInfo["status"]; };
};

export function registerRunsIpc(dependencies: Dependencies): void {
  registerRuntimeRequest("awaken:subscribe-thread-activity", async (event, payload: { threadId: string; subscriptionId: string; afterCursor?: number }) => {
    if (!dependencies.subscribeToThreadActivity) throw new Error("任务活动连接不可用");
    await dependencies.subscribeToThreadActivity(event.sender, payload.threadId, payload.subscriptionId, payload.afterCursor);
    return payload.subscriptionId;
  });
  registerRuntimeRequest("awaken:stop-thread-agents", async (_event, payload: { threadId: string }) => {
    await dependencies.requestRuntimeOK(`/v1/threads/${encodeURIComponent(payload.threadId)}/agents/stop`, { method: "POST" });
  });
  ipcMain.handle("awaken:get-runtime-info", async () => {
    try {
      const info = await dependencies.requestRuntime<RuntimeInfoWire>("/v1/runtime/info", {
        method: "GET",
      });
      const defaultProfile = await dependencies.modelProfileStore.defaultProfileSummary();
      if (defaultProfile) {
        info.configured_model = defaultProfile.model_id;
        info.protocol = defaultProfile.protocol;
        info.has_api_key = defaultProfile.has_key;
        info.base_url_host = dependencies.hostFromBaseURL(defaultProfile.base_url);
        info.model_profile = defaultProfile;
      } else {
        info.configured_model = "";
        info.has_api_key = false;
        info.base_url_host = "";
        info.model_profile = undefined;
      }

      return normalizeRuntimeInfo(info);
    } catch {
      console.error("[awaken-desktop] failed to get runtime info");
      throw new Error("Runtime unavailable");
    }
  });

  ipcMain.handle("awaken:get-runtime-logs", async () => {
    return dependencies.supervisor.getLogs() ?? ([] as RuntimeLogEntry[]);
  });

  registerRuntimeRequest("awaken:get-acp-runners", async (): Promise<ACPRunnersSettings> => {
    return dependencies.requestRuntime<ACPRunnersSettings>("/v1/acp/runners", {
      method: "GET",
    });
  });

  registerRuntimeRequest(
    "awaken:create-acp-runner",
    async (_event, request: ACPRunnerCreateRequest): Promise<ACPRunnerView> => {
      return dependencies.requestRuntime<ACPRunnerView>("/v1/acp/runners", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
    },
  );

  registerRuntimeRequest(
    "awaken:update-acp-runner",
    async (_event, runnerId: string, request: ACPRunnerUpdateRequest): Promise<ACPRunnerView> => {
      return dependencies.requestRuntime<ACPRunnerView>(`/v1/acp/runners/${encodeURIComponent(runnerId)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });
    },
  );

  registerRuntimeRequest("awaken:delete-acp-runner", async (_event, runnerId: string): Promise<void> => {
    await dependencies.requestRuntimeOK(`/v1/acp/runners/${encodeURIComponent(runnerId)}`, {
      method: "DELETE",
    });
  });

  registerRuntimeRequest(
    "awaken:probe-acp-runner",
    async (_event, runnerId: string): Promise<ACPRunnerProbeResult> => {
      return dependencies.requestRuntime<ACPRunnerProbeResult>(
        `/v1/acp/runners/${encodeURIComponent(runnerId)}/probe`,
        {
          method: "POST",
        },
      );
    },
  );

  registerRuntimeRequest("awaken:get-run", async (_event, runId: string) => {
    const run = await dependencies.requestRuntime(`/v1/runs/${runId}`, {
      method: "GET",
    });

    return normalizeRuntimeRun(run);
  });

  registerRuntimeRequest("awaken:get-thread-transcript", async (_event, threadId: string) => {
    let profile: Awaited<ReturnType<ModelProfileStore["resolveDefaultRunProfile"]>>;
    try {
      const [threads, profiles] = await Promise.all([
        dependencies.requestRuntime<RuntimeThreadWire[]>("/v1/threads", { method: "GET" }),
        dependencies.modelProfileStore.listProfiles() ?? Promise.resolve([]),
      ]);
      const thread = threads.find((candidate) => candidate.thread_id === threadId);
      const selectedProfileId = thread?.selected_model_profile_id?.trim();
      const selectedProfile = selectedProfileId
        ? profiles.find((candidate) => candidate.id === selectedProfileId && candidate.enabled)
        : undefined;
      const selectedEffort = selectedProfileId
        ? thread?.reasoning_effort_by_profile?.[selectedProfileId]
        : undefined;

      if (selectedProfile) {
        try {
          profile = await dependencies.modelProfileStore.resolveRunProfile(
            selectedProfile.id,
            selectedEffort,
          );
        } catch {
          // A saved thread preference may become unavailable after its key is
          // removed or its account metadata becomes unreadable. Rebuild with the
          // current ready default only in that case; never let a broken default
          // hide a still-runnable thread-specific profile.
          profile = await dependencies.modelProfileStore.resolveDefaultRunProfile();
        }
      } else {
        profile = await dependencies.modelProfileStore.resolveDefaultRunProfile();
      }
    } catch {
      // Transcript history remains readable when credentials are unavailable.
      // A current projection will be generated once a runnable profile exists.
      profile = undefined;
    }
    if (profile) {
      await dependencies.requestRuntimeOK(`/v1/threads/${threadId}/context-snapshot`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          model_profile: normalizeModelProfileRunConfig(profile),
        }),
      });
    }
    const transcript = await dependencies.requestRuntime<RuntimeThreadTranscriptWire>(
      `/v1/threads/${threadId}/transcript`,
      {
        method: "GET",
      },
    );

    return normalizeRuntimeThreadTranscript(transcript);
  });

  registerRuntimeRequest("awaken:rewind-thread", async (_event, request: RewindThreadRequest) => {
    const result = await dependencies.requestRuntime<RewindThreadResultWire>(
      `/v1/threads/${encodeURIComponent(request.threadId)}/rewind`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          target_run_id: request.targetRunId,
          expected_head_run_id: request.expectedHeadRunId,
        }),
      },
    );
    return normalizeRewindThreadResult(result);
  });

  registerRuntimeRequest("awaken:list-threads", async (_event, view?: RuntimeThreadListView) => {
    const threadView = view === "archived" ? "archived" : "active";
    const threads = await dependencies.requestRuntime<RuntimeThreadWire[]>(
      `/v1/threads?view=${threadView}`,
      {
        method: "GET",
      },
    );

    return threads.map(normalizeRuntimeThread);
  });

  registerRuntimeRequest(
    "awaken:set-thread-archived",
    async (_event, threadId: string, archived: boolean): Promise<SetThreadArchivedResult> => {
      try {
        const thread = await dependencies.requestRuntime<RuntimeThreadWire>(
          `/v1/threads/${encodeURIComponent(threadId)}/archive`,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ archived: Boolean(archived) }),
          },
        );
        return { ok: true, thread: normalizeRuntimeThread(thread) };
      } catch (error) {
        console.error("thread_archive_failed", {
          threadId,
          archived,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code: error instanceof RuntimeDomainError ? error.code : "THREAD_ARCHIVE_FAILED",
          message:
            error instanceof RuntimeDomainError
              ? error.message
              : "归档请求暂时未完成，Awaken 会保留当前会话，请稍后重试。",
        };
      }
    },
  );

  registerRuntimeRequest("awaken:rename-thread", async (_event, threadId: string, label: string) => {
    const body: RenameThreadRequest = { label };
    const thread = await dependencies.requestRuntime<RuntimeThreadWire>(`/v1/threads/${threadId}`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });

    return normalizeRuntimeThread(thread);
  });

  registerRuntimeRequest(
    "awaken:update-thread-model-preference",
    async (_event, threadId: string, request: ThreadModelPreferenceRequest) => {

      if (request.reasoningEffort) {
        const capability = await dependencies.modelProfileStore.reasoningCapabilityForProfile(
          request.modelProfileId,
        );
        if (!capability?.levels.includes(request.reasoningEffort)) {
          throw new Error(
            `所选模型不支持推理强度 ${request.reasoningEffort}，未更新会话偏好。`,
          );
        }
      }
      // Resolve the same exact-scope run profile used by the next request. This
      // validates existence, enabled/readable credential state and the selected
      // effort without consulting provider-global catalog metadata.
      await dependencies.modelProfileStore.resolveRunProfile(
        request.modelProfileId,
        request.reasoningEffort,
      );
      const thread = await dependencies.requestRuntime<RuntimeThreadWire>(
        `/v1/threads/${encodeURIComponent(threadId)}/model-preference`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            model_profile_id: request.modelProfileId,
            reasoning_effort: request.reasoningEffort,
          }),
        },
      );
      return normalizeRuntimeThread(thread);
    },
  );

  registerRuntimeRequest(
    "awaken:delete-thread",
    async (_event, threadId: string): Promise<ThreadDeleteResult> => {
      try {
        await dependencies.requestRuntimeOK(`/v1/threads/${encodeURIComponent(threadId)}`, {
          method: "DELETE",
        });
        return { ok: true };
      } catch (error) {
        console.error("thread_delete_failed", {
          threadId,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code: error instanceof RuntimeDomainError ? error.code : "THREAD_DELETE_FAILED",
          message:
            error instanceof RuntimeDomainError
              ? error.message
              : "删除请求暂时未完成，Awaken 会保留当前会话，请稍后重试。",
        };
      }
    },
  );

  registerRuntimeRequest(
    "awaken:delete-threads",
    async (_event, threadIds: string[]): Promise<ThreadBatchDeleteResult> => {
      try {
        const result = await dependencies.requestRuntime<ThreadBatchDeleteWire>(
          "/v1/threads/batch-delete",
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ thread_ids: threadIds }),
          },
        );
        return {
          ok: true,
          deletedThreadIds: result.deleted_thread_ids,
          failures: result.failures.map((failure) => ({
            threadId: failure.thread_id,
            code: failure.error,
            message: failure.message,
          })),
        };
      } catch (error) {
        console.error("thread_batch_delete_failed", {
          threadCount: threadIds.length,
          error: error instanceof Error ? error.message : String(error),
        });
        return {
          ok: false,
          code: error instanceof RuntimeDomainError ? error.code : "THREAD_BATCH_DELETE_FAILED",
          message: "批量删除请求暂时未完成，Awaken 会保留尚未确认删除的会话，请稍后重试。",
        };
      }
    },
  );

  registerRuntimeRequest(
    "awaken:delete-project-threads",
    async (
      _event,
      request: { projectId: string; idempotencyKey: string; },
    ): Promise<ProjectMutationResult> =>
      dependencies.runProjectMutation(
        `/v1/projects/${encodeURIComponent(request.projectId)}/threads`,
        "DELETE",
        request.projectId,
        request.idempotencyKey,
      ),
  );

  ipcMain.handle("awaken:start-run", async (_event, request: StartRunRequest) => {
    return runDesktopOperation(async () => {
      await dependencies.cancelActiveDreams();
      const body = normalizeStartRunRequest(request);
      const selectedProfile = await dependencies.modelProfileStore.resolveRunBundle(
        request.modelProfileId,
        request.reasoningEffort,
      );
      if (!selectedProfile) {
        throw new Error("请先在设置 > 模型中配置可用模型。");
      }
      body.model_profile = normalizeModelProfileRunConfig(selectedProfile);
      // Renderer selection hints terminate at Main. Core receives only the
      // fully resolved, credential-bearing model_profile assembled here.
      body.model_profile_id = undefined;
      body.reasoning_effort = undefined;
      await dependencies.prepareNetworkRoute("model-run");
      const run = await dependencies.requestRuntime<RuntimeRunWire>("/v1/runs", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      });

      const normalizedRun = normalizeRuntimeRun(run);
      dependencies.ensureComputerRunMonitor(normalizedRun.runId);
      return normalizedRun;
    });
  });

  registerRuntimeRequest("awaken:cancel-run", async (_event, runId: string) => {
    const run = await dependencies.requestRuntime(`/v1/runs/${runId}/cancel`, {
      method: "POST",
    });

    return normalizeRuntimeRun(run);
  });

  registerRuntimeRequest(
    "awaken:cancel-subagent",
    async (_event, payload: { runId: string; taskId: string; reason?: string; }) => {
      await dependencies.requestRuntimeOK(
        `/v1/runs/${encodeURIComponent(payload.runId)}/subagents/${encodeURIComponent(payload.taskId)}/cancel`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reason: payload.reason ?? "desktop_user" }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:answer-user-question",
    async (_event, payload: { runId: string; questionId: string; answer: string; }) => {
      await dependencies.requestRuntimeOK(
        `/v1/runs/${payload.runId}/user-questions/${payload.questionId}/answer`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ answer: payload.answer }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:cancel-user-question",
    async (_event, payload: { runId: string; questionId: string; }) => {
      await dependencies.requestRuntimeOK(
        `/v1/runs/${encodeURIComponent(payload.runId)}/user-questions/${encodeURIComponent(payload.questionId)}/cancel`,
        { method: "POST" },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:resolve-external-interaction",
    async (
      _event,
      payload: {
        runId: string;
        interactionId: string;
        resolution: "completed" | "cancelled" | "expired" | "failed";
        content?: Record<string, unknown>;
      },
    ) => {
      await dependencies.requestRuntimeOK(
        `/v1/runs/${payload.runId}/external-interactions/${payload.interactionId}/resolve`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ resolution: payload.resolution, content: payload.content }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:resolve-acp-permission",
    async (
      _event,
      payload: {
        runId: string;
        interactionId: string;
        optionId: string;
        cancelled: boolean;
      },
    ) => {
      await dependencies.requestRuntimeOK(
        `/v1/runs/${payload.runId}/acp-permissions/${payload.interactionId}/resolve`,
        {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            option_id: payload.optionId,
            cancelled: payload.cancelled,
          }),
        },
      );
    },
  );

  registerRuntimeRequest(
    "awaken:subscribe-run",
    async (
      event,
      payload: {
        runId: string;
        subscriptionId: string;
        afterCursor?: number;
        afterSeq?: number;
      },
    ) => {
      dependencies.ensureComputerRunMonitor(payload.runId);
      await dependencies.subscribeToRun(
        event.sender,
        payload.runId,
        payload.subscriptionId,
        payload.afterCursor ?? payload.afterSeq,
      );
      return payload.subscriptionId;
    },
  );

  ipcMain.handle("awaken:runtime-supervisor-status", async () => {
    return dependencies.toRendererSupervisorStatus(dependencies.supervisor.getStatus() ?? { status: "stopped" });
  });
}
