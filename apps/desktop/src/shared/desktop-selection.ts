export const DESKTOP_SELECT_ALL_REQUESTED_CHANNEL =
  "awaken:select-all-requested";

export type DesktopKeyboardInput = {
  type: string;
  key: string;
  code?: string;
  control: boolean;
  meta: boolean;
  alt: boolean;
  shift: boolean;
};

export function isDesktopSelectAllShortcut(
  input: DesktopKeyboardInput,
): boolean {
  const isPrimaryModifier = input.meta !== input.control;
  const isAKey =
    input.key.toLowerCase() === "a" || input.code?.toLowerCase() === "keya";

  return (
    input.type === "keyDown" &&
    isPrimaryModifier &&
    !input.alt &&
    !input.shift &&
    isAKey
  );
}
