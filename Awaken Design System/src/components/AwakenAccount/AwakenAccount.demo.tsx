import "./AwakenAccount.demo.css";
import { useEffect, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenProfileButton } from "../AwakenProfileButton";
import {
  AwakenProfileBalancePanel,
  AwakenProfileBalancePanelStates,
  AwakenProfileBalanceSamples,
} from "../AwakenProfileBalancePanel";
import { AwakenProfileMenuPanel } from "../AwakenProfileMenuPanel";
import { AwakenAccount } from "./AwakenAccount";

export function AwakenAccountDemo() {
  const [themePreference, setThemePreference] = useState<"system" | "light" | "dark">("light");
  const [accountOpen, setAccountOpen] = useState(true);
  const [panelStateId, setPanelStateId] = useState("available");
  const [lastAction, setLastAction] = useState("点击 Nolan 可展开个人菜单，再进入独立用量窗口。");
  const originalThemeRef = useRef<string | null>(null);
  const panelState =
    AwakenProfileBalancePanelStates.find((state) => state.id === panelStateId) ??
    AwakenProfileBalancePanelStates[0];

  useEffect(() => {
    const root = document.documentElement;
    if (originalThemeRef.current === null) {
      originalThemeRef.current = root.getAttribute("data-aw-theme") ?? "";
    }
    root.setAttribute("data-aw-theme", themePreference === "dark" ? "dark" : "light");
  }, [themePreference]);

  useEffect(() => () => {
    if (originalThemeRef.current) {
      document.documentElement.setAttribute("data-aw-theme", originalThemeRef.current);
    } else {
      document.documentElement.removeAttribute("data-aw-theme");
    }
  }, []);

  return (
    <section className="ads-component-demo aw-account-demo" aria-label="「账户」组件库 AwakenAccount 演示">
      <header className="ads-component-demo-header">
        <h2>「账户」组件库 <code>AwakenAccount + AwakenPopover</code></h2>
        <p>集中管理左下角个人入口、个人菜单、独立用量窗口与设置动作，不归属于左侧栏组件目录。</p>
      </header>

      <div className="aw-account-demo__controls">
        <p>{lastAction}</p>
        <div className="aw-account-demo__theme-toggle" role="group" aria-label="主题预览">
          <AwakenButton
            size="sm"
            variant={themePreference === "light" ? "default" : "ghost"}
            aria-pressed={themePreference === "light"}
            onClick={() => setThemePreference("light")}
          >
            浅色
          </AwakenButton>
          <AwakenButton
            size="sm"
            variant={themePreference === "dark" ? "default" : "ghost"}
            aria-pressed={themePreference === "dark"}
            onClick={() => setThemePreference("dark")}
          >
            暗色
          </AwakenButton>
        </div>
      </div>

      <section className="aw-account-demo__composition" aria-labelledby="aw-account-composition-title">
        <header className="aw-account-demo__section-heading">
          <h3 id="aw-account-composition-title">完整组合</h3>
          <p><code>AwakenAccount</code> 先展开个人菜单，再由“查询用量”打开左侧 API、右侧用量的独立窗口。</p>
        </header>
        <div className="aw-account-demo__composition-canvas">
          <AwakenAccount
            name="Nolan"
            avatarFallback="AW"
            balances={AwakenProfileBalanceSamples}
            themePreference={themePreference}
            open={accountOpen}
            onOpenChange={(open) => {
              setAccountOpen(open);
              setLastAction(open ? "已展开个人菜单。" : "已收起个人菜单。");
            }}
            onUsageOpenChange={(open) => {
              setLastAction(open ? "已打开用量窗口。" : "已关闭用量窗口。");
            }}
            onThemePreferenceChange={setThemePreference}
            onOpenSettings={() => {
              setLastAction("已触发进入设置回调（Gallery 模拟）。");
            }}
          />
        </div>
      </section>

      <section className="aw-account-demo__parts" aria-labelledby="aw-account-parts-title">
        <header className="aw-account-demo__section-heading">
          <h3 id="aw-account-parts-title">可独立组合的小组件</h3>
          <p>个人入口负责触发，快捷面板负责入口，双栏面板负责展示用量。</p>
        </header>
        <div className="aw-account-demo__part-list">
          <article className="aw-account-demo__part">
            <header>
              <h4>个人入口</h4>
              <code>AwakenProfileButton</code>
              <p>展示身份并通过受控展开状态关联账户面板。</p>
            </header>
            <div className="aw-account-demo__profile-canvas">
              <AwakenProfileButton
                name="Nolan"
                avatarFallback="AW"
                expanded={accountOpen}
                controls="awaken-account-demo-panel"
                onClick={() => setAccountOpen((open) => !open)}
              />
            </div>
          </article>

          <article className="aw-account-demo__part">
            <header>
              <h4>个人快捷面板</h4>
              <code>AwakenProfileMenuPanel</code>
              <p>左对齐展示“外观”“查询用量”“设置”，外观选项在右侧小窗中展开。</p>
            </header>
            <div className="aw-account-demo__panel-canvas">
              <AwakenProfileMenuPanel
                themePreference={themePreference}
                onThemePreferenceChange={setThemePreference}
                onOpenUsage={() => setLastAction("已触发查询用量回调（Gallery 模拟）。")}
                onOpenSettings={() => setLastAction("已触发进入设置回调（Gallery 模拟）。")}
              />
            </div>
          </article>

          <article className="aw-account-demo__part">
            <header>
              <h4>用量窗口内容</h4>
              <code>AwakenProfileBalancePanel</code>
              <p>覆盖可用、加载、空状态和错误状态，左右栏均由同一组件负责。</p>
            </header>
            <div className="aw-account-demo__panel-canvas">
              <div className="aw-account-demo__state-toggle" role="group" aria-label="额度面板状态">
                {AwakenProfileBalancePanelStates.map((state) => (
                  <AwakenButton
                    key={state.id}
                    size="sm"
                    variant={state.id === panelStateId ? "default" : "ghost"}
                    aria-pressed={state.id === panelStateId}
                    onClick={() => setPanelStateId(state.id)}
                  >
                    {state.label}
                  </AwakenButton>
                ))}
              </div>
              <AwakenProfileBalancePanel
                id="awaken-account-demo-panel"
                {...panelState.props}
                onOpenSettings={() => setLastAction("已触发进入设置回调（Gallery 模拟）。")}
              />
            </div>
          </article>
        </div>
      </section>
    </section>
  );
}
