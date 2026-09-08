import { AwakenComposerBar as ComposerBar } from "@awaken/design-system";
import type { ComponentProps } from "react";
import { composerAttachmentViews } from "./composerAttachments";
import type { useComposerInput } from "./useComposerInput";
import type { useComposerLifecycle } from "./useComposerLifecycle";
import type { useThreadComposerDraft } from "./useThreadComposerDraft";

type Props = {
  variant: "center" | "dock";
  draft: Pick<
    ReturnType<typeof useThreadComposerDraft>,
    "draft" | "setValue" | "setSelection" | "setComments"
  >;
  input: Pick<
    ReturnType<typeof useComposerInput>,
    | "handleComposerStructuredInputChange"
    | "handleAddContext"
    | "handleComposerInputKeyDown"
    | "composerBarTokens"
    | "handleRemoveComposerToken"
    | "composerBarSlashPalette"
    | "composerBarPluginPicker"
    | "handleRemoveComposerAttachment"
    | "handleRemoveFileLineComment"
    | "handlePasteImages"
  >;
  lifecycle: Pick<
    ReturnType<typeof useComposerLifecycle>,
    "composerBarState" | "composerFocusRequest" | "composerFocusOffset"
  >;
  editing: boolean;
  onSubmit: () => Promise<void>;
  metaItems: ComponentProps<typeof ComposerBar>["metaItems"];
  projectPicker?: ComponentProps<typeof ComposerBar>["projectPicker"];
};

export function ComposerSurface({
  variant,
  draft,
  input,
  lifecycle,
  editing,
  onSubmit,
  metaItems,
  projectPicker,
}: Props) {
  const { composerBarState, composerFocusRequest, composerFocusOffset } =
    lifecycle;
  return (
    <ComposerBar
      value={draft.draft.value}
      onChange={draft.setValue}
      onStructuredInputChange={input.handleComposerStructuredInputChange}
      onSubmit={() => {
        void onSubmit();
      }}
      onAddContext={input.handleAddContext}
      placeholder={
        variant === "center"
          ? "描述任务，或 / 选择 Skill、@ 选择内置功能或插件"
          : "输入后续任务…"
      }
      disabled={composerBarState.disabled || editing}
      isReady={composerBarState.isReady}
      submitMode={composerBarState.submitMode}
      submitDisabled={composerBarState.submitDisabled || editing}
      variant={variant}
      metaItems={metaItems}
      projectPicker={projectPicker}
      focusRequest={composerFocusRequest}
      focusOffset={composerFocusOffset}
      onInputSelectionChange={({ anchor, focus }) =>
        draft.setSelection({ anchor, focus })
      }
      onInputKeyDown={input.handleComposerInputKeyDown}
      tokens={input.composerBarTokens}
      onRemoveToken={input.handleRemoveComposerToken}
      slashPalette={input.composerBarSlashPalette}
      pluginPicker={input.composerBarPluginPicker}
      attachments={composerAttachmentViews(draft.draft.attachments)}
      onRemoveAttachment={input.handleRemoveComposerAttachment}
      fileComments={draft.draft.comments}
      onRemoveFileComment={input.handleRemoveFileLineComment}
      onClearFileComments={() => draft.setComments([])}
      onPasteImages={(files) => {
        void input.handlePasteImages(files);
      }}
    />
  );
}
