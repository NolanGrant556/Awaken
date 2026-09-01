import {
  AwakenButton,
  AwakenProviderLogo,
  AwakenSelect,
  AwakenSettingsSwitch,
} from "@awaken/design-system";
import { useEffect, useMemo, useState } from "react";
import type {
  AgentPlanAccount,
  AgentPlanMediaCapability,
  AgentPlanMediaToolConfiguration,
  AgentPlanMediaTools,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import "./AgentPlanMediaToolsPanel.css";

const CAPABILITY_OPTIONS: Array<{
  capability: AgentPlanMediaCapability;
  title: string;
  modelKind: "image" | "video";
}> = [
  {
    capability: "media.image.generate",
    title: "图片生成",
    modelKind: "image",
  },
  {
    capability: "media.video.generate",
    title: "视频生成",
    modelKind: "video",
  },
];

const EMPTY_DRAFTS: Record<AgentPlanMediaCapability, string> = {
  "media.image.generate": "",
  "media.video.generate": "",
};

type Props = {
  account: AgentPlanAccount | null;
  runtimeClient: RuntimeClient;
  onConfigureAccount: () => void;
  onToast: (message: string) => void;
};

export function AgentPlanMediaToolsPanel({
  account,
  runtimeClient,
  onConfigureAccount,
  onToast,
}: Props) {
  const [settings, setSettings] = useState<AgentPlanMediaTools>({
    configurations: [],
  });
  const [drafts, setDrafts] =
    useState<Record<AgentPlanMediaCapability, string>>(EMPTY_DRAFTS);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const configuredByCapability = useMemo(
    () =>
      new Map(
        settings.configurations.map((configuration) => [
          configuration.capability,
          configuration,
        ]),
      ),
    [settings],
  );

  const modelsByKind = useMemo(
    () => ({
      image: account?.catalog.filter((entry) => entry.kind === "image") ?? [],
      video: account?.catalog.filter((entry) => entry.kind === "video") ?? [],
    }),
    [account?.catalog],
  );

  useEffect(() => {
    let cancelled = false;
    void runtimeClient
      .getAgentPlanMediaTools()
      .then((nextSettings) => {
        if (cancelled) return;
        setSettings(nextSettings);
        setDrafts({
          "media.image.generate":
            nextSettings.configurations.find(
              (configuration) =>
                configuration.capability === "media.image.generate",
            )?.modelId ?? "",
          "media.video.generate":
            nextSettings.configurations.find(
              (configuration) =>
                configuration.capability === "media.video.generate",
            )?.modelId ?? "",
        });
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setMessage(
            error instanceof Error ? error.message : "生成工具加载失败",
          );
        }
      });
    return () => {
      cancelled = true;
    };
  }, [runtimeClient]);

  async function runAction(
    key: string,
    operation: () => Promise<void>,
  ): Promise<void> {
    setBusyAction(key);
    setMessage(null);
    try {
      await operation();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "操作失败，请重试");
    } finally {
      setBusyAction(null);
    }
  }

  async function saveConfiguration(
    capability: AgentPlanMediaCapability,
    title: string,
  ): Promise<void> {
    if (!account?.hasKey) {
      onConfigureAccount();
      return;
    }
    const modelId = drafts[capability].trim();
    if (!modelId) {
      setMessage(`请选择${title}模型`);
      return;
    }

    await runAction(`${capability}:save`, async () => {
      const existing = configuredByCapability.get(capability);
      const result = await runtimeClient.saveAgentPlanMediaTool({
        capability,
        modelId,
        enabled: existing?.enabled ?? true,
      });
      setSettings(result.settings);
      if (result.applyStatus !== "applied") {
        throw new Error(result.message ?? "生成工具运行时刷新失败");
      }
      onToast(`${title}工具已保存`);
    });
  }

  async function toggleConfiguration(
    configuration: AgentPlanMediaToolConfiguration,
    enabled: boolean,
  ): Promise<void> {
    await runAction(`${configuration.capability}:toggle`, async () => {
      const result = await runtimeClient.saveAgentPlanMediaTool({
        capability: configuration.capability,
        modelId: configuration.modelId,
        enabled,
      });
      setSettings(result.settings);
      if (result.applyStatus !== "applied") {
        throw new Error(result.message ?? "生成工具运行时刷新失败");
      }
      onToast(enabled ? "工具已启用" : "工具已停用");
    });
  }

  async function deleteConfiguration(
    configuration: AgentPlanMediaToolConfiguration,
    title: string,
  ): Promise<void> {
    if (!window.confirm(`删除${title}工具？`)) return;
    await runAction(`${configuration.capability}:delete`, async () => {
      const result = await runtimeClient.deleteAgentPlanMediaTool(
        configuration.capability,
      );
      setSettings(result.settings);
      setDrafts((current) => ({
        ...current,
        [configuration.capability]: "",
      }));
      if (result.applyStatus !== "applied") {
        throw new Error(result.message ?? "生成工具运行时刷新失败");
      }
      onToast(`${title}工具已删除`);
    });
  }

  return (
    <section className="agent-plan-media-tools" aria-label="图片与视频工具">
      <header className="agent-plan-media-tools__header">
        <h3>图片与视频工具</h3>
        {!account?.hasKey ? (
          <AwakenButton
            variant="default"
            size="sm"
            onClick={onConfigureAccount}
          >
            配置账户
          </AwakenButton>
        ) : null}
      </header>

      {message ? (
        <div className="agent-plan-media-tools__message" role="alert">
          {message}
        </div>
      ) : null}

      <div className="agent-plan-media-tools__cards">
        {CAPABILITY_OPTIONS.map((option) => {
          const configuration = configuredByCapability.get(option.capability);
          const models = modelsByKind[option.modelKind];
          const actionBusy = busyAction?.startsWith(option.capability) === true;
          const toggleBusy = busyAction?.endsWith(":toggle") === true;
          const saveBusy = busyAction === `${option.capability}:save`;
          return (
            <article
              className="agent-plan-media-tools__card"
              key={option.capability}
            >
              <header className="agent-plan-media-tools__card-header">
                <span className="agent-plan-media-tools__logo">
                  <AwakenProviderLogo
                    brand="volcengine"
                    name="火山 Agent Plan"
                  />
                </span>
                <strong>火山 Agent Plan：{option.title}</strong>
                {configuration ? (
                  <AwakenSettingsSwitch
                    variant="control"
                    label={`${option.title}启用状态`}
                    checked={configuration.enabled}
                    busy={actionBusy || toggleBusy}
                    disabled={
                      !account?.hasKey ||
                      (busyAction !== null && !actionBusy && !toggleBusy)
                    }
                    onCheckedChange={(enabled) =>
                      void toggleConfiguration(configuration, enabled)
                    }
                  />
                ) : null}
              </header>

              <div className="agent-plan-media-tools__card-body">
                <div className="agent-plan-media-tools__field">
                  <span>模型</span>
                  <AwakenSelect
                    value={drafts[option.capability]}
                    disabled={
                      !account?.hasKey ||
                      models.length === 0 ||
                      (busyAction !== null && !toggleBusy)
                    }
                    ariaLabel={`${option.title}模型`}
                    placeholder={
                      !account?.hasKey
                        ? "请先配置 Agent Plan 账户"
                        : models.length === 0
                          ? "当前没有可用模型"
                          : "请选择一个模型"
                    }
                    options={models.map((model) => ({
                      value: model.id,
                      label: model.displayName
                        ? `${model.displayName} (${model.id})`
                        : model.id,
                    }))}
                    onValueChange={(value) =>
                      setDrafts((current) => ({
                        ...current,
                        [option.capability]: value,
                      }))
                    }
                  />
                </div>

                <div className="agent-plan-media-tools__actions">
                  {configuration ? (
                    <AwakenButton
                      variant="default"
                      size="sm"
                      loading={busyAction === `${option.capability}:delete`}
                      disabled={busyAction !== null}
                      onClick={() =>
                        void deleteConfiguration(configuration, option.title)
                      }
                    >
                      删除
                    </AwakenButton>
                  ) : null}
                  <AwakenButton
                    variant="primary"
                    size="sm"
                    loading={saveBusy}
                    disabled={
                      !account?.hasKey ||
                      !drafts[option.capability] ||
                      busyAction !== null
                    }
                    onClick={() =>
                      void saveConfiguration(option.capability, option.title)
                    }
                  >
                    {configuration ? "保存更改" : "保存"}
                  </AwakenButton>
                </div>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}
