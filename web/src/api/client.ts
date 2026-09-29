export function getFieldText(val: any, fallback = ''): string {
  if (!val) return fallback;
  if (typeof val === 'string') return val;
  if (typeof val === 'object' && 'String' in val) {
    return val.Valid ? val.String : fallback;
  }
  return String(val);
}

export class ApiError extends Error {
  status: number;
  data: any;

  constructor(status: number, message: string, data?: any) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.data = data;
  }
}

export function getCookie(name: string): string | null {
  if (typeof document === 'undefined' || !document.cookie) return null;
  const cookies = document.cookie.split(';');
  for (let i = 0; i < cookies.length; i++) {
    const cookie = cookies[i].trim();
    const eqIdx = cookie.indexOf('=');
    if (eqIdx !== -1) {
      const key = cookie.substring(0, eqIdx).trim();
      if (key === name) {
        return decodeURIComponent(cookie.substring(eqIdx + 1));
      }
    }
  }
  return null;
}

let refreshPromise: Promise<void> | null = null;
let lastRefreshSuccess = 0;

export async function withRefreshLock<T>(fn: () => Promise<T>): Promise<T> {
  if (!refreshPromise) {
    refreshPromise = (fn() as Promise<any>).finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise as unknown as T;
}

// Cross-tab coordination: two tabs have separate heaps, so a module promise is
// not enough. The Web Lock makes one tab the refresher; the others wait and
// then reuse the shared cookie jar.
export async function withCrossTabLock<T>(fn: () => Promise<T>): Promise<T> {
  const locks =
    typeof navigator !== 'undefined'
      ? (navigator as Navigator & { locks?: LockManager }).locks
      : undefined;
  if (!locks) return withRefreshLock(fn);
  return locks.request('openlocalcrm-refresh', { mode: 'exclusive' }, () => withRefreshLock(fn));
}

export const UNAUTHORIZED_IS_NOT_SESSION_EXPIRY = new Set([
  '/api/v1/auth/login',
  '/api/v1/auth/totp/disable',
]);

const rawFetch = typeof window !== 'undefined' ? window.fetch.bind(window) : fetch;

async function executeRefresh(): Promise<void> {
  // If another tab or concurrent call refreshed very recently, the cookies are already fresh.
  if (Date.now() - lastRefreshSuccess < 2000) {
    return;
  }
  const headers = new Headers();
  const csrf = getCookie('csrf_token');
  if (csrf) headers.set('X-CSRF-Token', csrf);

  const res = await rawFetch('/api/v1/auth/refresh', {
    method: 'POST',
    headers,
    credentials: 'include',
  });
  if (!res.ok) {
    throw new Error('Refresh failed');
  }
  lastRefreshSuccess = Date.now();
}

if (typeof window !== 'undefined' && !(window as any).__crmFetchIntercepted) {
  (window as any).__crmFetchIntercepted = true;
  const originalFetch = window.fetch.bind(window);
  window.fetch = async function (input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const urlStr =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : (input as Request).url || '';

    const res = await originalFetch(input, init);

    const isNonExpiry =
      Array.from(UNAUTHORIZED_IS_NOT_SESSION_EXPIRY).some((path) => urlStr.includes(path)) ||
      urlStr.includes('/api/v1/auth/refresh');

    if (res.status === 401 && !isNonExpiry) {
      try {
        await withCrossTabLock(executeRefresh);
        return await originalFetch(input, init);
      } catch {
        window.dispatchEvent(new CustomEvent('auth:logout'));
        return res;
      }
    }
    return res;
  };
}

export async function apiFetch<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers || {});

  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }

  // Attach CSRF Token for mutating requests (Findings #6, #14)
  const method = (options.method || 'GET').toUpperCase();
  if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    const csrfToken = getCookie('csrf_token');
    if (csrfToken && !headers.has('X-CSRF-Token')) {
      headers.set('X-CSRF-Token', csrfToken);
    }
  }

  let res: Response;
  try {
    res = await fetch(endpoint, {
      ...options,
      headers,
      credentials: 'include',
    });
  } catch (netErr: any) {
    throw new ApiError(
      0,
      `Verbindung zum Server fehlgeschlagen (${netErr?.message || 'Netzwerkfehler'})`,
    );
  }

  if (res.ok) {
    if (res.status === 204) return {} as T;
    const contentType = res.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      return await res.json();
    }
    const text = await res.text();
    if (!text || text.trim() === '') return {} as T;
    try {
      return JSON.parse(text);
    } catch {
      return text as unknown as T;
    }
  }

  // Server responded with an HTTP error status (4xx / 5xx)
  let errorMessage = `HTTP ${res.status}: ${res.statusText}`;
  let errJson: any = null;
  try {
    const text = await res.text();
    try {
      errJson = JSON.parse(text);
      if (errJson && (errJson.message || errJson.error)) {
        errorMessage = errJson.message || errJson.error;
      }
    } catch {
      if (text) errorMessage = text;
    }
  } catch {
    // ignore
  }

  // Notify session expiry on 401 Unauthorized
  if (res.status === 401 && !UNAUTHORIZED_IS_NOT_SESSION_EXPIRY.has(endpoint)) {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('auth:logout'));
    }
  }

  throw new ApiError(res.status, errorMessage, errJson);
}

// --- Backup (streaming download / upload) ----------------------------------

/**
 * Streams an authenticated endpoint straight to a file.
 *
 * The Blob, the object URL and the download click are created only after the
 * reader reports `done` without throwing. An interrupted stream therefore never
 * produces a file: every buffered chunk is discarded and an error is thrown. If
 * the server sent a Content-Length, the received byte count must match it before
 * anything is saved, so a truncated body can never masquerade as a complete
 * backup.
 */
export async function downloadToFile(
  endpoint: string,
  fileName: string,
  options: RequestInit = {},
): Promise<number> {
  const headers = new Headers(options.headers || {});
  if (!headers.has('X-CSRF-Token')) {
    const csrfToken = getCookie('csrf_token');
    if (csrfToken) headers.set('X-CSRF-Token', csrfToken);
  }

  let res: Response;
  try {
    res = await fetch(endpoint, { ...options, headers, credentials: 'include' });
  } catch (netErr: any) {
    throw new ApiError(
      0,
      `Verbindung zum Server fehlgeschlagen (${netErr?.message || 'Netzwerkfehler'})`,
    );
  }

  if (!res.ok) {
    const text = await res.text().catch(() => '');
    let message = `HTTP ${res.status}: ${res.statusText}`;
    let data: any = null;
    try {
      data = JSON.parse(text);
      if (data && (data.message || data.error)) message = data.message || data.error;
    } catch {
      if (text) message = text;
    }
    if (res.status === 401) {
      if (typeof window !== 'undefined') {
        window.dispatchEvent(new CustomEvent('auth:logout'));
      }
    }
    throw new ApiError(res.status, message, data);
  }

  const reader = res.body?.getReader();
  if (!reader) throw new ApiError(res.status, 'Backup-Stream nicht verfügbar');

  const chunks: Uint8Array[] = [];
  let receivedBytes = 0;
  for (;;) {
    let step: ReadableStreamReadResult<Uint8Array>;
    try {
      step = await reader.read();
    } catch (streamErr: any) {
      throw new ApiError(
        0,
        `Backup-Stream abgebrochen (${streamErr?.message || 'Verbindung unterbrochen'})`,
      );
    }
    if (step.done) break;
    if (step.value) {
      chunks.push(step.value);
      receivedBytes += step.value.byteLength;
    }
  }

  const expected = Number(res.headers.get('Content-Length') || 0);
  if (expected > 0 && receivedBytes !== expected) {
    throw new ApiError(0, `Backup unvollständig: ${receivedBytes}/${expected} Bytes`);
  }

  const blob = new Blob(chunks as BlobPart[], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  try {
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = fileName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    URL.revokeObjectURL(url);
  }
  return receivedBytes;
}

export interface RestoreResponse {
  mode: string;
  counts: Record<string, number>;
  committed: boolean;
}

/**
 * Uploads a backup envelope. The committed flag in the error payload is the
 * only trustworthy answer: a rolled-back restore and a committed restore with a
 * failed audit insert are both HTTP 500 but mean opposite things.
 */
export async function uploadBackup(
  file: File,
  mode: 'merge' | 'replace',
): Promise<RestoreResponse> {
  const headers = new Headers();
  const csrfToken = getCookie('csrf_token');
  if (csrfToken) headers.set('X-CSRF-Token', csrfToken);

  let res: Response;
  try {
    res = await fetch(`/api/v1/backup/restore?mode=${mode}`, {
      method: 'POST',
      headers,
      credentials: 'include',
      body: file,
    });
  } catch (netErr: any) {
    throw new ApiError(
      0,
      `Verbindung zum Server fehlgeschlagen (${netErr?.message || 'Netzwerkfehler'})`,
    );
  }

  const text = await res.text();
  let data: any;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }

  if (!res.ok) {
    const committed = Boolean(data && data.committed);
    throw new ApiError(res.status, (data && (data.message || data.error)) || `HTTP ${res.status}`, {
      ...data,
      committed,
    });
  }
  return data as RestoreResponse;
}

// --- E-Mail (SMTP/IMAP) --------------------------------------------------

export interface EmailAccount {
  id: string;
  name: string;
  email_address: string;
  provider: string;
  imap_host: string;
  imap_port: number;
  smtp_host: string;
  smtp_port: number;
  username: string;
  has_password: boolean;
  is_active: boolean;
  account_type: string;
  last_sync_at?: string;
}

export interface EmailAccountInput {
  name: string;
  email_address?: string;
  provider?: string;
  imap_host: string;
  imap_port: number;
  smtp_host: string;
  smtp_port: number;
  username: string;
  password?: string;
  is_active?: boolean;
  account_type?: string;
}

export function getEmailAccounts(): Promise<EmailAccount[]> {
  return apiFetch<EmailAccount[]>('/api/v1/emails/accounts');
}

export function createEmailAccount(input: EmailAccountInput): Promise<EmailAccount> {
  return apiFetch<EmailAccount>('/api/v1/emails/accounts', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function updateEmailAccount(
  id: string,
  input: Partial<EmailAccountInput>,
): Promise<EmailAccount> {
  return apiFetch<EmailAccount>(`/api/v1/emails/accounts/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(input),
  });
}

export function deleteEmailAccount(id: string): Promise<{ success: boolean }> {
  return apiFetch<{ success: boolean }>(`/api/v1/emails/accounts/${id}`, { method: 'DELETE' });
}

export function testEmailAccount(id: string): Promise<{ success: boolean }> {
  return apiFetch<{ success: boolean }>(`/api/v1/emails/accounts/${id}/test`, {
    method: 'POST',
  });
}

export function syncEmailAccount(id: string): Promise<{ success: boolean; new_messages: number }> {
  return apiFetch<{ success: boolean; new_messages: number }>(
    `/api/v1/emails/accounts/${id}/sync`,
    { method: 'POST' },
  );
}

export interface SendEmailInput {
  account_id: string;
  from?: string;
  to: string[];
  cc?: string[];
  subject: string;
  body_text?: string;
  body_html?: string;
  in_reply_to?: string;
}

export function sendEmail(input: SendEmailInput): Promise<any> {
  return apiFetch<any>('/api/v1/emails/send', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export function tagEmailMessage(id: string, tags: string[]): Promise<any> {
  return apiFetch<any>(`/api/v1/emails/${id}/tags`, {
    method: 'PATCH',
    body: JSON.stringify({ tags }),
  });
}

export interface TestRunPayload {
  target_id: string;
  target_type: string;
  target_name: string;
}

export const testRunWorkflow = (id: string, payload: TestRunPayload) =>
  apiFetch<any>(`/api/v1/automations/${id}/test-run`, {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const approveWorkflowStep = (runId: string) =>
  apiFetch<any>(`/api/v1/automations/runs/${runId}/approve`, {
    method: 'POST',
  });
