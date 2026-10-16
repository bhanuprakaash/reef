import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useReducer } from 'react';
import { inspectorKey } from '@/lib/query-keys';
import {
  emptyTurn,
  foldEvent,
  type AgentEvent,
  type LiveTurn,
  type StreamEvent,
} from '@/modules/channels/utils/fold-events';

type TurnData = Omit<Extract<StreamEvent, { type: 'turn' }>, 'type'>;

function reduce(turn: LiveTurn, action: StreamEvent | 'clear'): LiveTurn {
  if (action !== 'clear') return foldEvent(turn, action);
  return turn.status === 'running' ? turn : { ...emptyTurn, note: turn.note };
}

export function useLiveTurn(channelId: number): LiveTurn {
  const queryClient = useQueryClient();
  const [turn, dispatch] = useReducer(reduce, emptyTurn);

  useEffect(() => {
    const source = new EventSource(`/channels/${channelId}/events`);
    const refresh = () => queryClient.invalidateQueries({ queryKey: ['messages', channelId] });

    source.onopen = () => {
      void refresh();
      void queryClient.invalidateQueries({ queryKey: ['channels', channelId] });
      void queryClient.invalidateQueries({ queryKey: ['proposal', channelId] });
      void queryClient.invalidateQueries({ queryKey: ['approvals', channelId] });
      void queryClient.invalidateQueries({ queryKey: ['turns'] });
    };
    source.addEventListener('turn', (e) => {
      const data: TurnData = JSON.parse(e.data);
      dispatch({ type: 'turn', ...data });
      void queryClient.invalidateQueries({ queryKey: ['turns'] });
      if (data.status === 'done') {
        void refresh().then(() => dispatch('clear'));
        void queryClient.invalidateQueries({ queryKey: ['proposal', channelId] });
        void queryClient.invalidateQueries({ queryKey: inspectorKey(channelId) });
      } else if (data.status === 'failed') {
        void refresh();
      }
    });
    source.addEventListener('pause', () => {
      void queryClient.invalidateQueries({ queryKey: ['channels', channelId] });
    });
    source.addEventListener('approval', () => {
      void queryClient.invalidateQueries({ queryKey: ['approvals', channelId] });
    });
    source.addEventListener('note', () => {
      void refresh();
    });
    source.addEventListener('agent', (e) => {
      const event: AgentEvent = JSON.parse(e.data);
      dispatch({ type: 'agent', event });
    });
    return () => source.close();
  }, [channelId, queryClient]);

  return turn;
}
