import React, { useState } from 'react';
import { useAuth } from '../../context/AuthContext';
import { useSSE } from '../../hooks/useSSE';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../../api/client';
import { Bell, Wifi, WifiOff, LogOut, User, Check, Menu, Search } from 'lucide-react';

interface HeaderProps {
  onMenuToggle?: () => void;
  onOpenSearch?: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onMenuToggle, onOpenSearch }) => {
  const { user, logout } = useAuth();
  const { connected } = useSSE();
  const queryClient = useQueryClient();
  const [isNotificationOpen, setIsNotificationOpen] = useState(false);

  const { data: notifications = [] } = useQuery<any[]>({
    queryKey: ['notifications'],
    queryFn: () => apiFetch('/api/v1/notifications/unread'),
    refetchInterval: 15000,
  });

  const markAllReadMutation = useMutation({
    mutationFn: () => apiFetch('/api/v1/notifications/read-all', { method: 'POST' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  return (
    <header className="h-16 border-b border-slate-800 bg-slate-900/50 backdrop-blur-md px-6 flex items-center justify-between sticky top-0 z-30">
      {/* Brand & Search Quick Switcher */}
      <div className="flex items-center gap-4">
        {onMenuToggle && (
          <button onClick={onMenuToggle} className="lg:hidden p-1.5 text-slate-400 hover:text-slate-200">
            <Menu className="w-5 h-5" />
          </button>
        )}
        <div className="hidden sm:flex items-center gap-2 text-sm text-slate-400">
          <span className="font-semibold text-slate-200">OpenLocalCRM</span>
          <span className="text-slate-600">/</span>
          <span className="text-xs">Single-Tenant</span>
        </div>

        {/* Search Bar / Command Palette trigger */}
        <button
          onClick={onOpenSearch}
          className="flex items-center gap-2 px-3 py-1.5 bg-slate-950/80 hover:bg-slate-950 text-slate-400 hover:text-slate-200 border border-slate-800 rounded-xl text-xs transition-colors"
        >
          <Search className="w-3.5 h-3.5 text-slate-500" />
          <span className="hidden md:inline">Schnellsuche...</span>
          <kbd className="hidden md:inline text-[10px] px-1.5 py-0.5 bg-slate-800 rounded border border-slate-700 text-slate-400 font-mono">⌘K</kbd>
        </button>
      </div>

      {/* Actions & Profile */}
      <div className="flex items-center gap-4">
        {/* SSE Realtime Status Badge */}
        <div
          title={connected ? 'Echtzeit-Synchronisation aktiv' : 'Keine SSE Verbindung'}
          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border ${
            connected
              ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
              : 'bg-rose-500/10 text-rose-400 border-rose-500/20'
          }`}
        >
          {connected ? <Wifi className="w-3.5 h-3.5" /> : <WifiOff className="w-3.5 h-3.5" />}
          <span className="hidden sm:inline">{connected ? 'Live Sync' : 'Offline'}</span>
        </div>

        {/* Notifications Dropdown (§14) */}
        <div className="relative">
          <button
            onClick={() => setIsNotificationOpen(!isNotificationOpen)}
            aria-label="Benachrichtigungen"
            className="relative p-2 rounded-xl text-slate-400 hover:text-slate-200 hover:bg-slate-800 transition-colors"
          >
            <Bell className="w-4 h-4" />
            {notifications.length > 0 && (
              <span className="absolute top-1.5 right-1.5 w-2 h-2 bg-emerald-500 rounded-full animate-pulse ring-2 ring-slate-900" />
            )}
          </button>

          {isNotificationOpen && (
            <div className="absolute right-0 mt-2 w-80 bg-slate-900 border border-slate-800 rounded-2xl shadow-2xl p-4 space-y-3 z-50">
              <div className="flex items-center justify-between border-b border-slate-800 pb-2">
                <span className="text-xs font-bold uppercase text-slate-400">Benachrichtigungen</span>
                {notifications.length > 0 && (
                  <button
                    onClick={() => markAllReadMutation.mutate()}
                    className="text-[11px] text-emerald-400 hover:text-emerald-300 flex items-center gap-1"
                  >
                    <Check className="w-3 h-3" /> Als gelesen markieren
                  </button>
                )}
              </div>

              <div className="divide-y divide-slate-800/60 max-h-64 overflow-y-auto">
                {notifications.length === 0 ? (
                  <div className="py-6 text-center text-xs text-slate-500">Keine neuen Benachrichtigungen</div>
                ) : (
                  notifications.map((n) => (
                    <div key={n.id} className="py-2.5 space-y-1">
                      <div className="text-xs font-bold text-slate-200">{n.title}</div>
                      <div className="text-[11px] text-slate-400 leading-snug">{n.message}</div>
                    </div>
                  ))
                )}
              </div>
            </div>
          )}
        </div>

        {/* User Info & Logout */}
        <div className="flex items-center gap-3 pl-2 border-l border-slate-800">
          <div className="w-8 h-8 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center text-slate-300">
            <User className="w-4 h-4" />
          </div>
          <div className="hidden md:block text-left">
            <div className="text-xs font-medium text-slate-200">{user?.email || 'Nicht angemeldet'}</div>
            <div className="text-[10px] text-slate-500">{user?.role || 'GAST'}</div>
          </div>
          {user && (
            <button
              onClick={logout}
              title="Abmelden"
              className="p-2 text-slate-500 hover:text-rose-400 transition-colors"
            >
              <LogOut className="w-4 h-4" />
            </button>
          )}
        </div>
      </div>
    </header>
  );
};
