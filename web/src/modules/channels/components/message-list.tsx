import type { Message } from '@/types/api';
import { mutedClass } from '@/utils/classes';

export function MessageList({ messages }: { messages: Message[] }) {
  if (messages.length === 0) return <p className={mutedClass}>No messages yet.</p>;

  return (
    <ul className="space-y-3">
      {messages.map((m) => (
        <li key={m.id}>
          <div className="text-xs font-medium uppercase text-neutral-500">{m.author}</div>
          <p className="whitespace-pre-wrap text-sm">{m.body}</p>
        </li>
      ))}
    </ul>
  );
}
