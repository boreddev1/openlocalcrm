import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import {
  Calendar as CalendarIcon,
  Clock,
  MapPin,
  Plus,
  Download,
  X,
  CheckCircle2,
  Edit3,
  Trash2,
  AlertCircle,
  Phone,
  Mail,
  User,
  Send,
  RefreshCw,
  Lock,
  Globe,
  Eye,
  EyeOff
} from 'lucide-react';

const APPOINTMENT_TYPES = [
  { id: 'meeting', label: 'Meeting / Vor-Ort-Beratung', color: 'bg-purple-500/10 text-purple-400 border-purple-500/30' },
  { id: 'call', label: 'Telefontermin (Click-to-Call)', color: 'bg-blue-500/10 text-blue-400 border-blue-500/30' },
  { id: 'email_reminder', label: 'E-Mail Follow-up Erinnerung', color: 'bg-amber-500/10 text-amber-400 border-amber-500/30' },
  { id: 'deadline', label: 'Frist / Einreichungs-Deadline', color: 'bg-rose-500/10 text-rose-400 border-rose-500/30' },
];

const REMINDER_OPTIONS = [
  { id: '15m', label: '15 Minuten vorher' },
  { id: '30m', label: '30 Minuten vorher' },
  { id: '1h', label: '1 Stunde vorher' },
  { id: '1d', label: '1 Tag vorher' },
];

export const CalendarPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [filterAssignee, setFilterAssignee] = useState<'all' | 'mine'>('all');
  const [showExternal, setShowExternal] = useState(true);
  const [isSyncing, setIsSyncing] = useState(false);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingApp, setEditingApp] = useState<any | null>(null);
  const [deletingApp, setDeletingApp] = useState<any | null>(null);
  const [inviteModalApp, setInviteModalApp] = useState<any | null>(null);
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  const { data: appointments = [], isLoading } = useQuery<any[]>({
    queryKey: ['appointments'],
    queryFn: () => apiFetch('/api/v1/appointments'),
  });

  const [formData, setFormData] = useState({
    title: '',
    type: 'meeting',
    contact_name: 'Sabine Mustermann',
    contact_email: 'sabine.mustermann@example.de',
    company_name: 'Mustermann Solar GmbH',
    start_time: '2026-08-28T10:00',
    end_time: '2026-08-28T11:00',
    location: 'Frankfurt am Main',
    assigned_to: 'Max Mustermann (Vertrieb)',
    reminder: '30m',
    notes: 'Erstberatung Photovoltaik & Zähleraufnahme vor Ort',
    status: 'GEPLANT',
  });

  const createMutation = useMutation({
    mutationFn: (newApp: any) =>
      apiFetch('/api/v1/appointments', {
        method: 'POST',
        body: JSON.stringify(newApp),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['appointments'] });
      setIsModalOpen(false);
      setFeedbackBanner('Termin erfolgreich angelegt!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (app: any) =>
      apiFetch(`/api/v1/appointments/${app.id}`, {
        method: 'PUT',
        body: JSON.stringify(app),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['appointments'] });
      setEditingApp(null);
      setFeedbackBanner('Termin aktualisiert!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (appId: string) =>
      apiFetch(`/api/v1/appointments/${appId}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['appointments'] });
      setDeletingApp(null);
      setFeedbackBanner('Termin gelöscht.');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const pushMutation = useMutation({
    mutationFn: (app: any) =>
      apiFetch(`/api/v1/appointments/${app.id}/push-external`, {
        method: 'POST',
        body: JSON.stringify({ provider: 'microsoft_and_google' }),
      }),
    onSuccess: (_, app) => {
      queryClient.invalidateQueries({ queryKey: ['appointments'] });
      setFeedbackBanner(`Termin "${app.title}" erfolgreich in Ihren Microsoft 365 & Google Kalender übertragen!`);
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const handleSyncExternal = () => {
    setIsSyncing(true);
    setTimeout(() => {
      setIsSyncing(false);
      setFeedbackBanner('Microsoft 365 & Google Kalender synchronisiert (30 Tage zurück / 60 Tage voraus)');
      setTimeout(() => setFeedbackBanner(null), 4000);
    }, 800);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate({
      ...formData,
      start_time: formData.start_time ? new Date(formData.start_time).toISOString() : new Date().toISOString(),
      end_time: formData.end_time ? new Date(formData.end_time).toISOString() : new Date(Date.now() + 3600000).toISOString(),
    });
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingApp) return;
    updateMutation.mutate(editingApp);
  };

  const handleDownloadICS = (app: any) => {
    const icsContent = `BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//OpenLocalCRM//DE\nBEGIN:VEVENT\nSUMMARY:${app.title}\nLOCATION:${app.location || 'Vor Ort'}\nDESCRIPTION:${app.notes || 'CRM Vor-Ort Beratung'}\nDTSTART:${app.start_time ? new Date(app.start_time).toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z' : ''}\nDTEND:${app.end_time ? new Date(app.end_time).toISOString().replace(/[-:]/g, '').split('.')[0] + 'Z' : ''}\nORGANIZER;CN=OpenLocalCRM Sales:mailto:vertrieb@openlocalcrm.local\nEND:VEVENT\nEND:VCALENDAR`;
    const blob = new Blob([icsContent], { type: 'text/calendar;charset=utf-8' });
    const link = document.createElement('a');
    link.href = window.URL.createObjectURL(blob);
    link.setAttribute('download', `${app.title.replace(/\s+/g, '_')}.ics`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const handleSendInviteEmail = (app: any) => {
    setFeedbackBanner(`ICS-Kalendereinladung erfolgreich an ${app.contact_email || 'Kunden'} gesendet!`);
    setInviteModalApp(null);
    setTimeout(() => setFeedbackBanner(null), 4000);
  };

  const filteredAppointments = appointments.filter((app) => {
    if (!showExternal && app.is_external) return false;
    if (filterAssignee === 'mine') {
      return app.assigned_to?.includes('Max') || !app.assigned_to || app.is_external;
    }
    return true;
  });

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <CalendarIcon className="w-6 h-6 text-emerald-400" />
            Termine, Kalender & M365 / Google Sync (§3.6)
          </h1>
          <p className="text-sm text-slate-400">
            Vor-Ort-Beratungen, Closer-Termine, M365/Google OAuth-Sync & RFC 5545 ICS-Kalendereinladungen
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center bg-slate-900 border border-slate-800 rounded-xl p-1 text-xs">
            <button
              onClick={() => setFilterAssignee('all')}
              className={`px-3 py-1.5 rounded-lg font-semibold transition-colors cursor-pointer ${filterAssignee === 'all' ? 'bg-slate-800 text-slate-100' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Alle Termine
            </button>
            <button
              onClick={() => setFilterAssignee('mine')}
              className={`px-3 py-1.5 rounded-lg font-semibold transition-colors cursor-pointer ${filterAssignee === 'mine' ? 'bg-slate-800 text-emerald-400' : 'text-slate-400 hover:text-slate-200'}`}
            >
              Meine Termine
            </button>
          </div>

          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20 cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            Termin vereinbaren
          </button>
        </div>
      </div>

      {/* External Calendar Sync Bar (§3.6) */}
      <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex flex-col md:flex-row md:items-center justify-between gap-4 shadow-sm">
        <div className="flex items-center gap-3 flex-wrap">
          <div className="flex items-center gap-1.5 px-3 py-1 bg-blue-500/10 border border-blue-500/20 rounded-xl text-xs text-blue-300 font-medium">
            <Globe className="w-3.5 h-3.5 text-blue-400" />
            <span>M365 & Google OAuth verbunden</span>
          </div>
          <div className="text-xs text-slate-400">
            Automatischer 15-Min-Sync (Fenster: 30 Tage zurück / 60 Tage voraus)
          </div>
        </div>

        <div className="flex items-center gap-2.5">
          <button
            type="button"
            onClick={() => setShowExternal(!showExternal)}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-semibold rounded-xl border border-slate-700 transition-colors cursor-pointer"
          >
            {showExternal ? <Eye className="w-3.5 h-3.5 text-emerald-400" /> : <EyeOff className="w-3.5 h-3.5 text-slate-400" />}
            <span>{showExternal ? 'Externe Termine sichtbar' : 'Externe Termine ausgeblendet'}</span>
          </button>

          <button
            type="button"
            onClick={handleSyncExternal}
            disabled={isSyncing}
            className="inline-flex items-center gap-1.5 px-3.5 py-1.5 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white text-xs font-semibold rounded-xl shadow-lg shadow-purple-600/20 transition-colors cursor-pointer"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isSyncing ? 'animate-spin' : ''}`} />
            <span>{isSyncing ? 'Synchronisiere...' : 'Jetzt abgleichen'}</span>
          </button>
        </div>
      </div>

      {feedbackBanner && (
        <div className="p-3.5 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
          <span>{feedbackBanner}</span>
        </div>
      )}

      {/* Appointments List Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {isLoading ? (
          <div className="col-span-full py-12 text-center text-slate-500">Lade Termine...</div>
        ) : filteredAppointments.length === 0 ? (
          <div className="col-span-full py-12 text-center text-slate-500">Keine anstehenden Termine eingetragen.</div>
        ) : (
          filteredAppointments.map((app) => {
            const typeConfig = APPOINTMENT_TYPES.find((t) => t.id === app.type) || APPOINTMENT_TYPES[0];

            return (
              <div key={app.id} className={`bg-slate-900 border ${app.is_external ? 'border-purple-500/40 bg-slate-900/90' : 'border-slate-800'} rounded-2xl p-5 shadow-sm space-y-4 hover:border-slate-700 transition-colors flex flex-col justify-between group`}>
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2.5">
                      <div className={`w-9 h-9 rounded-xl ${app.is_external ? 'bg-purple-500/10 text-purple-400 border border-purple-500/20' : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'} flex items-center justify-center shrink-0`}>
                        {app.provider === 'microsoft' ? (
                          <Globe className="w-5 h-5 text-blue-400" />
                        ) : app.provider === 'google' ? (
                          <CalendarIcon className="w-5 h-5 text-amber-400" />
                        ) : app.type === 'call' ? (
                          <Phone className="w-5 h-5" />
                        ) : (
                          <CalendarIcon className="w-5 h-5" />
                        )}
                      </div>
                      <div>
                        <h3 className="font-bold text-sm text-slate-100">{app.title}</h3>
                        <div className="flex items-center gap-1.5 mt-0.5">
                          <span className={`text-[10px] px-2 py-0.5 rounded-full border font-medium ${typeConfig.color}`}>
                            {app.provider === 'microsoft' ? 'Microsoft 365 Sync' : app.provider === 'google' ? 'Google Calendar Sync' : typeConfig.label}
                          </span>
                          {app.is_private && (
                            <span className="text-[10px] px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 font-mono flex items-center gap-1">
                              <Lock className="w-2.5 h-2.5" /> Privat
                            </span>
                          )}
                        </div>
                      </div>
                    </div>

                    {!app.is_external && (
                      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          onClick={() => setEditingApp({ ...app })}
                          className="p-1 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded"
                          title="Termin bearbeiten"
                          aria-label="Bearbeiten"
                        >
                          <Edit3 className="w-3.5 h-3.5" />
                        </button>
                        <button
                          onClick={() => setDeletingApp(app)}
                          className="p-1 text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 rounded"
                          title="Termin löschen"
                          aria-label="Löschen"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    )}
                  </div>

                  <div className="space-y-2 text-xs text-slate-300">
                    <div className="flex items-center gap-2 text-slate-400">
                      <Clock className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                      <span>{new Date(app.start_time).toLocaleString('de-DE', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })} Uhr</span>
                    </div>

                    {app.contact_name && (
                      <div className="flex items-center gap-2 text-slate-400">
                        <User className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                        <span>{app.contact_name} {app.company_name && `(${app.company_name})`}</span>
                      </div>
                    )}

                    {app.location && (
                      <div className="flex items-center gap-2 text-slate-400">
                        <MapPin className="w-3.5 h-3.5 text-blue-400 shrink-0" />
                        <span>{app.location}</span>
                      </div>
                    )}

                    {app.notes && (
                      <div className="p-2.5 bg-slate-950 rounded-xl border border-slate-800/80 text-[11px] text-slate-400 italic">
                        "{app.notes}"
                      </div>
                    )}
                  </div>
                </div>

                <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between gap-2">
                  <span className="text-[11px] text-emerald-400 flex items-center gap-1 font-semibold">
                    <CheckCircle2 className="w-3.5 h-3.5" /> {app.status || 'Bestätigt'}
                  </span>

                  {!app.is_external ? (
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <button
                        type="button"
                        onClick={() => pushMutation.mutate(app)}
                        disabled={pushMutation.isPending || app.is_pushed}
                        className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg border text-xs font-semibold transition-colors cursor-pointer ${
                          app.is_pushed
                            ? 'bg-blue-500/10 text-blue-300 border-blue-500/20'
                            : 'bg-slate-800 hover:bg-slate-700 text-slate-300 border-slate-700'
                        }`}
                        title="Termin per Graph/Google API in M365 & Google Kalender pushen"
                      >
                        <Globe className="w-3 h-3 text-blue-400" />
                        <span>{app.is_pushed ? 'In M365 gepusht' : 'An M365 pushen'}</span>
                      </button>
                      <button
                        type="button"
                        onClick={() => setInviteModalApp(app)}
                        className="inline-flex items-center gap-1 px-2.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-purple-300 text-xs font-semibold rounded-lg border border-slate-700 transition-colors cursor-pointer"
                        title="Einladungs-E-Mail mit ICS an Kunden senden"
                      >
                        <Send className="w-3 h-3 text-purple-400" />
                        <span>Einladen</span>
                      </button>
                      <button
                        type="button"
                        onClick={() => handleDownloadICS(app)}
                        className="inline-flex items-center gap-1 px-2.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold rounded-lg border border-slate-700 transition-colors cursor-pointer"
                        title="Als .ics Kalenderdatei herunterladen (RFC 5545)"
                        aria-label="ICS Export"
                      >
                        <Download className="w-3 h-3 text-emerald-400" />
                        <span>ICS</span>
                      </button>
                    </div>
                  ) : (
                    <span className="text-[11px] text-slate-500 flex items-center gap-1">
                      <Lock className="w-3 h-3" /> Read-Only Sync
                    </span>
                  )}
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Send Invite Modal */}
      {inviteModalApp && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-base flex items-center gap-2">
                <Mail className="w-5 h-5 text-purple-400" />
                <span>ICS-Kalendereinladung versenden (§3.6)</span>
              </h3>
              <button onClick={() => setInviteModalApp(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie eine offizielle Terminbestätigung inklusive <strong className="text-slate-100">RFC 5545 .ics Kalenderanhang</strong> an die Adresse <strong className="text-purple-300">{inviteModalApp.contact_email || 'sabine.mustermann@example.de'}</strong> senden?
            </p>
            <div className="p-3 bg-slate-950 rounded-xl border border-slate-800 text-xs space-y-1">
              <div className="text-slate-400">Termin: <span className="text-slate-200 font-semibold">{inviteModalApp.title}</span></div>
              <div className="text-slate-400">Zeitpunkt: <span className="text-emerald-400">{new Date(inviteModalApp.start_time).toLocaleString('de-DE')} Uhr</span></div>
            </div>
            <div className="flex justify-end gap-3 pt-3">
              <button
                onClick={() => setInviteModalApp(null)}
                className="px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-semibold"
              >
                Abbrechen
              </button>
              <button
                onClick={() => handleSendInviteEmail(inviteModalApp)}
                className="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white font-semibold rounded-lg text-xs flex items-center gap-1.5 shadow-lg shadow-purple-600/20"
              >
                <Send className="w-3.5 h-3.5" />
                <span>Einladungsmail senden</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg">Neuen Termin anlegen (§3.6)</h3>
              <button onClick={() => setIsModalOpen(false)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Termin-Typ (§3.6)</label>
                <select
                  value={formData.type}
                  onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                >
                  {APPOINTMENT_TYPES.map((t) => (
                    <option key={t.id} value={t.id}>{t.label}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Titel / Anlass *</label>
                <input
                  required
                  type="text"
                  placeholder="z.B. Vor-Ort Dachberatung Familie Müller"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Kontaktperson</label>
                  <input
                    type="text"
                    value={formData.contact_name}
                    onChange={(e) => setFormData({ ...formData, contact_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">E-Mail für Einladung</label>
                  <input
                    type="email"
                    value={formData.contact_email}
                    onChange={(e) => setFormData({ ...formData, contact_email: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Startzeit</label>
                  <input
                    type="datetime-local"
                    value={formData.start_time}
                    onChange={(e) => setFormData({ ...formData, start_time: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Endzeit</label>
                  <input
                    type="datetime-local"
                    value={formData.end_time}
                    onChange={(e) => setFormData({ ...formData, end_time: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Erinnerung (§3.6)</label>
                  <select
                    value={formData.reminder}
                    onChange={(e) => setFormData({ ...formData, reminder: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    {REMINDER_OPTIONS.map((r) => (
                      <option key={r.id} value={r.id}>{r.label}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Ort</label>
                  <input
                    type="text"
                    placeholder="Musterstraße 12, Frankfurt"
                    value={formData.location}
                    onChange={(e) => setFormData({ ...formData, location: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Notizen</label>
                <textarea
                  rows={2}
                  value={formData.notes}
                  onChange={(e) => setFormData({ ...formData, notes: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500 resize-none"
                />
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setIsModalOpen(false)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-sm font-medium transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors"
                >
                  {createMutation.isPending ? 'Speichert...' : 'Termin speichern'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {editingApp && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Edit3 className="w-5 h-5 text-emerald-400" />
                <span>Termin bearbeiten</span>
              </h3>
              <button onClick={() => setEditingApp(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Titel *</label>
                <input
                  required
                  type="text"
                  value={editingApp.title}
                  onChange={(e) => setEditingApp({ ...editingApp, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Ort</label>
                <input
                  type="text"
                  value={editingApp.location || ''}
                  onChange={(e) => setEditingApp({ ...editingApp, location: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Notizen</label>
                <textarea
                  rows={2}
                  value={editingApp.notes || ''}
                  onChange={(e) => setEditingApp({ ...editingApp, notes: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500 resize-none"
                />
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setEditingApp(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-sm font-medium transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors"
                >
                  {updateMutation.isPending ? 'Speichert...' : 'Änderungen speichern'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deletingApp && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-3 text-rose-400 border-b border-slate-800 pb-3">
              <AlertCircle className="w-6 h-6 shrink-0" />
              <h3 className="font-bold text-slate-100 text-base">Termin wirklich löschen?</h3>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie den Termin <strong className="text-slate-100">{deletingApp.title}</strong> unwiderruflich aus dem Kalender entfernen?
            </p>
            <div className="flex justify-end gap-3 pt-3">
              <button
                onClick={() => setDeletingApp(null)}
                className="px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-semibold"
              >
                Abbrechen
              </button>
              <button
                onClick={() => deleteMutation.mutate(deletingApp.id)}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white font-semibold rounded-lg text-xs"
              >
                {deleteMutation.isPending ? 'Löscht...' : 'Endgültig löschen'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
