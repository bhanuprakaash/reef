import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useReducer } from 'react';
import {
  emptyTurn,
  foldEvent,
  type AgentEvent,
  type LiveTurn,
  type StreamEvent,
} from '@/modules/channels/utils/fold-events';

type TurnData = { status: 'running' | 'done' | 'failed'; error?: string };

function reduce(turn: LiveTurn, action: StreamEvent | 'clear'): LiveTurn {
  if (action !== 'clear') return foldEvent(turn, action);
  return turn.status === 'running' ? turn : emptyTurn;
}

export function useLiveTurn(channelId: number): LiveTurn {
  const queryClient = useQueryClient();
  const [turn, dispatch] = useReducer(reduce, emptyTurn);

  useEffect(() => {
    const source = new EventSource(`/channels/${channelId}/events`);
    const refresh = () => queryClient.invalidateQueries({ queryKey: ['messages', channelId] });

    source.addEventListener('turn', (e) => {
      const data: TurnData = JSON.parse(e.data);
      dispatch({ type: 'turn', ...data });
      if (data.status === 'done') void refresh().then(() => dispatch('clear'));
      else if (data.status === 'failed') void refresh();
    });
    source.addEventListener('agent', (e) => {
      const event: AgentEvent = JSON.parse(e.data);
      dispatch({ type: 'agent', event });
      if (event.kind === 'session') void refresh();
    });
    return () => source.close();
  }, [channelId, queryClient]);

  return turn;
}
