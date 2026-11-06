import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../api/client';

// Placeholder shown when no data arrived yet. It must never read like a model
// name: issue #72 was exactly about the UI naming a model the instance never
// served. "— (nicht verbunden)" states the truth instead.
export const MODEL_NOT_CONNECTED = '— (nicht verbunden)';

// Single source for every model label in the SPA: stats.active_model of the
// AI observability surface (/api/v1/ai/observability). The backend fills it
// from the serving gateway (internal/ai/gateway.go ServingModel), so demo
// mode reports the DEMO marker and production reports the configured tag.
// /api/v1/ai/* is rate-limited (30/min per client, internal/server/router.go),
// so callers that only show the label inside conditionally mounted UI (drawer,
// modal, widget) pass enabled=false until that UI is actually open.
export function useAIModel(enabled = true): string {
  const { data } = useQuery<any>({
    queryKey: ['ai-observability'],
    queryFn: () => apiFetch('/api/v1/ai/observability'),
    staleTime: 60000,
    enabled,
  });
  return data?.stats?.active_model || MODEL_NOT_CONNECTED;
}
