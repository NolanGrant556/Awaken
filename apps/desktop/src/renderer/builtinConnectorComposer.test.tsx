// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CliConnectorStateEvent, CliConnectorView, CliProviderId, RuntimeSkill } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import { builtinConnectorMentionActions } from "./builtinConnectorComposer";
import { useComposerInput } from "./useComposerInput";
import { useThreadComposerDraft } from "./useThreadComposerDraft";
import { promptTokenViews } from "./promptPresentation";

const skill = (name: string): RuntimeSkill => ({ id: name, name, description: "", context: "inline", sourceOrigin: "agents", state: "loaded", rootDir: `/Users/test/.agents/skills/${name}`, skillFilePath: `/Users/test/.agents/skills/${name}/SKILL.md` });
const ready = (providerId: CliProviderId): CliConnectorView => ({ providerId, displayName: providerId, readiness: { providerId, activation: "enabled", install: "installed", executable: "available", authentication: "authorized", configuration: "ready", overall: "ready", updatedAt: "2026-09-14" } });
const skills = [skill("dws"), skill("lark-shared")];

describe("connected built-in Composer entries", () => {
  it("uses exact global Skill identities and hides unavailable or ambiguous entries", () => {
    const views = [ready("dingtalk-workspace"), ready("lark-cli")];
    const actions = builtinConnectorMentionActions(views, skills);
    expect(actions.map(a => [a.label, a.sectionLabel])).toEqual([["钉钉", "内置"], ["飞书", "内置"]]);
    expect(actions[0].composerToken).toMatchObject({ id: "builtin-connector:dingtalk-workspace", type: "skill", label: "钉钉", payload: { skillId: "dws", sourceOrigin: "agents", skillFilePath: skills[0].skillFilePath } });
    expect(builtinConnectorMentionActions([], skills)).toEqual([]);
    for (const overrides of [{ authentication: "required" }, { overall: "working" }, { activation: "disabled" }, { install: "installing" }, { executable: "missing" }]) {
      const view = ready("dingtalk-workspace"); Object.assign(view.readiness, overrides);
      expect(builtinConnectorMentionActions([view], skills)).toEqual([]);
    }
    expect(builtinConnectorMentionActions(views, [{ ...skills[0], sourceOrigin: "plugin" }, { ...skills[1], projectPath: "/project" }])).toEqual([]);
    expect(builtinConnectorMentionActions(views, [skills[0], { ...skills[0], id: "other" }])).toEqual([]);
    expect(builtinConnectorMentionActions(views, [{ ...skills[0], userInvocable: false }])).toEqual([]);
    expect(builtinConnectorMentionActions(views, [{ ...skills[0], state: "disabled_by_policy" }])).toEqual([]);
  });

  let root: Root;
  let input: ReturnType<typeof useComposerInput>;
  let draft: ReturnType<typeof useThreadComposerDraft>;
  let listener: (event: CliConnectorStateEvent) => void;
  let client: RuntimeClient;
  function Harness() {
    draft = useThreadComposerDraft("thread:fixture");
    input = useComposerInput({ runtimeClient: client, desktopReady: true, isBusy: false, isWorkspaceValid: true, workspaceRootForRun: "/fixture", defaultWorkspacePath: "/fixture", disabled: false, draft, requestComposerFocus: vi.fn(), setNotice: vi.fn() });
    return null;
  }
  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    const container = document.createElement("div"); document.body.append(container); root = createRoot(container);
    client = { listSkills: vi.fn(async () => skills), listCliConnectors: vi.fn(async () => [ready("dingtalk-workspace"), ready("lark-cli")]), listPlugins: vi.fn(async () => []), subscribeCliConnectorState: vi.fn(callback => { listener = callback; return vi.fn(); }), subscribeComposerCatalogEvents: vi.fn(() => vi.fn()) } as unknown as RuntimeClient;
  });
  afterEach(async () => { await act(async () => root.unmount()); document.body.replaceChildren(); vi.restoreAllMocks(); });

  it("shares both menus, inserts a usable Skill token, keeps brand in replay and avoids duplicates", async () => {
    await act(async () => root.render(<Harness />));
    expect(input.composerBarPluginPicker.options.map(a => a.label)).toEqual(["钉钉", "飞书"]);
    await act(async () => { draft.setValue("@"); draft.setSelection({ anchor: 1, focus: 1 }); });
    expect(input.composerBarSlashPalette.actions.filter(a => a.type === "connector").map(a => a.label)).toEqual(["钉钉", "飞书"]);
    await act(async () => input.handleSlashActionSelect("builtin-connector:dingtalk-workspace"));
    expect(draft.draft.tokens[0].payload?.skillId).toBe("dws");
    expect(input.composerBarTokens[0].brandIcon).toBe("dingtalk");
    expect(draft.draft.value).toBe("");
    await act(async () => input.handleComposerPluginSelect("builtin-connector:dingtalk-workspace"));
    expect(draft.draft.tokens).toHaveLength(1);
    await act(async () => input.handleComposerPluginSelect("builtin-connector:lark-cli"));
    expect(draft.draft.tokens[1].payload?.skillId).toBe("lark-shared");
    const replay = JSON.parse(JSON.stringify(draft.draft.tokens));
    expect(promptTokenViews(replay)?.map(t => t.brandIcon)).toEqual(["dingtalk", "feishu"]);
  });

  it("updates both open menus after logout and reconnection without destroying the draft", async () => {
    await act(async () => root.render(<Harness />));
    await act(async () => { draft.setValue("@"); draft.setSelection({ anchor: 1, focus: 1 }); });
    const disconnected = ready("dingtalk-workspace"); disconnected.readiness.authentication = "required";
    await act(async () => listener({ connector: disconnected }));
    expect(input.composerBarPluginPicker.options.map(a => a.label)).toEqual(["飞书"]);
    expect(input.composerBarSlashPalette.actions.some(a => a.label === "钉钉")).toBe(false);
    expect(draft.draft.value).toBe("@");
    await act(async () => listener({ connector: ready("dingtalk-workspace") }));
    expect(input.composerBarPluginPicker.options.map(a => a.label)).toEqual(["钉钉", "飞书"]);
  });

  it("preserves newer state events while filling untouched providers from a late snapshot", async () => {
    let resolve!: (views: CliConnectorView[]) => void;
    vi.mocked(client.listCliConnectors).mockImplementation(() => new Promise(done => { resolve = done; }));
    await act(async () => root.render(<Harness />));
    const disconnected = ready("dingtalk-workspace"); disconnected.readiness.authentication = "required";
    await act(async () => listener({ connector: disconnected }));
    await act(async () => resolve([ready("dingtalk-workspace"), ready("lark-cli")]));
    expect(input.composerBarPluginPicker.options.map(a => a.label)).toEqual(["飞书"]);
  });

  it("does not advertise connections when their state query fails", async () => {
    vi.mocked(client.listCliConnectors).mockRejectedValue(new Error("offline"));
    await act(async () => root.render(<Harness />));
    expect(input.composerBarPluginPicker.options).toEqual([]);
    expect(input.composerBarPluginPicker.error).toContain("连接状态暂不可用");
  });
});
