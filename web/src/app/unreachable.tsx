import { Creature } from '@/components/creature';

export function Unreachable() {
  return (
    <div className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-2.5 bg-main px-4 text-center font-sans text-fg">
      <Creature role="lead" state="failed" size={52} />
      <h1 className="text-[15px] font-normal">Can't reach reef serve</h1>
      <p className="text-[13px] text-muted">
        The server stopped. Your channels are safe. Start it again with `reef serve`; this window
        reconnects by itself.
      </p>
    </div>
  );
}
