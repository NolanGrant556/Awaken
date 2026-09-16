/** Strip Electron's transport envelope while retaining the actionable cause. */
export function desktopErrorMessage(message: string): string {
  return message.replace(/^Error invoking remote method '[^']+':\s*(?:Error:\s*)?/, "");
}
