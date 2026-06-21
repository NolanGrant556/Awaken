import React from "react";
import { renderToString } from "react-dom/server";
import {
  AssistantMessage,
  ApprovalCard,
  ComposerDock,
  ComposerBar,
  ExecutionFeed,
  FileChangeSummary,
  LightInspectorRail,
  RunStage,
  ThoughtTrace,
  ToolTraceGroup,
  ToolTraceLine,
  UserPromptBlock,
  WorkbenchShell,
} from "../dist/index.js";

const noop = () => {};

const tabs = [
  { id: "changes", label: "Changes", badge: 1, deltaAdded: 12, deltaRemoved: 3 },
  { id: "browser", label: "Browser" },
  { id: "terminal", label: "Terminal", badge: 2 },
  { id: "files", label: "Files" },
];

const metaItems = [
  { id: "repo", label: "awaken" },
  { id: "workspace", label: "local" },
  { id: "model", label: "Composer" },
  { id: "runtime", label: "Local" },
];

function composerForState(state) {
  const isBusy = state === "running" || state === "approval";
  return React.createElement(ComposerDock, null,
    React.createElement(ComposerBar, {
      value:
        state === "done"
          ? "Follow up in the same thread"
          : state === "error"
            ? "Recover from the failed build"
            : "",
      onChange: noop,
      onSubmit: noop,
      placeholder: isBusy ? "Awaken is working..." : state === "done" ? "Send follow-up" : "Ask Awaken to work on...",
      disabled: isBusy,
      isReady: isBusy || state === "done" || state === "error",
      submitMode: isBusy ? "stop" : "send",
      variant: "dock",
      metaItems,
    }),
  );
}

function centerComposer() {
  return React.createElement("div", { className: "run-stage-center-composer" },
    React.createElement(ComposerBar, {
      value: "",
      onChange: noop,
      onSubmit: noop,
      placeholder: "Plan, Build, / for skills, @ for context",
      isReady: false,
      variant: "center",
      metaItems,
    }),
  );
}

function narrativeForState(state) {
  if (state === "idle") {
    return React.createElement(ExecutionFeed, { label: `${state} narrative` });
  }

  const children = [
    React.createElement(UserPromptBlock, {
      key: "prompt",
      text: "Refactor the desktop renderer.",
    }),
    React.createElement(ThoughtTrace, {
      key: "thought",
      text: "Checking layout boundaries.",
      status: state === "done" ? "done" : "active",
    }),
    React.createElement(ToolTraceGroup, {
      key: "exploring",
      actions: [
        {
          id: "read-file",
          title: "Read workspace file",
          detail: "packages/ui/src/components/workbench/WorkbenchShell.tsx",
          status: "done",
        },
      ],
    }),
    React.createElement(ToolTraceLine, {
      key: "tool",
      title: state === "error" ? "Shell command failed" : "Run shell command",
      detail: "npm run typecheck:ui",
      status: state === "error" ? "error" : "done",
    }),
  ];

  if (state === "approval") {
    children.push(React.createElement(ApprovalCard, {
      key: "approval",
      inline: true,
      open: true,
      title: "Approve patch",
      description: "Awaken wants to modify a workspace file.",
      affectedPaths: ["apps/desktop/src/renderer/App.tsx"],
      patchSummary: "Render approval inside the center narrative.",
      riskTone: "warning",
      onApprove: noop,
      onReject: noop,
    }));
  }

  if (state === "done") {
    children.push(React.createElement(FileChangeSummary, {
      key: "file",
      files: [
        {
          id: "shell",
          title: "WorkbenchShell.tsx",
          path: "packages/ui/src/components/workbench/WorkbenchShell.tsx",
          status: "done",
        },
      ],
      status: "done",
    }));
  }

  children.push(React.createElement(AssistantMessage, {
    key: "answer",
    markdown: state === "error" ? "The run stopped after a build error." : "Workbench layout components are ready.",
    status: state === "error" ? "error" : "done",
  }));

  return React.createElement(ExecutionFeed, { label: `${state} narrative` }, children);
}

function workbenchForState(state) {
  return React.createElement(WorkbenchShell, {
    stage: React.createElement(RunStage, { composer: state === "idle" ? undefined : composerForState(state) },
      state === "idle" ? centerComposer() : null,
      state === "idle" ? null : narrativeForState(state),
    ),
    inspector: React.createElement(LightInspectorRail, {
      title: "On Awaken",
      status: "Local",
      items: tabs,
      activeId: state === "done" || state === "error" ? "changes" : "terminal",
      onSelect: noop,
    },
      React.createElement("section", { className: "inspector-section" },
        React.createElement("h2", null, "Mock State"),
        React.createElement("p", null, "This smoke uses renderToString only."),
      ),
    ),
  });
}

for (const state of ["idle", "running", "approval", "done", "error"]) {
  const html = renderToString(workbenchForState(state));
  const required = state === "idle" ? [
    "Run stage",
    "On Awaken",
    "Plan, Build, / for skills, @ for context",
  ] : [
    "Run stage",
    "On Awaken",
    "Refactor the desktop renderer.",
    "Thought",
    "Read workspace file",
    state === "error" ? "Shell command failed" : "Run shell command",
    state === "approval" ? "Approve patch" : state === "done" ? "File changes" : "Workbench",
  ];
  for (const marker of required) {
    if (!html.includes(marker)) {
      throw new Error(`render smoke missing ${marker} for ${state}`);
    }
  }
  for (const forbidden of [
    "Ready for a " + "run",
    "No narrative " + "yet",
    "run-stage-" + "header",
  ]) {
    if (html.includes(forbidden)) {
      throw new Error(`render smoke included old marker ${forbidden} for ${state}`);
    }
  }
  if (state === "idle" && html.includes("composer-bar-dock")) {
    throw new Error("render smoke included dock composer for idle");
  }
  if (state === "done" && !html.includes("Send follow-up")) {
    throw new Error("render smoke missing follow-up composer for done");
  }
}

console.log("render smoke passed: idle/running/approval/done/error");
