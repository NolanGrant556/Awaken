import "./AwakenAssistantMessage.css";
import { useEffect, useRef, useState, type ComponentType } from "react";
import { AwakenButton } from "../AwakenButton/AwakenButton";
import { AwakenAssistantMessage } from "./AwakenAssistantMessage";
import { AwakenAssistantMessageStates } from "./AwakenAssistantMessage.states";

export function AwakenAssistantMessageDemo() {
  const [theme, setTheme] = useState<"light" | "dark">("light");
  const originalThemeRef = useRef<string | undefined>(undefined);
  const Component = AwakenAssistantMessage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenAssistantMessageStates[0]?.props ?? {};

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
    <section className="ads-component-demo" aria-label="助手消息 AwakenAssistantMessage 组件演示">
      <header className="ads-component-demo-header">
        <h2>助手消息 <code>AwakenAssistantMessage</code></h2>
        <p>用于渲染助手输出的富 Markdown 回答，包括 GFM、带语言标签与语法着色的代码块、文章式表格、Mermaid 图表和运行状态样式。</p>
        <div className="ads-component-demo-actions" role="group" aria-label="助手消息主题预览">
          <AwakenButton size="sm" variant={theme === "light" ? "default" : "ghost"} onClick={() => setTheme("light")}>浅色</AwakenButton>
          <AwakenButton size="sm" variant={theme === "dark" ? "default" : "ghost"} onClick={() => setTheme("dark")}>深色</AwakenButton>
        </div>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenAssistantMessageStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <Component {...(defaultProps as Record<string, unknown>)} {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
