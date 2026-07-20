import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenSkillPreviewDialog } from "./AwakenSkillPreviewDialog";
import { AwakenSkillPreviewDialogState } from "./AwakenSkillPreviewDialog.states";

type PreviewMode = "content" | "loading" | "error";

export function AwakenSkillPreviewDialogDemo() {
  const [mode, setMode] = useState<PreviewMode | null>(null);

  return (
    <section className="ads-component-demo" aria-label="Skill 预览窗口 AwakenSkillPreviewDialog 组件演示">
      <header className="ads-component-demo-header">
        <h2>Skill 预览窗口 <code>AwakenSkillPreviewDialog</code></h2>
        <p>只读展示完整 YAML frontmatter 与安全 Markdown，并提供加载和错误状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        <article className="ads-component-demo-state">
          <h3>正文</h3>
          <AwakenButton onClick={() => setMode("content")}>打开正文预览</AwakenButton>
        </article>
        <article className="ads-component-demo-state">
          <h3>加载中</h3>
          <AwakenButton onClick={() => setMode("loading")}>打开加载状态</AwakenButton>
        </article>
        <article className="ads-component-demo-state">
          <h3>读取失败</h3>
          <AwakenButton onClick={() => setMode("error")}>打开错误状态</AwakenButton>
        </article>
      </div>
      <AwakenSkillPreviewDialog
        open={mode !== null}
        contentKey={mode ?? undefined}
        frontmatterRaw={mode === "content" ? AwakenSkillPreviewDialogState.frontmatterRaw : undefined}
        markdown={mode === "content" ? AwakenSkillPreviewDialogState.markdown : undefined}
        loading={mode === "loading"}
        errorMessage={mode === "error" ? "Skill 文件已移动或当前没有读取权限。" : null}
        onOpenChange={(open) => {
          if (!open) {
            setMode(null);
          }
        }}
      />
    </section>
  );
}
