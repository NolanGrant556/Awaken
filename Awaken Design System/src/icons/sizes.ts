/**
 * 图标尺寸档。
 *
 * 这五档和 tokens.css 里既有的 `--aw-icon-size-12/14/16/20/24` 是同一套，
 * 那些 token 早就存在但从未被强制 —— 迁移前的实测是 11/12/13/14/15/16/17/18/19/20/22/42
 * 共 12 档在混用。`AwakenIcon` 只接受这五档，让既有契约真的生效。
 *
 * 16 是主用尺寸，12 是可读下限。20 及以上才允许使用品牌标记。
 */
export const AWAKEN_ICON_SIZES = [12, 14, 16, 20, 24] as const;

export type AwakenIconSize = (typeof AWAKEN_ICON_SIZES)[number];

export function isAwakenIconSize(value: number): value is AwakenIconSize {
  return (AWAKEN_ICON_SIZES as readonly number[]).includes(value);
}
