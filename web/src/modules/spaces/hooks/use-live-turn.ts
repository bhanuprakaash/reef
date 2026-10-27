import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useReducer } from 'react';
import { inspectorKey } from '@/lib/query-keys';
import {
  emptyTurn,
  foldEvent,
  type AgentEvent,
  type LiveTurn,
  type StreamEvent,
} from '@/modules/spaces/utils/fold-events';

type TurnData = Omit<Extract<StreamEvent, { type: 'turn' }>, 'type'>;

function reduce(turn: LiveTurn, action: StreamEvent | 'clear'): LiveTurn {
  if (action !== 'clear') return foldEvent(turn, action);
  return turn.status === 'running' ? turn : { ...emptyTurn, note: turn.note };
}

export function useLiveTurn(spaceId: number): LiveTurn {
  const queryClient = useQueryClient();
  const [turn, dispatch] = useReducer(reduce, emptyTurn);

  useEffect(() => {
    const source = new EventSource(`/spaces/${spaceId}/events`);
    const refresh = () => queryClient.invalidateQueries({ queryKey: ['messages', spaceId] });

    source.onopen = () => {
      void refresh();
      void queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] });
      void queryClient.invalidateQueries({ queryKey: ['proposal', spaceId] });
      void queryClient.invalidateQueries({ queryKey: ['direct-messages', spaceId] });
      void queryClient.invalidateQueries({ queryKey: ['turns'] });
    };
    source.addEventListener('turn', (e) => {
      const data: TurnData = JSON.parse(e.data);
      dispatch({ type: 'turn', ...data });
      void queryClient.invalidateQueries({ queryKey: ['turns'] });
      void queryClient.invalidateQueries({ queryKey: ['direct-messages', spaceId] });
      if (data.status === 'done') {
        void refresh().then(() => dispatch('clear'));
        void queryClient.invalidateQueries({ queryKey: ['proposal', spaceId] });
        void queryClient.invalidateQueries({ queryKey: inspectorKey(spaceId) });
        void queryClient.invalidateQueries({ queryKey: ['features'] });
      } else if (data.status === 'failed') {
        void refresh();
      }
    });
    source.addEventListener('pause', () => {
      void queryClient.invalidateQueries({ queryKey: ['spaces', spaceId] });
    });
    source.addEventListener('name', () => {
      void queryClient.invalidateQueries({ queryKey: ['spaces'] });
    });
    source.addEventListener('direct', () => {
      void queryClient.invalidateQueries({ queryKey: ['direct-messages', spaceId] });
    });
    source.addEventListener('note', () => {
      void refresh();
    });
    source.addEventListener('agent', (e) => {
      const event: AgentEvent = JSON.parse(e.data);
      dispatch({ type: 'agent', event });
    });
    return () => source.close();
  }, [spaceId, queryClient]);

  return turn;
}
