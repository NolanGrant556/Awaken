// Structural DOM types keep lib.dom out of the Main compilation.
type OfficeElement = {
  style: { tableLayout: string; width: string; maxWidth: string; position: string; top: string; removeProperty(name: string): string };
  rows: ArrayLike<OfficeElement>;
  cells: ArrayLike<OfficeElement>;
  colSpan: number;
  rowSpan: number;
  tHead: OfficeElement | null;
  getBoundingClientRect(): { width: number; height: number };
  querySelectorAll(selector: string): Iterable<OfficeElement>;
};
export type OfficeLayoutDocument = {
  querySelectorAll(selector: string): Iterable<OfficeElement>;
  fonts?: { ready: Promise<unknown> };
};
type LayoutObserver = new (callback: () => void) => {
  observe(element: OfficeElement, options?: { attributes: boolean; attributeFilter: string[] }): void;
};

// Runs only in the isolated Office preview document. No bridge, network or
// filesystem access: the bundled converter remains the content authority.
export function repairWordTableWidths(doc: OfficeLayoutDocument): void {
  for (const table of doc.querySelectorAll('table[data-path]')) {
    if (table.style.tableLayout !== 'fixed') continue;
    const columns = Array.from(table.querySelectorAll(':scope > colgroup > col'));
    if (!columns.length) continue;
    if (!/^\d+(\.\d+)?pt$/.test(table.style.width) ||
        !columns.every(col => /^\d+(\.\d+)?pt$/.test(col.style.width))) continue;
    const gridTotal = columns.reduce((sum, col) => sum + parseFloat(col.style.width), 0);
    if (gridTotal <= parseFloat(table.style.width) + 0.5) continue;
    // The converter takes max(grid width, cell width) independently for each
    // column. Mixed grid/tcW widths can then exceed tblW and the paper itself.
    const row = Array.from(table.rows).find(row => row.cells.length === columns.length &&
      Array.from(row.cells).every(cell => cell.colSpan === 1 && cell.rowSpan === 1 && /^\d+(\.\d+)?pt$/.test(cell.style.width)));
    const widths = row ? Array.from(row.cells, cell => parseFloat(cell.style.width))
      : columns.map(col => parseFloat(col.style.width));
    const total = widths.reduce((sum, width) => sum + width, 0);
    if (!widths.every(width => Number.isFinite(width) && width > 0) || total <= 0) continue;
    columns.forEach((col, index) => { col.style.width = `${widths[index] / total * 100}%`; });
    table.style.maxWidth = '100%';
    // colgroup owns the fixed grid. Per-cell absolute minima must not expand it.
    for (const row of Array.from(table.rows)) for (const cell of Array.from(row.cells)) cell.style.removeProperty('width');
  }
}

export function maintainSpreadsheetFrozenRows(doc: OfficeLayoutDocument, MutationObserver: LayoutObserver, ResizeObserver: LayoutObserver): void {
  const sync = () => {
    for (const table of doc.querySelectorAll('.sheet-content.active .table-wrapper table')) {
      if (!table.getBoundingClientRect().width) continue;
      let top = table.tHead?.getBoundingClientRect().height ?? 0;
      for (const row of table.querySelectorAll('tr[data-frozen]')) {
        for (const cell of Array.from(row.cells)) if (cell.style.position === 'sticky') cell.style.top = `${top}px`;
        top += row.getBoundingClientRect().height;
      }
    }
  };
  // Inactive sheets have zero geometry. Measure after activation, and again
  // after fonts or viewport changes, rather than caching zero at document load.
  const observer = new MutationObserver(sync);
  for (const sheet of doc.querySelectorAll('.sheet-content')) observer.observe(sheet, { attributes: true, attributeFilter: ['class'] });
  const resize = new ResizeObserver(sync);
  for (const table of doc.querySelectorAll('.table-wrapper table')) resize.observe(table);
  void doc.fonts?.ready.then(sync);
  sync();
}
