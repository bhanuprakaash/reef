import { useParams } from 'react-router';

export function ChannelPage() {
  const { id } = useParams();
  return <h1 className="text-xl font-semibold">Channel {id}</h1>;
}
