import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { 
  Search, 
  Users, 
  Building2, 
  KanbanSquare, 
  Calendar, 
  CheckSquare, 
  Zap, 
  Mail, 
  Sparkles, 
  BarChart3, 
  Settings, 
  MapPin,
  X
} from 'lucide-react';

interface CommandPaletteProps {
  isOpen: boolean;
  onClose: () => void;
}

export const CommandPalette: React.FC<CommandPaletteProps> = ({ isOpen, onClose }) => {
  const [query, setQuery] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        if (isOpen) onClose();
      }
      if (e.key === 'Escape' && isOpen) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const quickLinks = [
    { label: 'Deals & Pipeline', path: '/deals', icon: KanbanSquare, category: 'CRM Kern' },
    { label: 'Kontakte & Leads', path: '/contacts', icon: Users, category: 'CRM Kern' },
    { label: 'Firmen & B2B', path: '/companies', icon: Building2, category: 'CRM Kern' },
    { label: 'Termine & Kalender', path: '/calendar', icon: Calendar, category: 'CRM Kern' },
    { label: 'Aufgaben / Todos', path: '/todos', icon: CheckSquare, category: 'CRM Kern' },
    { label: 'Automatisierung & Workflows', path: '/automations', icon: Zap, category: 'Automatisierung' },
    { label: 'E-Mail Inbox & Tagging', path: '/inbox', icon: Mail, category: 'Kommunikation' },
    { label: 'KI-Copilot & Research', path: '/agent', icon: Sparkles, category: 'KI & Agent' },
    { label: 'D2D / Gebietskarte', path: '/map', icon: MapPin, category: 'Feldvertrieb' },
    { label: 'Vertriebs-Report & Forecast', path: '/reports', icon: BarChart3, category: 'Reporting' },
    { label: 'Systemeinstellungen & 2FA', path: '/settings', icon: Settings, category: 'Administration' },
  ];

  const searchResults = [
    { label: 'Dr. Michael Weber (Kontakt)', path: '/contacts', icon: Users, category: 'Kontakte' },
    { label: 'Sabine Mustermann (Kontakt)', path: '/contacts', icon: Users, category: 'Kontakte' },
    { label: 'Energie Südwest GmbH (Firma)', path: '/companies', icon: Building2, category: 'Firmen' },
    { label: '30 kWp Gewerbedach Solaranlage (Deal)', path: '/deals', icon: KanbanSquare, category: 'Deals' },
    { label: 'Wärmepumpe & 15 kWp PV (Deal)', path: '/deals', icon: KanbanSquare, category: 'Deals' },
  ];

  const filteredLinks = quickLinks.filter((item) =>
    item.label.toLowerCase().includes(query.toLowerCase())
  );

  const filteredResults = searchResults.filter((item) =>
    item.label.toLowerCase().includes(query.toLowerCase())
  );

  const handleSelect = (path: string) => {
    navigate(path);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-start justify-center pt-24 p-4">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl max-w-xl w-full shadow-2xl overflow-hidden animate-fadeIn">
        {/* Search Header */}
        <div className="flex items-center px-4 py-3.5 border-b border-slate-800 gap-3">
          <Search className="w-5 h-5 text-slate-400 shrink-0" />
          <input
            type="text"
            placeholder="Suche nach Kontakten, Deals, Seiten oder Befehlen... (z.B. Deals, Weber, PV)"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full bg-transparent text-sm text-slate-100 placeholder-slate-500 focus:outline-none"
            autoFocus
          />
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Results Body */}
        <div className="max-h-96 overflow-y-auto p-2 space-y-3 divide-y divide-slate-800/60">
          {query.trim().length > 0 && filteredResults.length > 0 && (
            <div className="space-y-1">
              <span className="text-[10px] font-bold uppercase text-slate-500 px-3 tracking-wider">Suchtreffer</span>
              {filteredResults.map((item) => {
                const Icon = item.icon;
                return (
                  <button
                    key={item.label}
                    onClick={() => handleSelect(item.path)}
                    className="w-full flex items-center justify-between px-3 py-2 rounded-xl text-xs text-slate-200 hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <div className="flex items-center gap-2.5">
                      <Icon className="w-4 h-4 text-emerald-400 shrink-0" />
                      <span>{item.label}</span>
                    </div>
                    <span className="text-[10px] text-slate-500">{item.category}</span>
                  </button>
                );
              })}
            </div>
          )}

          <div className="space-y-1 pt-2">
            <span className="text-[10px] font-bold uppercase text-slate-500 px-3 tracking-wider">Navigation & Module</span>
            {filteredLinks.map((item) => {
              const Icon = item.icon;
              return (
                <button
                  key={item.label}
                  onClick={() => handleSelect(item.path)}
                  className="w-full flex items-center justify-between px-3 py-2 rounded-xl text-xs text-slate-200 hover:bg-slate-800/80 transition-colors text-left"
                >
                  <div className="flex items-center gap-2.5">
                    <Icon className="w-4 h-4 text-slate-400 shrink-0" />
                    <span>{item.label}</span>
                  </div>
                  <span className="text-[10px] text-slate-500">{item.category}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Footer Shortcut hints */}
        <div className="p-3 bg-slate-950 border-t border-slate-800 text-[11px] text-slate-500 flex items-center justify-between px-4">
          <span>Drücken Sie <kbd className="px-1.5 py-0.5 bg-slate-800 rounded border border-slate-700 text-slate-300 font-mono">ESC</kbd> zum Schließen</span>
          <span><kbd className="px-1.5 py-0.5 bg-slate-800 rounded border border-slate-700 text-slate-300 font-mono">⌘K</kbd> / <kbd className="px-1.5 py-0.5 bg-slate-800 rounded border border-slate-700 text-slate-300 font-mono">Ctrl+K</kbd></span>
        </div>
      </div>
    </div>
  );
};
