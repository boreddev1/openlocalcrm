import React from 'react';
import { Link } from 'react-router-dom';
import { AlertTriangle, KeyRound } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';

export const PasswordNagBanner: React.FC = () => {
  const { user } = useAuth();

  // Non-dismissable: stays visible while must_change_password === true
  // null means unknown, never rendered as "no change needed"
  if (!user || user.must_change_password !== true) {
    return null;
  }

  return (
    <div
      data-testid="password-nag-banner"
      className="bg-rose-500/15 border-b border-rose-500/30 px-4 py-2 text-rose-200 text-xs flex flex-wrap items-center justify-between gap-3 shadow-md backdrop-blur-sm z-50"
    >
      <div className="flex items-center gap-2 font-medium">
        <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />
        <span>
          <strong className="font-bold text-rose-300">Sicherheitshinweis:</strong> Bitte ändern Sie
          Ihr initiales Passwort umgehend in den Systemeinstellungen (§2.2).
        </span>
      </div>

      <Link
        to="/settings"
        className="px-3 py-1 bg-rose-600 hover:bg-rose-500 text-white font-semibold rounded-lg text-xs transition-colors flex items-center gap-1.5 shrink-0"
      >
        <KeyRound className="w-3.5 h-3.5" />
        Passwort jetzt ändern
      </Link>
    </div>
  );
};
