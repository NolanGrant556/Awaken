import { LanguageDescription, type LanguageSupport } from "@codemirror/language";
import { languages } from "@codemirror/language-data";

export async function loadPreviewLanguage(
  fileName: string,
  languageName?: string,
): Promise<LanguageSupport | undefined> {
  const description =
    LanguageDescription.matchFilename(languages, fileName) ??
    (languageName
      ? LanguageDescription.matchLanguageName(languages, languageName, true)
      : null);
  if (!description) return undefined;
  try {
    return await description.load();
  } catch {
    return undefined;
  }
}
