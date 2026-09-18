import React, { useEffect, useState } from 'react';
import { UserCheck, Sparkles, X } from 'lucide-react';
import { apiFetch } from '../../api/client';
import { useAuth } from '../../context/AuthContext';

interface HealthResponse {
  status: string;
  demo_mode?: boolean;
}

export const DemoBanner: React.FC = () => {
  const [isDemo, setIsDemo] = useState<boolean>(false);
  const [closed, setClosed] = useState<boolean>(false);
  const { user, login } = useAuth();

  useEffect(() => {
    apiFetch<HealthResponse>('/api/v1/health')
      .then((res) => {
        if (res && res.demo_mode) {
          setIsDemo(true);
        }
      })
      .catch(() => {
        // Backend not reachable yet
      });
  }, []);

  if (!isDemo || closed) return null;

  const handleQuickSwitch = async (email: string) => {
    try {
      await login(email, 'demo123');
    } catch (err: any) {
      console.error('Demo login switch error:', err);
    }
  };

  return (
    <div className="bg-gradient-to-r from-amber-500/20 via-amber-600/15 to-amber-500/20 border-b border-amber-500/30 px-4 py-2 text-amber-200 text-xs flex flex-wrap items-center justify-between gap-3 shadow-md backdrop-blur-sm z-50">
      <div className="flex items-center gap-2 font-medium">
        <Sparkles className="w-4 h-4 text-amber-400 shrink-0" />
        <span>
          <strong className="font-bold text-amber-300">Demo-Modus aktiv:</strong> Autarke In-Memory Testumgebung ohne externe PostgreSQL-Datenbank.
        </span>
        {user && (
          <span className="hidden md:inline-block bg-amber-400/10 px-2 py-0.5 rounded border border-amber-400/20 text-amber-300">
            Angemeldet als: <strong>{user.email}</strong> ({user.role})
          </span>
        )}
      </div>

      <div className="flex items-center gap-2">
        <span className="text-amber-400/80 hidden sm:inline">Schnell-Wechsel:</span>
        <button
          onClick={() => handleQuickSwitch('admin@openlocalcrm.local')}
          className="px-2.5 py-1 bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/40 rounded-lg text-amber-200 font-semibold transition-all hover:scale-105 active:scale-95 flex items-center gap-1.5"
        >
          <UserCheck className="w-3.5 h-3.5 text-amber-400" />
          Admin (demo123)
        </button>
        <button
          onClick={() => handleQuickSwitch('vertrieb@openlocalcrm.local')}
          className="px-2.5 py-1 bg-slate-800/80 hover:bg-slate-800 border border-amber-500/30 rounded-lg text-amber-300 font-medium transition-all hover:scale-105 active:scale-95"
        >
          Vertrieb (demo123)
        </button>
        <button
          onClick={() => setClosed(true)}
          className="p-1 hover:bg-amber-500/20 rounded text-amber-400/70 hover:text-amber-300 transition-colors ml-1"
          title="Banner ausblenden"
          aria-label="Banner ausblenden"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>
    </div>
  );
};
