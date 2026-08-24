import { memo, type ReactElement } from "react";

import {
  type AwakenIconConcept,
  type AwakenIconName,
  type AwakenIconSize,
  getAwakenIcon,
  iconForConcept,
} from "../../icons";
import "./AwakenIcon.css";

export type AwakenIconProps = {
  /** 图形名。与 `concept` 二者取一。 */
  name?: AwakenIconName;
  /** 概念名。产品代码优先用它 —— 概念稳定，图形可替换。 */
  concept?: AwakenIconConcept;
  /** 只接受 tokens.css 里既有的五档。 */
  size?: AwakenIconSize;
  className?: string;
  /**
   * 有 label 时渲染为 `role="img"` 并带无障碍名；
   * 省略时按装饰性图形处理（`aria-hidden`），由外层控件提供可访问名。
   */
  label?: string;
};

/**
 * Awaken 图标的唯一渲染出口。
 *
 * 描边宽度、viewBox、端点与接合方式全部由图标层决定，不接受调用点覆盖 ——
 * 描边参与光学归一化，能被覆盖就等于放弃「同屏视觉重量一致」这条不变量。
 * 颜色跟随 `currentColor`。
 */
function AwakenIconImpl({
  name,
  concept,
  size = 16,
  className,
  label,
}: AwakenIconProps): ReactElement {
  const resolved: AwakenIconName | undefined = concept ? iconForConcept(concept) : name;
  if (!resolved) {
    throw new Error("AwakenIcon 需要 name 或 concept 之一");
  }
  const icon = getAwakenIcon(resolved);

  const classes = ["aw-icon", `aw-icon--${resolved}`];
  if (className) classes.push(className);

  const a11y = label ? { role: "img" as const, "aria-label": label } : { "aria-hidden": true as const };

  const common = {
    width: size,
    height: size,
    viewBox: icon.viewBox,
    className: classes.join(" "),
    focusable: false as const,
    ...a11y,
  };

  const isFilled = icon.mode === "brand-fill" || (icon.mode === "line" && icon.filled);
  if (isFilled) {
    return (
      <svg {...common} fill="currentColor">
        {icon.paths.map((d, i) => (
          <path key={i} d={d} />
        ))}
      </svg>
    );
  }

  return (
    <svg
      {...common}
      fill="none"
      stroke="currentColor"
      strokeWidth={icon.strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      {icon.paths.map((d, i) => (
        <path
          key={i}
          d={d}
          fill={icon.mode === "line" && icon.filledPathIndexes?.includes(i) ? "currentColor" : undefined}
        />
      ))}
    </svg>
  );
}

export const AwakenIcon = memo(AwakenIconImpl);
AwakenIcon.displayName = "AwakenIcon";
