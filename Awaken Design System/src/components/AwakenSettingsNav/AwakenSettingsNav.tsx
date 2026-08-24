import "./AwakenSettingsNav.css";
import { Fragment } from "react";
import { AwakenIcon } from "../AwakenIcon";
import type { AwakenIconConcept } from "../../icons";

export type AwakenSettingsNavItem = {
  id: string;
  label: string;
  concept?: AwakenIconConcept;
  disabled?: boolean;
  /** 分组标题。与前一项的 groupLabel 不同时在该条目上方渲染；它不是 tab。 */
  groupLabel?: string;
};

export type AwakenSettingsNavProps = {
  items: AwakenSettingsNavItem[];
  activeId?: string;
  ariaLabel?: string;
  onSelect?: (id: string) => void;
};

export function AwakenSettingsNav({
  items,
  activeId,
  ariaLabel = "设置分区",
  onSelect,
}: AwakenSettingsNavProps) {
  return (
    <nav className="aw-settings-nav" role="tablist" aria-label={ariaLabel}>
      {items.map((item, index) => {
        // 分组标题用 presentation 渲染：tablist 的直接语义子级只能是 tab，
        // 标题只是纯文本分隔，不占用 tab 语义也不参与键盘 tab 序列。
        const showGroupLabel =
          item.groupLabel !== undefined && item.groupLabel !== items[index - 1]?.groupLabel;
        return (
          <Fragment key={item.id}>
            {showGroupLabel ? (
              <div className="aw-settings-nav__group-label" role="presentation">
                {item.groupLabel}
              </div>
            ) : null}
            <button
              type="button"
              className={[
                "aw-settings-nav__item",
                item.id === activeId ? "aw-settings-nav__item--active" : "",
              ].filter(Boolean).join(" ")}
              role="tab"
              aria-selected={item.id === activeId}
              disabled={item.disabled}
              onClick={() => onSelect?.(item.id)}
            >
              {item.concept ? <AwakenIcon concept={item.concept} size={16} /> : null}
              <span>{item.label}</span>
            </button>
          </Fragment>
        );
      })}
    </nav>
  );
}
