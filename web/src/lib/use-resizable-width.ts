import { useState } from 'react';

type Options = { key: string; initial: number; min: number; maxShare: number };

function stored({ key, initial, min }: Options) {
  try {
    const width = Number(localStorage.getItem(key));
    return width >= min ? width : initial;
  } catch {
    return initial;
  }
}

// useResizableWidth keeps a pane's width across reloads. A pane on the right edge
// grows as its left-edge handle moves left, so the drag delta is startX - clientX.
export function useResizableWidth(options: Options) {
  const { key, min, maxShare } = options;
  const [width, setWidth] = useState(() => stored(options));

  const startResize = (e: React.PointerEvent<HTMLElement>) => {
    const handle = e.currentTarget;
    const startX = e.clientX;
    const startWidth = width;
    const max = Math.max(min, window.innerWidth * maxShare);
    let latest = startWidth;
    handle.setPointerCapture(e.pointerId);
    const move = (ev: PointerEvent) => {
      if (ev.buttons === 0) return end();
      latest = Math.round(Math.min(max, Math.max(min, startWidth + startX - ev.clientX)));
      setWidth(latest);
    };
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('lostpointercapture', end);
      try {
        localStorage.setItem(key, String(latest));
      } catch {
        // storage can be blocked; the width then lasts until reload
      }
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('lostpointercapture', end);
  };

  return { width, startResize };
}
