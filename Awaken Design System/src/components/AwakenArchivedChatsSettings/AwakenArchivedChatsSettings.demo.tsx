import { useEffect, useMemo, useRef, useState } from "react";
import "./AwakenArchivedChatsSettings.demo.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import {
  AwakenArchivedChatsSettings,
  type AwakenArchivedChatGroup,
} from "./AwakenArchivedChatsSettings";
import {
  AwakenArchivedChatsSettingsSampleGroups,
  AwakenArchivedChatsSettingsSampleProjectOptions,
  AwakenArchivedChatsSettingsStates,
} from "./AwakenArchivedChatsSettings.states";

export function AwakenArchivedChatsSettingsDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const [groups, setGroups] = useState<AwakenArchivedChatGroup[]>(
    AwakenArchivedChatsSettingsSampleGroups,
  );
  const [query, setQuery] = useState("");
  const [projectFilter, setProjectFilter] = useState("all");
  const originalThemeRef = useRef<string | null>(null);

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", theme);
  }, [theme]);

  useEffect(() => () => {
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  const visibleGroups = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    return groups
      .filter((group) => projectFilter === "all" || group.id === projectFilter)
      .map((group) => ({
        ...group,
        rows: group.rows.filter((row) =>
          normalizedQuery === "" || row.title.toLocaleLowerCase().includes(normalizedQuery),
        ),
      }))
      .filter((group) => group.rows.length > 0);
  }, [groups, projectFilter, query]);

  function removeRow(threadId: string): void {
    setGroups((current) =>
      current
        .map((group) => ({
          ...group,
          rows: group.rows.filter((row) => row.threadId !== threadId),
        }))
        .filter((group) => group.rows.length > 0),
    );
  }

  return (
    <section className="ads-component-demo" aria-label="已归档的聊天设置 AwakenArchivedChatsSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>已归档的聊天 <code>AwakenArchivedChatsSettings</code></h2>
        <p>归档会话的项目分组列表；仅注入 mock 数据，搜索/筛选/取消归档/删除都只改本地状态。</p>
      </header>
      <div className="aw-archived-chats-settings-demo__controls">
        <span>主题预览</span>
        <div className="aw-archived-chats-settings-demo__theme-toggle" role="group" aria-label="主题预览">
          <AwakenButton size="sm" variant={theme === "light" ? "default" : "ghost"} onClick={() => setTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={theme === "dark" ? "default" : "ghost"} onClick={() => setTheme("dark")}>深色</AwakenButton>
        </div>
      </div>
      <AwakenSettingsSectionPreview activeSectionId="archived-chats" sectionTitle="已归档的聊天">
        <AwakenArchivedChatsSettings
          groups={visibleGroups}
          query={query}
          onQueryChange={setQuery}
          projectFilterOptions={AwakenArchivedChatsSettingsSampleProjectOptions}
          projectFilterValue={projectFilter}
          onProjectFilterChange={setProjectFilter}
          filtersActive={query.trim() !== "" || projectFilter !== "all"}
          onUnarchive={removeRow}
          onDelete={removeRow}
          onClearFilters={() => {
            setQuery("");
            setProjectFilter("all");
          }}
        />
      </AwakenSettingsSectionPreview>
      <div className="ads-component-demo-grid">
        {AwakenArchivedChatsSettingsStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.caption}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenArchivedChatsSettings
                groups={state.props.groups ?? []}
                query={state.props.query ?? ""}
                projectFilterOptions={
                  state.props.projectFilterOptions ?? AwakenArchivedChatsSettingsSampleProjectOptions
                }
                projectFilterValue={state.props.projectFilterValue ?? "all"}
                filtersActive={state.props.filtersActive}
                busyThreadIds={state.props.busyThreadIds}
                loading={state.props.loading}
                errorMessage={state.props.errorMessage}
                onQueryChange={() => undefined}
                onProjectFilterChange={() => undefined}
                onUnarchive={() => undefined}
                onDelete={() => undefined}
                onRetry={() => undefined}
                onClearFilters={() => undefined}
              />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
