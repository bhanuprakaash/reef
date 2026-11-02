import { Link } from 'react-router';
import { Button } from '@/components/button';

export function SectionHeading({
  to,
  label,
  addLabel,
  onAdd,
}: {
  to: string;
  label: string;
  addLabel: string;
  onAdd: () => void;
}) {
  return (
    <div className="flex items-center justify-between pt-2.5 pr-1">
      <h4 className="text-xs">
        <Link
          to={to}
          className="group/label flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-muted outline-none hover:bg-hover/50 hover:text-fg focus-visible:ring-3 focus-visible:ring-ring max-md:py-3"
        >
          {label}
          <svg
            width="10"
            height="10"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.4"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="transition-transform group-hover/label:translate-x-0.5"
            aria-hidden="true"
          >
            <path d="M9 6l6 6-6 6" />
          </svg>
        </Link>
      </h4>
      <Button
        type="button"
        variant="ghost"
        size="iconSm"
        aria-label={addLabel}
        title={addLabel}
        onClick={onAdd}
        className="hover:text-fg max-md:size-11"
      >
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          aria-hidden="true"
        >
          <path d="M12 5v14M5 12h14" />
        </svg>
      </Button>
    </div>
  );
}
