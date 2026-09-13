// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { repairWordTableWidths, maintainSpreadsheetFrozenRows, type OfficeLayoutDocument } from '../../shared/office-preview-layout';
import { normalizeOfficePreviewLayout } from '../../main/office-preview-layout';

afterEach(() => { document.body.replaceChildren(); vi.restoreAllMocks(); });

describe('Office preview layout', () => {
  it('uses cell proportions within the declared Word table width instead of accumulating grid minima', () => {
    document.body.innerHTML = `<table data-path="/body/table[1]" style="width:432pt;table-layout:fixed"><colgroup><col style="width:216pt"><col style="width:340pt"></colgroup><tbody><tr><td style="width:130pt">名称</td><td style="width:340pt">长正文</td></tr></tbody></table>`;
    repairWordTableWidths(document as unknown as OfficeLayoutDocument);
    const table = document.querySelector('table')!;
    const widths = Array.from(table.querySelectorAll('col'), col => parseFloat(col.style.width));
    expect(widths.reduce((a,b) => a+b)).toBeCloseTo(100);
    expect(widths[1] / widths[0]).toBeCloseTo(340 / 130);
    expect(table.style.width).toBe('432pt');
    expect(table.style.maxWidth).toBe('100%');
    expect(Array.from(table.rows[0].cells).every(cell => !cell.style.width)).toBe(true);
    expect(table.textContent).toBe('名称长正文');
  });

  it('keeps automatic tables and uses the existing grid when all rows are merged', () => {
    document.body.innerHTML = `<table data-path="a" style="width:90pt"><colgroup><col style="width:90pt"></colgroup><tr><td>auto</td></tr></table><table data-path="b" style="width:60pt;table-layout:fixed"><colgroup><col style="width:30pt"><col style="width:60pt"></colgroup><tr><td colspan="2">merged</td></tr></table>`;
    repairWordTableWidths(document as unknown as OfficeLayoutDocument);
    expect(document.querySelector('col')!.style.width).toBe('90pt');
    expect(parseFloat(document.querySelectorAll('col')[1].style.width)).toBeCloseTo(100/3);
    expect(document.querySelectorAll('td')[1].colSpan).toBe(2);
  });

  it('recomputes frozen rows after activating a previously hidden sheet and after fonts resize it', () => {
    document.body.innerHTML = `<div class="sheet-content"><div class="table-wrapper"><table><thead><tr><th>A</th></tr></thead><tbody><tr data-frozen><th style="position:sticky;top:0px">1</th><td style="position:sticky;top:0px">标题</td></tr><tr data-frozen><th style="position:sticky;top:0px">2</th></tr></tbody></table></div></div>`;
    let resize: () => void = () => {}, mutation: () => void = () => {};
    class RO { constructor(cb: () => void) { resize=cb; } observe() {} }
    class MO { constructor(cb: () => void) { mutation=cb; } observe() {} }
    let rowHeight=32;
    vi.spyOn(HTMLElement.prototype,'getBoundingClientRect').mockImplementation(function(this: HTMLElement) {
      return {width:600,height:this.tagName === 'THEAD'?24:rowHeight} as DOMRect;
    });
    maintainSpreadsheetFrozenRows(document as unknown as OfficeLayoutDocument, MO, RO);
    const rows=document.querySelectorAll<HTMLTableRowElement>('tr[data-frozen]');
    expect(rows[0].cells[0].style.top).toBe('0px');
    document.querySelector('.sheet-content')!.classList.add('active'); mutation();
    expect(rows[0].cells[0].style.top).toBe('24px');
    expect(rows[1].cells[0].style.top).toBe('56px');
    rowHeight=48; resize();
    expect(rows[1].cells[0].style.top).toBe('72px');
  });

  it('registers the repair before converter pagination and leaves other formats alone', () => {
    const html='<html><head></head><body><script>paginate()</script></body></html>';
    const repaired=normalizeOfficePreviewLayout(html,'.docx');
    expect(repaired.indexOf('data-awaken-office-layout')).toBeLessThan(repaired.indexOf('</head>'));
    expect(repaired).toContain("DOMContentLoaded");
    expect(normalizeOfficePreviewLayout(html,'.pptx')).toBe(html);
  });
});
