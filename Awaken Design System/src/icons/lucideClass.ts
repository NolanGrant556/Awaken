export function awakenLucideClassName(exportName: string): string {
  const kebab = exportName
    .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
    .replace(/([A-Z])([A-Z][a-z])/g, "$1-$2")
    .toLowerCase();
  return `lucide lucide-${kebab}`;
}
