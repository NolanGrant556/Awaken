import { type ComposerBarStructuredInputValue } from "@awaken/design-system";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { FileLineComment, RuntimePlugin } from "../shared/runtime";
import {
  MAX_COMPOSER_IMAGE_BYTES,
  SUPPORTED_COMPOSER_IMAGE_MIME_TYPES,
  composerDraftAttachmentFromPickedFile,
  readComposerDraftImageAttachment,
} from "./composerAttachments";
import {
  actionsFromSlashProviders,
  activeComposerPaletteActions,
  builtinMentionActions,
  composerActionTriggerFromDraft,
  composerPaletteActions,
  composerTokenFromSlashAction,
  firstSelectableSlashAction,
  pluginMentionActionsFromPlugins,
  skillSlashProvider,
  type SlashAction,
} from "./composerSlash";
import {
  composerStructuredDraftLength,
  insertComposerTokenAtOffset,
  reconcileComposerStructuredInput,
  removeComposerTextRange,
  replaceComposerActionTriggerWithToken,
} from "./composerStructuredDraft";
import { composerTokenEndOffset } from "./promptPresentation";
import type { RuntimeClient } from "./runtime/client";
import { useThreadComposerDraft } from "./useThreadComposerDraft";
type Options = {
  runtimeClient: RuntimeClient;
  desktopReady: boolean;
  isBusy: boolean;
  isWorkspaceValid: boolean;
  workspaceRootForRun: string;
  defaultWorkspacePath: string;
  disabled: boolean;
  draft: ReturnType<typeof useThreadComposerDraft>;
  requestComposerFocus: (offset?: number) => void;
  setNotice: (message: string | null) => void;
};
export function useComposerInput(options: Options) {
  const {
    runtimeClient,
    desktopReady,
    isBusy,
    isWorkspaceValid,
    workspaceRootForRun,
    defaultWorkspacePath,
    requestComposerFocus,
    setNotice,
  } = options;
  const composerBarState = { disabled: options.disabled };
  const {
    draft,
    setValue: setComposerValue,
    setTokens: setComposerTokens,
    setTokenLeadingTexts: setComposerTokenLeadingTexts,
    setTokenPlacements: setComposerTokenPlacements,
    setSelection: setComposerInputSelection,
    setAttachments: setComposerAttachments,
    setComments: setComposerComments,
  } = options.draft;
  const {
    value: composerValue,
    tokens: composerTokens,
    tokenLeadingTexts: composerTokenLeadingTexts,
    tokenPlacements: composerTokenPlacements,
    selection: composerInputSelection,
    attachments: composerAttachments,
    comments: composerComments,
  } = draft;

  const slashProviders = useMemo(
    () => [
      skillSlashProvider((workspaceRoot) =>
        runtimeClient.listSkills(workspaceRoot),
      ),
    ],
    [runtimeClient],
  );

  const builtinMentionCatalog = useMemo(() => builtinMentionActions(), []);

  const [slashActions, setSlashActions] = useState<SlashAction[]>([]);

  const [slashLoading, setSlashLoading] = useState(false);

  const [slashError, setSlashError] = useState<string | null>(null);

  const [slashActiveActionId, setSlashActiveActionId] = useState<
    string | undefined
  >();

  const [composerPluginCatalog, setComposerPluginCatalog] = useState<
    RuntimePlugin[]
  >([]);

  const composerPluginCatalogRef = useRef<RuntimePlugin[]>([]);

  const composerPluginActions = useMemo(
    () => pluginMentionActionsFromPlugins(composerPluginCatalog),
    [composerPluginCatalog],
  );

  const composerPluginBrandByName = useMemo(
    () =>
      new Map(
        composerPluginCatalog.flatMap((plugin) =>
          plugin.brandIcon ? [[plugin.name, plugin.brandIcon] as const] : [],
        ),
      ),
    [composerPluginCatalog],
  );

  const immediateMentionActions = useMemo(
    () => [...builtinMentionCatalog, ...composerPluginActions],
    [builtinMentionCatalog, composerPluginActions],
  );

  const [composerPluginsLoading, setComposerPluginsLoading] = useState(false);

  const [composerPluginsError, setComposerPluginsError] = useState<
    string | null
  >(null);

  const [composerCatalogRevision, setComposerCatalogRevision] = useState(0);

  useEffect(
    () =>
      runtimeClient.subscribeComposerCatalogEvents((event) => {
        setComposerCatalogRevision((current) =>
          Math.max(current, event.revision),
        );
      }),
    [runtimeClient],
  );

  const composerInlineTextPrefix = useMemo(
    () =>
      composerTokens
        .map(
          (token) =>
            token.leadingText ?? composerTokenLeadingTexts[token.id] ?? "",
        )
        .join(""),
    [composerTokenLeadingTexts, composerTokens],
  );

  const trimmedEffectiveComposerValue =
    `${composerInlineTextPrefix}${composerValue}`.trim();

  const composerActionTrigger = composerActionTriggerFromDraft({
    value: composerValue,
    tokens: composerTokens,
    leadingTexts: composerTokenLeadingTexts,
    selection: composerInputSelection,
  });

  const slashPaletteOpen =
    Boolean(composerActionTrigger) &&
    desktopReady &&
    isWorkspaceValid &&
    !isBusy;

  const activePaletteActions = activeComposerPaletteActions(
    composerActionTrigger?.marker,
    slashActions,
    immediateMentionActions,
  );

  const filteredSlashActions = useMemo(
    () =>
      composerPaletteActions(
        activePaletteActions,
        composerActionTrigger?.query ?? "",
      ),
    [activePaletteActions, composerActionTrigger?.query],
  );

  const composerBarTokens = useMemo(
    () =>
      composerTokens.map((token) => ({
        id: token.id,
        type: token.type,
        label: token.label,
        placement: composerTokenPlacements[token.id] ?? "before",
        leadingText: token.leadingText ?? composerTokenLeadingTexts[token.id],
        title:
          typeof token.payload?.skillFilePath === "string"
            ? token.payload.skillFilePath
            : typeof token.payload?.pluginName === "string"
              ? token.payload.pluginName
              : token.label,
        brandIcon:
          token.type === "plugin" &&
          typeof token.payload?.pluginName === "string"
            ? composerPluginBrandByName.get(token.payload.pluginName)
            : undefined,
      })),
    [
      composerPluginBrandByName,
      composerTokenLeadingTexts,
      composerTokenPlacements,
      composerTokens,
    ],
  );

  const composerBarSlashActions = useMemo(
    () =>
      filteredSlashActions.map(
        ({
          id,
          type,
          label,
          sectionLabel,
          description,
          icon,
          disabled,
          disabledReason,
          preview,
        }) => ({
          id,
          type,
          label,
          sectionLabel,
          description,
          icon,
          disabled,
          disabledReason,
          preview,
        }),
      ),
    [filteredSlashActions],
  );

  const composerBarSlashPalette = {
    open: slashPaletteOpen,
    actions: composerBarSlashActions,
    activeActionId: slashActiveActionId,
    loading: slashLoading,
    error: slashError,
    emptyLabel: "没有匹配项",
    onSelectAction: handleSlashActionSelect,
    onActiveActionChange: setSlashActiveActionId,
  };

  const composerBarPluginPicker = {
    label: "插件",
    emptyLabel: "没有可用插件",
    loading: composerPluginsLoading,
    error: composerPluginsError,
    disabled: composerBarState.disabled,
    options: composerPluginActions.map((action) => ({
      id: action.id,
      label: action.label,
      sectionLabel: action.sectionLabel,
      icon: action.icon,
      disabled: action.disabled,
      disabledReason: action.disabledReason,
      selected: composerTokens.some(
        (token) => token.id === action.composerToken?.id,
      ),
    })),
    onSelectOption: handleComposerPluginSelect,
  };

  useEffect(() => {
    if (!slashPaletteOpen || composerActionTrigger?.marker !== "/") {
      setSlashLoading(false);
      setSlashError(null);
      setSlashActiveActionId(undefined);
      return;
    }

    let cancelled = false;
    setSlashLoading(true);
    setSlashError(null);
    setSlashActions([]);
    void actionsFromSlashProviders(slashProviders, { workspaceRootForRun })
      .then((actions) => {
        if (cancelled) {
          return;
        }
        setSlashActions(actions);
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }
        setSlashActions([]);
        setSlashError(
          error instanceof Error ? error.message : "命令菜单加载失败",
        );
      })
      .finally(() => {
        if (!cancelled) {
          setSlashLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [
    composerCatalogRevision,
    composerActionTrigger?.marker,
    slashPaletteOpen,
    slashProviders,
    workspaceRootForRun,
  ]);

  useEffect(() => {
    if (!slashPaletteOpen) {
      return;
    }
    const activeStillVisible = filteredSlashActions.some(
      (action) => action.id === slashActiveActionId,
    );
    if (!activeStillVisible) {
      setSlashActiveActionId(
        firstSelectableSlashAction(filteredSlashActions)?.id,
      );
    }
  }, [filteredSlashActions, slashActiveActionId, slashPaletteOpen]);

  useEffect(() => {
    if (!desktopReady) {
      setComposerPluginsLoading(false);
      setComposerPluginsError(null);
      return;
    }

    let cancelled = false;
    const hasSnapshot = composerPluginCatalogRef.current.length > 0;
    setComposerPluginsLoading(!hasSnapshot);
    setComposerPluginsError(null);
    void runtimeClient
      .listPlugins()
      .then((plugins) => {
        if (!cancelled) {
          composerPluginCatalogRef.current = plugins;
          setComposerPluginCatalog(plugins);
        }
      })
      .catch((error) => {
        if (!cancelled && !hasSnapshot) {
          setComposerPluginsError(
            error instanceof Error ? error.message : "插件列表加载失败",
          );
        }
      })
      .finally(() => {
        if (!cancelled) {
          setComposerPluginsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [composerCatalogRevision, desktopReady, runtimeClient]);

  function handleSlashActionSelect(actionId: string): void {
    const action = filteredSlashActions.find((item) => item.id === actionId);
    if (!action || action.disabled) {
      return;
    }
    const token = composerTokenFromSlashAction(action);
    if (!token) {
      return;
    }
    if (!composerActionTrigger) {
      return;
    }
    const nextDraft = replaceComposerActionTriggerWithToken({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      trigger: composerActionTrigger,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    setSlashActiveActionId(undefined);
    requestComposerFocus(nextDraft.focusOffset);
  }

  function handleComposerPluginSelect(actionId: string): void {
    const action = composerPluginActions.find((item) => item.id === actionId);
    if (!action || action.disabled) {
      return;
    }
    const token = composerTokenFromSlashAction(action);
    if (!token) {
      return;
    }

    const existingTokenOffset = composerTokenEndOffset(
      composerTokens,
      composerTokenLeadingTexts,
      token.id,
    );
    if (existingTokenOffset !== undefined) {
      setComposerInputSelection({
        anchor: existingTokenOffset,
        focus: existingTokenOffset,
      });
      requestComposerFocus(existingTokenOffset);
      return;
    }

    const draftLength = composerStructuredDraftLength(
      composerTokens,
      composerTokenLeadingTexts,
      composerValue,
    );
    const offset = composerInputSelection?.focus ?? draftLength;
    const nextDraft = insertComposerTokenAtOffset({
      tokens: composerTokens,
      placements: composerTokenPlacements,
      leadingTexts: composerTokenLeadingTexts,
      value: composerValue,
      offset,
      token,
    });
    setComposerTokens(nextDraft.tokens);
    setComposerTokenPlacements(nextDraft.placements);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerValue(nextDraft.value);
    setComposerInputSelection({
      anchor: nextDraft.focusOffset,
      focus: nextDraft.focusOffset,
    });
    requestComposerFocus(nextDraft.focusOffset);
  }

  function handleRemoveComposerToken(tokenId: string): void {
    const removedTokenIndex = composerTokens.findIndex(
      (item) => item.id === tokenId,
    );
    const removedToken = composerTokens[removedTokenIndex];
    const leadingText =
      removedToken?.leadingText ?? composerTokenLeadingTexts[tokenId] ?? "";
    const nextToken = composerTokens[removedTokenIndex + 1];
    if (leadingText && nextToken) {
      const nextLeadingText =
        nextToken.leadingText ?? composerTokenLeadingTexts[nextToken.id] ?? "";
      const mergedLeadingText = `${leadingText}${nextLeadingText}`;
      setComposerTokens((current) =>
        current
          .filter((token) => token.id !== tokenId)
          .map((token) =>
            token.id === nextToken.id
              ? { ...token, leadingText: mergedLeadingText }
              : token,
          ),
      );
      setComposerTokenLeadingTexts((current) => {
        const next = { ...current, [nextToken.id]: mergedLeadingText };
        delete next[tokenId];
        return next;
      });
    } else if (leadingText) {
      setComposerValue((current) => `${leadingText}${current}`);
      setComposerTokens((current) =>
        current.filter((token) => token.id !== tokenId),
      );
    } else {
      setComposerTokens((current) =>
        current.filter((token) => token.id !== tokenId),
      );
    }
    setComposerTokenPlacements((current) => {
      if (!(tokenId in current)) {
        return current;
      }
      const next = { ...current };
      delete next[tokenId];
      return next;
    });
    if (!nextToken) {
      setComposerTokenLeadingTexts((current) => {
        if (!(tokenId in current)) {
          return current;
        }
        const next = { ...current };
        delete next[tokenId];
        return next;
      });
    }
    requestComposerFocus();
  }

  function handleComposerStructuredInputChange(
    nextValue: ComposerBarStructuredInputValue,
  ): void {
    const nextDraft = reconcileComposerStructuredInput(
      composerTokens,
      composerTokenPlacements,
      nextValue,
    );
    setComposerValue(nextDraft.value);
    setComposerTokens(nextDraft.tokens);
    setComposerTokenLeadingTexts(nextDraft.leadingTexts);
    setComposerTokenPlacements(nextDraft.placements);
  }

  function handleComposerInputKeyDown(
    event: ReactKeyboardEvent<HTMLTextAreaElement | HTMLDivElement>,
  ): void {
    if (!slashPaletteOpen) {
      return;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      if (composerActionTrigger) {
        const nextDraft = removeComposerTextRange({
          tokens: composerTokens,
          placements: composerTokenPlacements,
          leadingTexts: composerTokenLeadingTexts,
          value: composerValue,
          start: composerActionTrigger.start,
          end: composerActionTrigger.end,
        });
        setComposerTokens(nextDraft.tokens);
        setComposerTokenPlacements(nextDraft.placements);
        setComposerTokenLeadingTexts(nextDraft.leadingTexts);
        setComposerValue(nextDraft.value);
        setComposerInputSelection({
          anchor: composerActionTrigger.start,
          focus: composerActionTrigger.start,
        });
        requestComposerFocus(composerActionTrigger.start);
      }
      setSlashActiveActionId(undefined);
      return;
    }
    if (
      event.key !== "ArrowDown" &&
      event.key !== "ArrowUp" &&
      event.key !== "Enter" &&
      event.key !== "Tab"
    ) {
      return;
    }
    event.preventDefault();
    const selectableActions = filteredSlashActions.filter(
      (action) => !action.disabled,
    );
    if (selectableActions.length === 0) {
      return;
    }
    const activeIndex = Math.max(
      0,
      selectableActions.findIndex(
        (action) => action.id === slashActiveActionId,
      ),
    );
    if (event.key === "ArrowDown") {
      setSlashActiveActionId(
        selectableActions[(activeIndex + 1) % selectableActions.length]?.id,
      );
      return;
    }
    if (event.key === "ArrowUp") {
      setSlashActiveActionId(
        selectableActions[
          (activeIndex - 1 + selectableActions.length) %
            selectableActions.length
        ]?.id,
      );
      return;
    }
    handleSlashActionSelect(
      slashActiveActionId ?? selectableActions[0]?.id ?? "",
    );
  }

  async function handleAddContext(): Promise<void> {
    if (composerBarState.disabled) {
      return;
    }
    setNotice(null);
    try {
      const result = await runtimeClient.pickAttachments(
        workspaceRootForRun || defaultWorkspacePath,
      );
      if (result.canceled) {
        if (result.error) {
          setNotice(result.error);
        }
        return;
      }
      const nextAttachments = result.files.map(
        composerDraftAttachmentFromPickedFile,
      );
      setComposerAttachments((current) => [...current, ...nextAttachments]);
      if (result.errors?.length) {
        setNotice(result.errors.slice(0, 3).join("；"));
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "添加附件失败");
    }
  }

  async function handlePasteImages(files: File[]): Promise<void> {
    if (files.length === 0 || composerBarState.disabled) {
      return;
    }
    setNotice(null);
    const unsupportedFile = files.find(
      (file) =>
        !SUPPORTED_COMPOSER_IMAGE_MIME_TYPES.includes(
          file.type as (typeof SUPPORTED_COMPOSER_IMAGE_MIME_TYPES)[number],
        ),
    );
    if (unsupportedFile) {
      setNotice("仅支持粘贴 PNG、JPEG 或 WebP 图片");
      return;
    }
    const oversizedFile = files.find(
      (file) => file.size > MAX_COMPOSER_IMAGE_BYTES,
    );
    if (oversizedFile) {
      setNotice(
        `单张图片不能超过 ${MAX_COMPOSER_IMAGE_BYTES / (1024 * 1024)} MiB`,
      );
      return;
    }

    try {
      const startIndex = composerAttachments.length;
      const nextAttachments = await Promise.all(
        files.map((file, index) =>
          readComposerDraftImageAttachment(file, startIndex + index),
        ),
      );
      setComposerAttachments((current) => [...current, ...nextAttachments]);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "粘贴图片失败");
    }
  }

  function handleRemoveComposerAttachment(attachmentId: string): void {
    setComposerAttachments((current) =>
      current.filter((attachment) => attachment.id !== attachmentId),
    );
    setNotice(null);
  }

  function handleCreateFileLineComment(comment: FileLineComment): void {
    if (composerComments.length >= 20) {
      setNotice("每次任务最多添加 20 条文件评论");
      return;
    }
    setComposerComments((current) => {
      if (current.some((item) => item.id === comment.id)) return current;
      return [...current, { ...comment, validationState: "ready" }];
    });
    setNotice(null);
  }

  function handleAddFileTextSelection(selection: FileLineComment): void {
    handleCreateFileLineComment({
      ...selection,
      kind: "selection",
      comment: "",
    });
  }

  function handleRemoveFileLineComment(commentId: string): void {
    setComposerComments((current) =>
      current.filter((comment) => comment.id !== commentId),
    );
    setNotice(null);
  }
  return {
    slashProviders,
    builtinMentionCatalog,
    slashActions,
    setSlashActions,
    slashLoading,
    setSlashLoading,
    slashError,
    setSlashError,
    slashActiveActionId,
    setSlashActiveActionId,
    composerPluginCatalog,
    setComposerPluginCatalog,
    composerPluginCatalogRef,
    composerPluginActions,
    composerPluginBrandByName,
    immediateMentionActions,
    composerPluginsLoading,
    setComposerPluginsLoading,
    composerPluginsError,
    setComposerPluginsError,
    composerCatalogRevision,
    setComposerCatalogRevision,
    composerInlineTextPrefix,
    trimmedEffectiveComposerValue,
    composerActionTrigger,
    slashPaletteOpen,
    activePaletteActions,
    filteredSlashActions,
    composerBarTokens,
    composerBarSlashActions,
    composerBarSlashPalette,
    composerBarPluginPicker,
    handleSlashActionSelect,
    handleComposerPluginSelect,
    handleRemoveComposerToken,
    handleComposerStructuredInputChange,
    handleComposerInputKeyDown,
    handleAddContext,
    handlePasteImages,
    handleRemoveComposerAttachment,
    handleCreateFileLineComment,
    handleAddFileTextSelection,
    handleRemoveFileLineComment,
  };
}
