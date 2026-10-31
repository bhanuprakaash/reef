import { useEffect, useLayoutEffect, useRef, useState } from 'react';

const _nearBottom = 120;

// Keeps the box scrolled to the end as its content grows, but only while the user is near the end,
// or when `jump` changes (they just sent a message). `hold` marks the box before content is added
// above, and the next change of `top` keeps what the user was reading where it was.
export function useStickToBottom(jump?: unknown, top?: unknown) {
  const box = useRef<HTMLDivElement>(null);
  const stuck = useRef(true);
  const [atBottom, setAtBottom] = useState(true);
  const held = useRef<number | undefined>(undefined);

  const hold = () => {
    held.current = box.current?.scrollHeight;
  };

  useLayoutEffect(() => {
    const el = box.current;
    if (!el || held.current === undefined) return;
    el.scrollTop += el.scrollHeight - held.current;
    held.current = undefined;
  }, [top]);

  const onScroll = () => {
    const el = box.current;
    if (!el) return;
    stuck.current = el.scrollHeight - el.scrollTop - el.clientHeight < _nearBottom;
    setAtBottom(stuck.current);
  };

  const toBottom = () => {
    const el = box.current;
    if (!el) return;
    stuck.current = true;
    setAtBottom(true);
    el.scrollTop = el.scrollHeight;
  };

  useEffect(() => {
    const el = box.current;
    if (!el?.firstElementChild) return;
    const observer = new ResizeObserver(() => {
      if (stuck.current) el.scrollTop = el.scrollHeight;
    });
    observer.observe(el.firstElementChild);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    stuck.current = true;
    setAtBottom(true);
    el.scrollTop = el.scrollHeight;
  }, [jump]);

  return { box, onScroll, atBottom, toBottom, hold };
}
