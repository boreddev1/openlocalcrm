import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiFetch, getFieldText } from '../api/client';
import { Users, Building2, KanbanSquare, CheckSquare, ArrowUpRight, Plus } from 'lucide-react';
import { Link } from 'react-router-dom';

export const DashboardPage: React.FC = () => {
  const { data: rawContacts = [] } = useQuery<any[]>({
    queryKey: ['contacts'],
    queryFn: () => apiFetch('/api/v1/contacts?limit=5'),
  });
  const contacts = Array.isArray(rawContacts) ? rawContacts : [];

  const { data: rawTodos = [] } = useQuery<any[]>({
    queryKey: ['todos'],
    queryFn: () => apiFetch('/api/v1/todos?limit=5'),
  });
  const todos = Array.isArray(rawTodos) ? rawTodos : [];

  const { data: rawDeals = [] } = useQuery<any[]>({
    queryKey: ['deals'],
    queryFn: () => apiFetch('/api/v1/deals'),
  });
  const deals = Array.isArray(rawDeals) ? rawDeals : [];

  const { data: rawCompanies = [] } = useQuery<any[]>({
    queryKey: ['companies'],
    queryFn: () => apiFetch('/api/v1/companies'),
  });
  const companies = Array.isArray(rawCompanies) ? rawCompanies : [];

  const metrics = [
    {
      title: 'Aktive Kontakte',
      value: contacts.length.toString(),
      icon: Users,
      change: contacts.length > 0 ? 'Im Adressbuch' : 'Keine Kontakte',
    },
    {
      title: 'Offene Deals',
      value: deals.length.toString(),
      icon: KanbanSquare,
      change: deals.length > 0 ? 'Pipeline aktiv' : 'Keine Deals',
    },
    {
      title: 'Offene Aufgaben',
      value: todos.length.toString(),
      icon: CheckSquare,
      change: todos.length > 0 ? 'Für heute geplant' : 'Keine Aufgaben',
    },
    {
      title: 'Firmen im Portfolio',
      value: companies.length.toString(),
      icon: Building2,
      change: companies.length > 0 ? 'B2B & D2D' : 'Keine Firmen',
    },
  ];

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Page Title & Action */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">Vertriebs-Dashboard</h1>
          <p className="text-sm text-slate-400">
            Übersicht über Leads, Deals, Termine und Team-Aufgaben
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Link
            to="/contacts"
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20"
          >
            <Plus className="w-4 h-4" />
            Neuer Kontakt
          </Link>
        </div>
      </div>

      {/* Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {metrics.map((m, idx) => {
          const Icon = m.icon;
          return (
            <div
              key={idx}
              className="bg-slate-900 border border-slate-800 rounded-xl p-5 shadow-sm"
            >
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-400">{m.title}</span>
                <div className="w-8 h-8 rounded-lg bg-slate-800 flex items-center justify-center text-emerald-400">
                  <Icon className="w-4 h-4" />
                </div>
              </div>
              <div className="mt-3 text-2xl font-bold text-slate-100">{m.value}</div>
              <div className="mt-1 text-xs text-slate-500">{m.change}</div>
            </div>
          );
        })}
      </div>

      {/* Split section: Recent Contacts & Tasks */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Recent Contacts */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 flex flex-col">
          <div className="flex items-center justify-between pb-4 border-b border-slate-800">
            <h2 className="font-semibold text-slate-200 text-base">Aktuelle Kontakte</h2>
            <Link
              to="/contacts"
              className="text-xs text-emerald-400 hover:underline inline-flex items-center gap-1"
            >
              Alle anzeigen <ArrowUpRight className="w-3 h-3" />
            </Link>
          </div>

          <div className="divide-y divide-slate-800/60 flex-1">
            {contacts.length === 0 ? (
              <div className="py-8 text-center text-sm text-slate-500">
                Noch keine Kontakte angelegt.
              </div>
            ) : (
              contacts.map((c: any) => {
                const email = getFieldText(c.email);
                const phone = getFieldText(c.phone);
                const position = getFieldText(c.position, 'Interessent');
                return (
                  <div key={c.id} className="py-3 flex items-center justify-between">
                    <div>
                      <div className="font-medium text-sm text-slate-200">
                        {c.first_name} {c.last_name}
                      </div>
                      <div className="text-xs text-slate-500">
                        {email || phone || 'Keine Kontaktdaten'}
                      </div>
                    </div>
                    <span className="text-[11px] px-2 py-0.5 rounded-full bg-slate-800 text-slate-300">
                      {position}
                    </span>
                  </div>
                );
              })
            )}
          </div>
        </div>

        {/* Open Todos */}
        <div className="bg-slate-900 border border-slate-800 rounded-xl p-5 flex flex-col">
          <div className="flex items-center justify-between pb-4 border-b border-slate-800">
            <h2 className="font-semibold text-slate-200 text-base">Offene Todos & Termine</h2>
            <Link
              to="/todos"
              className="text-xs text-emerald-400 hover:underline inline-flex items-center gap-1"
            >
              Alle Todos <ArrowUpRight className="w-3 h-3" />
            </Link>
          </div>

          <div className="divide-y divide-slate-800/60 flex-1">
            {todos.length === 0 ? (
              <div className="py-8 text-center text-sm text-slate-500">Keine offenen Aufgaben.</div>
            ) : (
              todos.map((t: any) => (
                <div key={t.id} className="py-3 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <input
                      type="checkbox"
                      className="rounded border-slate-700 bg-slate-950 text-emerald-500 focus:ring-0 cursor-pointer"
                    />
                    <div>
                      <div className="font-medium text-sm text-slate-200">{t.title}</div>
                      <div className="text-xs text-slate-500">
                        {t.description || 'Keine Zusatzbeschreibung'}
                      </div>
                    </div>
                  </div>
                  <span className="text-[11px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                    {t.priority}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
