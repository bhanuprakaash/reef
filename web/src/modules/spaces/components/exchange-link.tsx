import { Link } from 'react-router';
import { useExchange } from '@/modules/spaces/api/get-exchange';

type Props = { exchangeId: number; spaceId: number };

export function ExchangeLink({ exchangeId, spaceId }: Props) {
  const { data } = useExchange(exchangeId);
  if (!data || data.request.spaceId === spaceId) return null;
  return (
    <Link to={`/spaces/${data.request.spaceId}`} className="px-1 text-[13px] text-soft underline">
      Open consultation
    </Link>
  );
}
