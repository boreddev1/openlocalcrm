import React, { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { useNavigate } from 'react-router-dom';
import { Lock, Mail, ShieldCheck, KeyRound, Sparkles, UserCheck } from 'lucide-react';
import { apiFetch } from '../api/client';

export const LoginPage: React.FC = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [totpCode, setTotpCode] = useState('');
  const [totpRequired, setTotpRequired] = useState(false);
  const [isDemo, setIsDemo] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { login } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    apiFetch<{ demo_mode?: boolean }>('/api/v1/health')
      .then((res) => {
        if (res && res.demo_mode) {
          setIsDemo(true);
          setEmail('admin@openlocalcrm.local');
          setPassword('demo123');
        }
      })
      .catch(() => {});
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsSubmitting(true);
    try {
      const result = await login(email, password, totpRequired ? totpCode : undefined);
      if (result.totpRequired) {
        setTotpRequired(true);
        setError(null);
      } else {
        navigate('/');
      }
    } catch (err: any) {
      setError(err?.message || 'Ungültige Anmeldedaten oder Server nicht erreichbar');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleQuickFill = (demoEmail: string) => {
    setEmail(demoEmail);
    setPassword('demo123');
    setTotpRequired(false);
    setTotpCode('');
    setError(null);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-950 p-4">
      <div className="max-w-md w-full bg-slate-900 border border-slate-800 rounded-3xl p-8 shadow-2xl space-y-6">
        {/* Brand */}
        <div className="text-center space-y-2">
          <div className="w-12 h-12 rounded-2xl bg-emerald-500 text-slate-950 font-black text-2xl flex items-center justify-center mx-auto shadow-xl shadow-emerald-500/20">
            O
          </div>
          <h2 className="text-2xl font-bold text-slate-100">OpenLocalCRM</h2>
          <p className="text-xs text-slate-400">Sicherer Single-Tenant Anmeldebereich für Vertriebsteams</p>
        </div>

        {/* Demo Mode Notice & Quick Fills */}
        {isDemo && (
          <div className="bg-amber-500/10 border border-amber-500/30 rounded-2xl p-4 space-y-3">
            <div className="flex items-center gap-2 text-amber-300 text-xs font-semibold">
              <Sparkles className="w-4 h-4 text-amber-400 shrink-0" />
              <span>Demo-Modus aktiv (In-Memory ohne PostgreSQL)</span>
            </div>
            <p className="text-[11px] text-amber-200/80 leading-relaxed">
              Wählen Sie ein vorbereitetes Test-Konto für die Präsentation:
            </p>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => handleQuickFill('admin@openlocalcrm.local')}
                className={`px-3 py-2 text-xs font-semibold rounded-xl border transition-all text-left flex flex-col gap-0.5 ${
                  email === 'admin@openlocalcrm.local'
                    ? 'bg-amber-500/25 border-amber-400 text-amber-200 shadow-sm'
                    : 'bg-slate-950/60 border-amber-500/20 text-slate-300 hover:border-amber-500/40'
                }`}
              >
                <span className="flex items-center gap-1 text-amber-300 font-bold">
                  <UserCheck className="w-3 h-3" /> Admin
                </span>
                <span className="text-[10px] text-slate-400">demo123</span>
              </button>

              <button
                type="button"
                onClick={() => handleQuickFill('vertrieb@openlocalcrm.local')}
                className={`px-3 py-2 text-xs font-semibold rounded-xl border transition-all text-left flex flex-col gap-0.5 ${
                  email === 'vertrieb@openlocalcrm.local'
                    ? 'bg-amber-500/25 border-amber-400 text-amber-200 shadow-sm'
                    : 'bg-slate-950/60 border-amber-500/20 text-slate-300 hover:border-amber-500/40'
                }`}
              >
                <span className="flex items-center gap-1 text-amber-300 font-bold">
                  <UserCheck className="w-3 h-3" /> Vertrieb
                </span>
                <span className="text-[10px] text-slate-400">demo123</span>
              </button>
            </div>
          </div>
        )}

        {error && (
          <div className="p-3 bg-rose-500/10 border border-rose-500/30 text-rose-300 rounded-xl text-xs flex items-center gap-2">
            <span>⚠️</span>
            <span>{error}</span>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1.5">E-Mail-Adresse</label>
            <div className="relative">
              <Mail className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
              <input
                type="email"
                required
                value={email}
                disabled={totpRequired}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full pl-10 pr-4 py-2.5 bg-slate-950 border border-slate-800 rounded-xl text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500 disabled:opacity-60"
                placeholder="name@unternehmen.de"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1.5">Passwort</label>
            <div className="relative">
              <Lock className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
              <input
                type="password"
                required
                value={password}
                disabled={totpRequired}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full pl-10 pr-4 py-2.5 bg-slate-950 border border-slate-800 rounded-xl text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500 disabled:opacity-60"
                placeholder="••••••••"
              />
            </div>
          </div>

          {totpRequired && (
            <div className="p-4 bg-emerald-500/10 border border-emerald-500/30 rounded-2xl space-y-2 animate-in fade-in">
              <label className="block text-xs font-bold text-emerald-300 flex items-center gap-1.5">
                <KeyRound className="w-4 h-4" /> 2FA Authentifizierungscode
              </label>
              <input
                type="text"
                required
                autoFocus
                maxLength={6}
                value={totpCode}
                onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ''))}
                className="w-full text-center tracking-[0.4em] font-mono text-lg font-bold py-2.5 bg-slate-950 border border-emerald-500/50 rounded-xl text-emerald-300 placeholder-slate-600 focus:outline-none focus:ring-2 focus:ring-emerald-500"
                placeholder="000000"
              />
              <p className="text-[11px] text-slate-400">
                Geben Sie den 6-stelligen Zahlencode aus Ihrer Authenticator-App ein.
              </p>
            </div>
          )}

          <button
            type="submit"
            disabled={isSubmitting}
            className="w-full py-3 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-slate-950 font-bold rounded-xl text-sm transition-all shadow-lg shadow-emerald-600/25 flex items-center justify-center gap-2 cursor-pointer"
          >
            <ShieldCheck className="w-4 h-4" />
            {isSubmitting
              ? 'Wird überprüft...'
              : totpRequired
              ? 'Code bestätigen & Anmelden'
              : 'Anmelden'}
          </button>

          {totpRequired && (
            <button
              type="button"
              onClick={() => {
                setTotpRequired(false);
                setTotpCode('');
                setError(null);
              }}
              className="w-full text-xs text-slate-400 hover:text-slate-200 text-center py-1 transition-colors"
            >
              Abbrechen und mit anderem Konto anmelden
            </button>
          )}
        </form>

        <div className="text-center text-xs text-slate-500">
          Rechtssicher & DSGVO-konform (Single-Tenant)
        </div>
      </div>
    </div>
  );
};
