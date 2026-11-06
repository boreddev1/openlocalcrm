import React, { useCallback, useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Check, AlertCircle, KeyRound } from 'lucide-react';

interface InviteInfo {
  valid: boolean;
  email: string;
  first_name: string;
  last_name: string;
  expires_at: string;
}

type Phase = 'validating' | 'ready' | 'submitting' | 'accepted' | 'unusable';

const MIN_PASSWORD_LENGTH = 8;

interface ValidationOutcome {
  info: InviteInfo | null;
  error: string | null;
}

/**
 * One network round trip per token, even when the component mounts twice.
 *
 * Both accept routes share the login rate limiter, which allows five requests
 * per IP per minute and counts every request. React 18's StrictMode double-mount
 * in development would otherwise spend two of them on the same read-only check.
 * A full page load resets this module, so re-opening the link re-validates.
 */
const validationRequests = new Map<string, Promise<ValidationOutcome>>();

function validateOnce(token: string): Promise<ValidationOutcome> {
  const cached = validationRequests.get(token);
  if (cached) return cached;

  const pending = (async (): Promise<ValidationOutcome> => {
    try {
      const res = await fetch(`/api/v1/auth/accept-invite?token=${encodeURIComponent(token)}`, {
        credentials: 'include',
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok || body?.valid !== true) {
        return {
          info: null,
          error:
            body?.message ||
            'Diese Einladung ist nicht mehr gültig. Sie wurde bereits verwendet, wurde widerrufen oder ist abgelaufen.',
        };
      }
      return { info: body as InviteInfo, error: null };
    } catch {
      return {
        info: null,
        error: 'Der Server ist nicht erreichbar. Bitte versuchen Sie es später erneut.',
      };
    }
  })();

  validationRequests.set(token, pending);
  return pending;
}

/**
 * AcceptInvitePage is the public target of an invite link.
 *
 * It is deliberately session-free: it validates the token with the read-only GET
 * endpoint and submits the chosen password with POST. Neither call sets a cookie,
 * because login remains the only session-establishing operation in this codebase.
 */
export const AcceptInvitePage: React.FC = () => {
  const [searchParams] = useSearchParams();
  const token = (searchParams.get('token') || '').trim();

  const [phase, setPhase] = useState<Phase>('validating');
  const [invite, setInvite] = useState<InviteInfo | null>(null);
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);

  const validate = useCallback(async () => {
    if (!token) {
      setPhase('unusable');
      setError(
        'Dieser Link enthält kein Einladungstoken. Bitte öffnen Sie den vollständigen Link aus der Einladung oder fordern Sie eine neue Einladung an.',
      );
      return;
    }
    setPhase('validating');
    setError(null);
    const { info, error: failure } = await validateOnce(token);
    if (info) {
      setInvite(info);
      setPhase('ready');
      return;
    }
    setPhase('unusable');
    setError(failure);
  }, [token]);

  useEffect(() => {
    void validate();
  }, [validate]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Das Passwort muss mindestens ${MIN_PASSWORD_LENGTH} Zeichen lang sein.`);
      return;
    }
    if (password !== confirmPassword) {
      setError('Die Passwörter stimmen nicht überein.');
      return;
    }

    setPhase('submitting');
    try {
      const res = await fetch('/api/v1/auth/accept-invite', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ token, password }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) {
        setPhase('ready');
        setError(body?.message || 'Die Einladung konnte nicht angenommen werden.');
        return;
      }
      setPhase('accepted');
    } catch {
      setPhase('ready');
      setError('Der Server ist nicht erreichbar. Bitte versuchen Sie es später erneut.');
    }
  };

  return (
    <div className="min-h-screen bg-slate-950 flex items-center justify-center px-4">
      <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl p-8 space-y-6">
        <div>
          <h1 className="text-xl font-bold text-slate-100 flex items-center gap-2">
            <KeyRound className="w-5 h-5 text-emerald-400" />
            Einladung annehmen
          </h1>
          <p className="text-xs text-slate-400 mt-1">
            Single-Tenant Anmeldebereich für Vertriebsteams
          </p>
        </div>

        {phase === 'validating' && (
          <div className="flex items-center gap-3 text-slate-400 text-sm">
            <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-emerald-500" />
            <span>Einladung wird geprüft...</span>
          </div>
        )}

        {phase === 'unusable' && (
          <div
            data-testid="invite-unusable"
            className="p-4 bg-rose-500/10 border border-rose-500/20 rounded-xl text-sm text-rose-300 flex items-start gap-2"
          >
            <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {phase === 'accepted' && (
          <div
            data-testid="invite-accepted"
            className="p-4 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-sm text-emerald-300 flex items-start gap-2"
          >
            <Check className="w-4 h-4 mt-0.5 shrink-0" />
            <div>
              <p className="font-semibold">Ihr Konto ist jetzt aktiv.</p>
              <p className="mt-1 text-emerald-200/80">
                Melden Sie sich jetzt mit Ihrem neuen Passwort an. Diese Seite hat Sie nicht
                angemeldet.
              </p>
            </div>
          </div>
        )}

        {phase === 'unusable' && (
          <Link
            to="/login"
            className="block text-center px-4 py-2.5 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-semibold transition-colors"
          >
            Zum Login
          </Link>
        )}

        {phase === 'accepted' && (
          <Link
            to="/login"
            className="block text-center px-4 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors"
          >
            Zum Login
          </Link>
        )}

        {(phase === 'ready' || phase === 'submitting') && invite && (
          <>
            <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-300 space-y-1">
              <p>
                <span className="text-slate-500">Eingeladenes Konto:</span>{' '}
                <span className="font-semibold text-slate-100">{invite.email}</span>
              </p>
              <p>
                <span className="text-slate-500">Name:</span>{' '}
                <span className="text-slate-200">
                  {[invite.first_name, invite.last_name].filter(Boolean).join(' ')}
                </span>
              </p>
              <p>
                <span className="text-slate-500">Link gültig bis:</span>{' '}
                <span className="text-slate-200">
                  {new Date(invite.expires_at).toLocaleString('de-DE')}
                </span>
              </p>
            </div>

            {error && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/20 rounded-xl text-xs text-rose-300 flex items-start gap-2">
                <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <form onSubmit={submit} className="space-y-4">
              <div>
                <label
                  htmlFor="invite-password"
                  className="block text-xs font-semibold text-slate-300 mb-1"
                >
                  Neues Passwort
                </label>
                <input
                  id="invite-password"
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={`Min. ${MIN_PASSWORD_LENGTH} Zeichen`}
                  autoComplete="new-password"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label
                  htmlFor="invite-password-confirm"
                  className="block text-xs font-semibold text-slate-300 mb-1"
                >
                  Passwort wiederholen
                </label>
                <input
                  id="invite-password-confirm"
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="Passwort wiederholen"
                  autoComplete="new-password"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <button
                type="submit"
                disabled={phase === 'submitting'}
                className="w-full px-4 py-2.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-slate-950 font-bold rounded-xl text-xs flex items-center justify-center gap-2 transition-colors"
              >
                <Check className="w-3.5 h-3.5" />
                <span>{phase === 'submitting' ? 'Wird übernommen...' : 'Einladung annehmen'}</span>
              </button>
            </form>
          </>
        )}
      </div>
    </div>
  );
};
