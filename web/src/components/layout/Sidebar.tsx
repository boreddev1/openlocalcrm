import React from 'react';
import { NavLink } from 'react-router-dom';
import { 
  LayoutDashboard, 
  Users, 
  Building2, 
  KanbanSquare, 
  CheckSquare, 
  Calendar,
  BarChart3,
  MapPin, 
  Mail, 
  Sparkles, 
  Zap,
  Settings 
} from 'lucide-react';

interface SidebarProps {
  isOpen: boolean;
  onClose: () => void;
}

export const Sidebar: React.FC<SidebarProps> = ({ isOpen, onClose }) => {
  const links = [
    { to: '/', label: 'Dashboard', icon: LayoutDashboard },
    { to: '/deals', label: 'Deals & Pipeline', icon: KanbanSquare },
    { to: '/contacts', label: 'Kontakte & Leads', icon: Users },
    { to: '/companies', label: 'Firmen', icon: Building2 },
    { to: '/calendar', label: 'Termine & Kalender', icon: Calendar },
    { to: '/todos', label: 'Aufgaben / Todos', icon: CheckSquare },
    { to: '/automations', label: 'Automatisierung & Workflows', icon: Zap },
    { to: '/map', label: 'D2D / Gebietskarte', icon: MapPin },
    { to: '/inbox', label: 'E-Mail Inbox & Tags', icon: Mail },
    { to: '/agent', label: 'KI-Copilot & Research', icon: Sparkles },
    { to: '/reports', label: 'Vertriebs-Report', icon: BarChart3 },
    { to: '/settings', label: 'Einstellungen', icon: Settings },
  ];

  return (
    <>
      {/* Mobile Backdrop */}
      {isOpen && (
        <div 
          className="fixed inset-0 bg-black/60 z-40 md:hidden backdrop-blur-sm transition-opacity"
          onClick={onClose}
        />
      )}

      <aside className={`
        fixed md:static inset-y-0 left-0 z-50
        w-64 bg-slate-900 border-r border-slate-800 flex flex-col
        transform transition-transform duration-200 ease-in-out
        ${isOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'}
      `}>
        {/* Brand Header */}
        <div className="h-16 flex items-center px-6 border-b border-slate-800 gap-3">
          <div className="w-8 h-8 rounded-lg bg-emerald-500 flex items-center justify-center font-bold text-slate-950 shadow-lg shadow-emerald-500/20">
            O
          </div>
          <div>
            <h1 className="font-bold text-slate-100 text-lg leading-tight tracking-tight">OpenLocalCRM</h1>
            <span className="text-[10px] text-emerald-400 font-medium uppercase tracking-wider">Single-Tenant CRM</span>
          </div>
        </div>

        {/* Navigation Links */}
        <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
          {links.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                onClick={onClose}
                className={({ isActive }) => `
                  flex items-center gap-3 px-3 py-2 rounded-lg text-xs font-medium transition-colors
                  ${isActive 
                    ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' 
                    : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'}
                `}
              >
                <Icon className="w-4 h-4 shrink-0" />
                <span>{item.label}</span>
              </NavLink>
            );
          })}
        </nav>

        {/* Footer info */}
        <div className="p-4 border-t border-slate-800 text-xs text-slate-500 flex items-center justify-between">
          <span>OpenLocalCRM v3.0.0</span>
          <span className="inline-flex items-center gap-1.5 text-emerald-400">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
            Lokal
          </span>
        </div>
      </aside>
    </>
  );
};
