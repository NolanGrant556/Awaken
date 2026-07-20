import "./AwakenSkillCard.demo.css";
import { useState } from "react";
import type { AwakenSkillItem } from "../AwakenSkills/AwakenSkills.types";
import { AwakenSkillCard } from "./AwakenSkillCard";
import { AwakenSkillCardStates } from "./AwakenSkillCard.states";

export function AwakenSkillCardDemo() {
  const [skills, setSkills] = useState<AwakenSkillItem[]>(() =>
    AwakenSkillCardStates.map((state) => state.skill)
  );
  const [notice, setNotice] = useState("点击卡片主体请求预览；右侧开关只改变启停状态。");

  return (
    <section className="ads-component-demo" aria-label="Skill 卡片 AwakenSkillCard 组件演示">
      <header className="ads-component-demo-header">
        <h2>Skill 卡片 <code>AwakenSkillCard</code></h2>
        <p>统一承载 Skill 名称、说明、来源、诊断、预览入口与启停操作。</p>
      </header>
      <p className="aw-skill-card-demo__notice" role="status">{notice}</p>
      <div className="aw-skill-card-demo__stack">
        {skills.map((item) => (
          <AwakenSkillCard
            key={item.id}
            skill={item}
            onPreview={() => setNotice(`已请求预览：${item.name}`)}
            onEnabledChange={(enabled) => {
              setSkills((current) => current.map((skill) =>
                skill.id === item.id ? { ...skill, enabled } : skill));
            }}
          />
        ))}
      </div>
    </section>
  );
}
