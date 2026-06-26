import "./AwakenDialog.css";
import { useState, type ComponentType } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "./AwakenDialog";
import { AwakenDialogStates } from "./AwakenDialog.states";

export function AwakenDialogDemo() {
  const Component = AwakenDialog as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDialogStates[0]?.props ?? {};
  const [previewOpen, setPreviewOpen] = useState(false);

  return (
    <section className="ads-component-demo" aria-label="弹窗 AwakenDialog 组件演示">
      <header className="ads-component-demo-header">
        <h2>弹窗 <code>AwakenDialog</code></h2>
        <p>用于创建项目、确认操作和需要焦点锁定的桌面端模态流程。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDialogStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas" style={{ minHeight: 360 }}>
              {state.id === "create-project" ? (
                <>
                  <AwakenButton variant="default" size="md" onClick={() => setPreviewOpen(true)}>
                    打开创建项目弹窗
                  </AwakenButton>
                  <Component
                    {...(defaultProps as Record<string, unknown>)}
                    {...(state.props as Record<string, unknown>)}
                    open={previewOpen}
                    onOpenChange={setPreviewOpen}
                  />
                </>
              ) : (
                <Component {...(defaultProps as Record<string, unknown>)} {...(state.props as Record<string, unknown>)} />
              )}
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
