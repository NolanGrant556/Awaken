import { useState } from "react";
import { AwakenButton } from "../AwakenButton";
import type { AwakenSkillProject } from "../AwakenSkills/AwakenSkills.types";
import { AwakenProjectSkillsDialog } from "./AwakenProjectSkillsDialog";
import { AwakenProjectSkillsDialogState } from "./AwakenProjectSkillsDialog.states";

export function AwakenProjectSkillsDialogDemo() {
  const [open, setOpen] = useState(false);
  const [project, setProject] = useState<AwakenSkillProject>(AwakenProjectSkillsDialogState);
  const [notice, setNotice] = useState("打开窗口后可操作每张 Skill 卡片。");

  return (
    <section className="ads-component-demo" aria-label="项目 Skills 窗口 AwakenProjectSkillsDialog 组件演示">
      <header className="ads-component-demo-header">
        <h2>项目 Skills 窗口 <code>AwakenProjectSkillsDialog</code></h2>
        <p>承接项目文件夹的下一层交互，统一展示项目名称、路径和 Skill 卡片。</p>
      </header>
      <div className="ads-component-demo-state">
        <h3>实时交互</h3>
        <div className="ads-component-demo-canvas">
          <AwakenButton onClick={() => setOpen(true)}>打开项目 Skills 窗口</AwakenButton>
          <span role="status">{notice}</span>
        </div>
      </div>
      <AwakenProjectSkillsDialog
        open={open}
        project={project}
        onOpenChange={setOpen}
        onSkillPreview={(skill) => setNotice(`已请求预览：${skill.name}`)}
        onSkillToggle={(skill, enabled) => {
          setProject((current) => ({
            ...current,
            skills: current.skills.map((item) => item.id === skill.id ? { ...item, enabled } : item),
          }));
        }}
      />
    </section>
  );
}
