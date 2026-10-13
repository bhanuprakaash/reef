import { useParams } from 'react-router';

export function FeaturePage() {
  const { id } = useParams();
  return <h1 className="text-xl font-semibold">Feature {id}</h1>;
}
