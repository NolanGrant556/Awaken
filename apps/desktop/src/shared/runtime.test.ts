import { describe, expect, it } from "vitest";

import {
  normalizeBranchThreadRequest,
  normalizeRuntimeThread,
  normalizeRuntimeThreadContext,
  normalizeRuntimeThreadLite,
  normalizeRunProfile,
  normalizeStartRunRequest,
} from "./runtime.js";

describe("normalizeRunProfile", () => {
  it("serializes camelCase profile fields into wire format", () => {
    expect(
      normalizeRunProfile({
        id: "analysis",
        name: "Analysis",
        description: "profile description",
        systemPrompt: "be precise",
        model: "gpt-test",
        maxTurns: 3,
        tools: ["list_files"],
        disallowedTools: ["shell"],
        permissionMode: "ask",
      }),
    ).toEqual({
      id: "analysis",
      name: "Analysis",
      description: "profile description",
      system_prompt: "be precise",
      model: "gpt-test",
      max_turns: 3,
      tools: ["list_files"],
      disallowed_tools: ["shell"],
      permission_mode: "ask",
    });
  });

  it("returns undefined for missing or empty profile", () => {
    expect(normalizeRunProfile()).toBeUndefined();
    expect(normalizeRunProfile({})).toBeUndefined();
  });
});

describe("normalizeStartRunRequest", () => {
  it("includes serialized profile when present", () => {
    expect(
      normalizeStartRunRequest({
        input: "hello",
        threadId: "thread-1",
        workspace: {
          workspaceRoot: "/tmp/workspace",
          writableRoots: ["/tmp/workspace"],
        },
        client: {
          sessionId: "session-1",
          requestId: "request-1",
          entrypoint: "desktop",
        },
        profile: {
          systemPrompt: "follow profile",
          maxTurns: 2,
          disallowedTools: ["shell"],
          permissionMode: "ask",
        },
      }),
    ).toEqual({
      input: "hello",
      thread_id: "thread-1",
      workspace: {
        workspace_root: "/tmp/workspace",
        writable_roots: ["/tmp/workspace"],
      },
      client: {
        session_id: "session-1",
        request_id: "request-1",
        entrypoint: "desktop",
      },
      profile: {
        system_prompt: "follow profile",
        max_turns: 2,
        disallowed_tools: ["shell"],
        permission_mode: "ask",
      },
      policy_options: undefined,
      attachments: undefined,
    });
  });

  it("serializes Wave 3 policy options and local image attachments", () => {
    expect(
      normalizeStartRunRequest({
        input: "look",
        workspace: {
          workspaceRoot: "/tmp/workspace",
          writableRoots: ["/tmp/workspace"],
        },
        client: {
          sessionId: "session-1",
          requestId: "request-1",
          entrypoint: "desktop",
        },
        policyOptions: {
          busyInputMode: "queue",
          backgroundNotificationMode: "error",
          imageInputMode: "native",
        },
        attachments: [{ path: "/tmp/workspace/a.png", type: "image" }],
      }),
    ).toMatchObject({
      policy_options: {
        busy_input_mode: "queue",
        background_notification_mode: "error",
        image_input_mode: "native",
      },
      attachments: [{ path: "/tmp/workspace/a.png", type: "image" }],
    });
  });

  it("omits profile when it is missing", () => {
    expect(
      normalizeStartRunRequest({
        input: "hello",
        workspace: {
          workspaceRoot: "/tmp/workspace",
          writableRoots: ["/tmp/workspace"],
        },
        client: {
          sessionId: "session-1",
          requestId: "request-1",
          entrypoint: "desktop",
        },
      }),
    ).toEqual({
      input: "hello",
      thread_id: undefined,
      workspace: {
        workspace_root: "/tmp/workspace",
        writable_roots: ["/tmp/workspace"],
      },
      client: {
        session_id: "session-1",
        request_id: "request-1",
        entrypoint: "desktop",
      },
      profile: undefined,
      policy_options: undefined,
      attachments: undefined,
    });
  });
});

describe("Wave 1 runtime normalizers", () => {
  it("normalizes thread metadata, state, and pending action", () => {
    expect(
      normalizeRuntimeThread({
        thread_id: "thread-1",
        label: "Generated title",
        state: "requires_action",
        pending_action: {
          ToolName: "apply_patch",
          ActionDescription: "Approve patch",
          ToolUseID: "tool-1",
          RequestID: "approval-1",
          Input: { title: "Approve patch" },
        },
        metadata: {
          session_id: "thread-1",
          ai_title: "Generated title",
          last_prompt: "Fix tests",
          task_summary: "Tests are fixed",
          branch: {
            original_thread_id: "thread-0",
            title: "Fix tests (Branch)",
          },
        },
        workspace: {
          workspace_root: "/tmp/workspace",
          writable_roots: ["/tmp/workspace"],
        },
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:01:00Z",
      }),
    ).toMatchObject({
      threadId: "thread-1",
      state: "requires_action",
      pendingAction: {
        toolName: "apply_patch",
        requestId: "approval-1",
      },
      metadata: {
        aiTitle: "Generated title",
        lastPrompt: "Fix tests",
        taskSummary: "Tests are fixed",
        branch: {
          originalThreadId: "thread-0",
          title: "Fix tests (Branch)",
        },
      },
    });
  });

  it("normalizes lite, context, and branch request payloads", () => {
    expect(
      normalizeRuntimeThreadLite({
        thread_id: "thread-1",
        label: "Thread",
        state: "idle",
        head: "{",
        tail: "}",
        size: 2,
      }),
    ).toEqual({
      threadId: "thread-1",
      label: "Thread",
      state: "idle",
      head: "{",
      tail: "}",
      size: 2,
    });

    expect(
      normalizeRuntimeThreadContext({
        thread_id: "thread-1",
        files: { markdown: "Files in context", files: [] },
        usage: {
          markdown: "Context Usage",
          model: "test-model",
          model_display_name: "Test Model",
          total_tokens: 10,
          max_tokens: 100,
          percentage: 10,
          message_count: 2,
        },
      }),
    ).toMatchObject({
      threadId: "thread-1",
      usage: {
        totalTokens: 10,
        modelDisplayName: "Test Model",
        maxTokens: 100,
        messageCount: 2,
      },
    });

    expect(normalizeBranchThreadRequest({ forkSessionId: "thread-2", customTitle: "Branch" })).toEqual({
      fork_session_id: "thread-2",
      custom_title: "Branch",
    });
  });
});
