import "./AwakenAccount.demo.css";
import { useEffect, useRef, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenProfileButton } from "../AwakenProfileButton";
import {
  AwakenProfileBalancePanel,
  AwakenProfileBalancePanelStates,
  AwakenProfileBalanceSamples,
} from "../AwakenProfileBalancePanel";
import { AwakenAccount } from "./AwakenAccount";

export function AwakenAccountDemo() {
  const [themePreference, setThemePreference] = useState<"system" | "light" | "dark">("light");
  const [accountOpen, setAccountOpen] = useState(true);
  const [panelStateId, setPanelStateId] = useState("available");
  const [lastAction, setLastAction] = useState("点击 Nolan 可展开或收起账户额度面板。");
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
        <p>集中管理左下角个人入口、真实账户弹出层、模型额度面板与设置动作，不归属于左侧栏组件目录。</p>
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
          <p><code>AwakenAccount</code> 通过 <code>AwakenPopover</code> 组合个人入口、额度查询结果和设置动作；点击 Nolan 可验证打开与关闭。</p>
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
              setLastAction(open ? "已展开账户额度面板。" : "已收起账户额度面板。");
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
          <p>个人入口负责触发，额度面板负责展示数据与设置动作。</p>
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
              <h4>额度与设置面板</h4>
              <code>AwakenProfileBalancePanel</code>
              <p>覆盖可用、加载、空状态和错误状态，设置动作由 consumer 处理。</p>
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
                themePreference={themePreference}
                onThemePreferenceChange={setThemePreference}
                onOpenSettings={() => setLastAction("已触发进入设置回调（Gallery 模拟）。")}
              />
            </div>
          </article>
        </div>
      </section>
    </section>
  );
}
