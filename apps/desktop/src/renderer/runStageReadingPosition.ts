// A reading position belongs to a turn and a visible block, not a pixel offset
// in the whole transcript. Inspector resizing reflows all earlier paragraphs.
export type RunStageReadingPosition = {
  turnId: string;
  path: number[];
  offset: number;
  scrollTop: number;
  width: number;
};

export function captureRunStageReadingPosition(element: HTMLElement): RunStageReadingPosition | undefined {
  const viewport = element.getBoundingClientRect();
  if (!element.clientWidth || !element.clientHeight) return;
  const blocks = Array.from(element.querySelectorAll<HTMLElement>(
    '.execution-turn-body p, .execution-turn-body h1, .execution-turn-body h2, .execution-turn-body h3, .execution-turn-body h4, .execution-turn-body li, .execution-turn-body tr, .execution-turn-body pre, .execution-turn-body button',
  ));
  // Skip the sticky prompt band; its position does not identify the text being read.
  const block = blocks.find(node => {
    const rect = node.getBoundingClientRect();
    return rect.height > 0 && rect.bottom > viewport.top + 80 && rect.top < viewport.bottom;
  }) ?? Array.from(element.querySelectorAll<HTMLElement>('[data-execution-turn-id]')).find(node => {
    const rect = node.getBoundingClientRect();
    return rect.height > 0 && rect.bottom > viewport.top && rect.top < viewport.bottom;
  });
  const turn = block?.closest<HTMLElement>('[data-execution-turn-id]');
  if (!block || !turn?.dataset.executionTurnId) return;
  const path: number[] = [];
  let node: Element = block;
  while (node !== turn && node.parentElement) {
    path.unshift(Array.from(node.parentElement.children).indexOf(node));
    node = node.parentElement;
  }
  return { turnId: turn.dataset.executionTurnId, path,
    offset: block.getBoundingClientRect().top - viewport.top,
    scrollTop: element.scrollTop, width: element.clientWidth };
}

export function restoredRunStageScrollTop(element: HTMLElement, position: RunStageReadingPosition): number | undefined {
  let node: Element | undefined = Array.from(element.querySelectorAll<HTMLElement>('[data-execution-turn-id]'))
    .find(turn => turn.dataset.executionTurnId === position.turnId);
  if (!node) return;
  for (const index of position.path) node = node?.children[index];
  const top = node
    ? element.scrollTop + node.getBoundingClientRect().top - element.getBoundingClientRect().top - position.offset
    : position.scrollTop;
  return Math.max(0, Math.min(element.scrollHeight - element.clientHeight, top));
}
