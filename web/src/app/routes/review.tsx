import { useParams } from 'react-router';
import { ReviewDiff } from '@/app/review-diff';

export function ReviewPage() {
  const id = Number(useParams().id);

  return (
    <div className="-m-4 flex h-screen flex-col md:-m-6">
      <ReviewDiff id={id} />
    </div>
  );
}
