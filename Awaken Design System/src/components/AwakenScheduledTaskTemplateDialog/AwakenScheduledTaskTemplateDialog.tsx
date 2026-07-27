import "./AwakenScheduledTaskTemplateDialog.css";
import { useEffect, useState } from "react";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";

// 模板卡数据由 Renderer 的内置模板目录投影而来（契约 §5）；
// Design System 只接收卡片数据和回调，不读取目录、不构造任何 Core payload。
export type AwakenScheduledTaskTemplateCard = {
  templateId: string;
  name: string;
  description: string;
  frequencyText: string;
};

export type AwakenScheduledTaskTemplateCategory = {
  id: string;
  label: string;
  templates: AwakenScheduledTaskTemplateCard[];
};

export type AwakenScheduledTaskTemplateDialogProps = {
  open: boolean;
  title: string;
  description: string;
  categories: AwakenScheduledTaskTemplateCategory[];
  useTemplateLabel?: string;
  onOpenChange: (open: boolean) => void;
  onSelect: (templateId: string) => void;
};

export function AwakenScheduledTaskTemplateDialog({
  open,
  title,
  description,
  categories,
  useTemplateLabel = "使用模板",
  onOpenChange,
  onSelect,
}: AwakenScheduledTaskTemplateDialogProps) {
  const [activeCategoryId, setActiveCategoryId] = useState<string | null>(null);

  // 每次重新打开都回到第一个分类，不记住上次浏览位置。
  useEffect(() => {
    if (open) {
      setActiveCategoryId(null);
    }
  }, [open]);

  const activeCategory =
    categories.find((category) => category.id === activeCategoryId) ?? categories[0] ?? null;

  return (
    <AwakenDialog
      open={open}
      className="aw-scheduled-task-template-dialog"
      title={title}
      onOpenChange={onOpenChange}
    >
      <div className="aw-scheduled-task-template-dialog__content">
        <p className="aw-scheduled-task-template-dialog__description">{description}</p>
        {categories.length > 1 ? (
          <div className="aw-scheduled-task-template-dialog__tabs" role="tablist" aria-label={title}>
            {categories.map((category) => (
              <button
                key={category.id}
                type="button"
                role="tab"
                aria-selected={activeCategory?.id === category.id}
                data-active={activeCategory?.id === category.id ? "true" : "false"}
                onClick={() => setActiveCategoryId(category.id)}
              >
                {category.label}
              </button>
            ))}
          </div>
        ) : null}
        {activeCategory ? (
          <div className="aw-scheduled-task-template-dialog__grid">
            {activeCategory.templates.map((template) => (
              <article className="aw-scheduled-task-template-dialog__card" key={template.templateId}>
                <strong className="aw-scheduled-task-template-dialog__card-name">{template.name}</strong>
                <p className="aw-scheduled-task-template-dialog__card-description">{template.description}</p>
                <span className="aw-scheduled-task-template-dialog__card-frequency">{template.frequencyText}</span>
                <span className="aw-scheduled-task-template-dialog__card-action">
                  <AwakenButton variant="default" size="sm" onClick={() => onSelect(template.templateId)}>
                    {useTemplateLabel}
                  </AwakenButton>
                </span>
              </article>
            ))}
          </div>
        ) : null}
      </div>
    </AwakenDialog>
  );
}
