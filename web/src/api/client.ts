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
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(new RegExp('(^|;\\s*)(' + name + ')=([^;]*)'));
  return match ? decodeURIComponent(match[3]) : null;
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
  if (res.status === 401 && endpoint !== '/api/v1/auth/login') {
    if (typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('auth:logout'));
    }
  }

  throw new ApiError(res.status, errorMessage, errJson);
}
