import { describe, expect, it } from "vitest";

import { normalizeRunProfile, normalizeStartRunRequest } from "./runtime.js";

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
    });
  });
});
