import { repairWordTableWidths, maintainSpreadsheetFrozenRows } from '../shared/office-preview-layout.js';

export const OFFICE_PREVIEW_LAYOUT_VERSION = 2;

export function normalizeOfficePreviewLayout(html: string, extension: string): string {
  const repair = extension === '.docx' ? repairWordTableWidths
    : extension === '.xlsx' ? maintainSpreadsheetFrozenRows : undefined;
  if (!repair) return html;
  // Register before the converter's body script so Word widths are fixed before
  // its pagination measures the document. This edits only the disposable HTML.
  const script = `<script data-awaken-office-layout="${OFFICE_PREVIEW_LAYOUT_VERSION}">document.addEventListener('DOMContentLoaded',function(){(${repair.toString()})(document,MutationObserver,ResizeObserver);},{once:true});</script>`;
  return html.replace('</head>', `${script}\n</head>`);
}
