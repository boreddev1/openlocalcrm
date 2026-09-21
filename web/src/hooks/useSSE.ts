import { useEffect, useState } from 'react';

export interface SSEMessage {
  type: string;
  data: any;
}

export function useSSE(url: string = '/events/stream') {
  const [connected, setConnected] = useState<boolean>(false);
  const [lastEvent, setLastEvent] = useState<SSEMessage | null>(null);

  useEffect(() => {
    let eventSource: EventSource | null = null;
    let reconnectTimeout: ReturnType<typeof setTimeout>;

    function connect() {
      // Authenticate via HttpOnly cookie (Finding #26: No token in query params)
      eventSource = new EventSource(url, { withCredentials: true });

      eventSource.addEventListener('connected', () => {
        setConnected(true);
      });

      eventSource.onopen = () => {
        setConnected(true);
      };

      eventSource.onerror = () => {
        setConnected(false);
        eventSource?.close();
        // Reconnect after 3s
        reconnectTimeout = setTimeout(connect, 3000);
      };

      // Custom event types
      const events = ['email_received', 'todo_assigned', 'deal_updated', 'ai_stream'];
      events.forEach((evtType) => {
        eventSource?.addEventListener(evtType, (event: MessageEvent) => {
          try {
            const data = JSON.parse(event.data);
            setLastEvent({ type: evtType, data });
          } catch {
            setLastEvent({ type: evtType, data: event.data });
          }
        });
      });
    }

    const handleLogout = () => {
      setConnected(false);
      clearTimeout(reconnectTimeout);
      eventSource?.close();
      eventSource = null;
    };
    window.addEventListener('auth:logout', handleLogout);

    connect();

    return () => {
      window.removeEventListener('auth:logout', handleLogout);
      clearTimeout(reconnectTimeout);
      eventSource?.close();
    };
  }, [url]);

  return { connected, lastEvent };
}
