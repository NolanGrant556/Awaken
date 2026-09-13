import "./AwakenToolTraceGroup.css";
import { useEffect, useRef, useState, type ComponentType } from "react";
import { AwakenButton } from "../AwakenButton/AwakenButton";
import { AwakenDisclosure } from "../AwakenDisclosure/AwakenDisclosure";
import { AwakenToolTraceLine } from "../AwakenToolTraceLine/AwakenToolTraceLine";
import { AwakenToolTraceLineStates } from "../AwakenToolTraceLine/AwakenToolTraceLine.states";
import { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";
import { AwakenToolTraceGroupStates } from "./AwakenToolTraceGroup.states";

export function AwakenToolTraceGroupDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const originalThemeRef = useRef<string | undefined>(undefined);
  const GroupComponent = AwakenToolTraceGroup as unknown as ComponentType<Record<string, unknown>>;
  const LineComponent = AwakenToolTraceLine as unknown as ComponentType<Record<string, unknown>>;

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === undefined) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", theme);
    return () => {
      if (originalThemeRef.current) root.setAttribute("data-aw-theme", originalThemeRef.current);
      else root.removeAttribute("data-aw-theme");
    };
  }, [theme]);

  return (
    <section className="ads-component-demo" aria-label="工具轨迹组件库演示">
      <header className="ads-component-demo-header">
        <h2>
          工具轨迹 <code>AwakenDisclosure + AwakenToolTraceGroup + AwakenToolTraceLine</code>
        </h2>
        <p>
          先按真实 Desktop 终态展示“执行时间”如何兼任过程 disclosure 并收纳活动批次，再集中观测批次摘要、
          单次工具与类型化详情卡。活动组 Chevron 常驻，单条工具 Chevron 仅在悬停、
          键盘聚焦或展开时可见；当前活动使用轻量文字与三点反馈。
        </p>
        <div className="ads-component-demo-actions" role="group" aria-label="工具轨迹主题预览">
          <AwakenButton size="sm" variant={theme === "light" ? "default" : "ghost"} onClick={() => setTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={theme === "dark" ? "default" : "ghost"} onClick={() => setTheme("dark")}>深色</AwakenButton>
        </div>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state">
          <h3>执行时间/过程 · 默认收起</h3>
          <div className="ads-component-demo-canvas">
            <AwakenDisclosure title="执行时间 17 秒" defaultOpen={false}>
              <GroupComponent
                {...(AwakenToolTraceGroupStates[0].props as Record<string, unknown>)}
                animateItems={false}
              />
            </AwakenDisclosure>
          </div>
        </article>
        <article className="ads-component-demo-state">
          <h3>执行时间/过程 · 已展开</h3>
          <div className="ads-component-demo-canvas">
            <AwakenDisclosure title="执行时间 17 秒" defaultOpen>
              <GroupComponent
                {...(AwakenToolTraceGroupStates[0].props as Record<string, unknown>)}
                animateItems={false}
              />
            </AwakenDisclosure>
          </div>
        </article>
        {AwakenToolTraceGroupStates.map((state) => (
          <article key={`group:${state.id}`} className="ads-component-demo-state">
            <h3>工具轨迹组 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <GroupComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
        {AwakenToolTraceLineStates.map((state) => (
          <article key={`line:${state.id}`} className="ads-component-demo-state">
            <h3>工具轨迹行 · {state.label}</h3>
            <div className="ads-component-demo-canvas">
              <LineComponent {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
